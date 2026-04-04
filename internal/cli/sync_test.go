package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAudibleClient implements audible.AudibleClient for testing.
type fakeAudibleClient struct {
	checkAuthErr     error
	libraryItems     []audible.LibraryItem
	libraryExportErr error
	quickstartErr    error
}

func (f *fakeAudibleClient) Quickstart(ctx context.Context) error {
	return f.quickstartErr
}
func (f *fakeAudibleClient) CheckAuth(ctx context.Context) error {
	return f.checkAuthErr
}
func (f *fakeAudibleClient) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return f.libraryItems, f.libraryExportErr
}
func (f *fakeAudibleClient) Download(ctx context.Context, asin string, outputDir string) error {
	return audible.ErrNotImplemented
}

// setupSyncDB creates a temp HOME with config and empty DB for sync tests.
func setupSyncDB(t *testing.T) string {
	t.Helper()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Create config dir
	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	// Write config
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("library_path: /tmp/test\n"), 0644))

	// Create DB so it exists when sync opens it
	dbPath := filepath.Join(cfgDir, "earworm.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	database.Close()

	return cfgPath
}

func TestRunSync_Success(t *testing.T) {
	cfgPath := setupSyncDB(t)

	runtime := 360
	fake := &fakeAudibleClient{
		libraryItems: []audible.LibraryItem{
			{
				ASIN:             "B001",
				Title:            "Test Book One",
				Authors:          "Author One",
				Narrators:        "Narrator One",
				RuntimeLengthMin: &runtime,
				PurchaseDate:     "2024-01-15",
			},
			{
				ASIN:         "B002",
				Title:        "Test Book Two",
				Authors:      "Author Two",
				Narrators:    "Narrator Two",
				PurchaseDate: "2024-02-20",
				IsFinished:   true,
			},
		},
	}

	origClient := newAudibleClient
	newAudibleClient = func() audible.AudibleClient { return fake }
	t.Cleanup(func() { newAudibleClient = origClient })

	out, err := executeCommand(t, "--config", cfgPath, "sync")
	require.NoError(t, err)
	assert.Contains(t, out, "Sync complete:")
	assert.Contains(t, out, "2 books")
}

func TestRunSync_AuthFailure(t *testing.T) {
	cfgPath := setupSyncDB(t)

	fake := &fakeAudibleClient{
		checkAuthErr: &audible.AuthError{Message: "token expired"},
	}

	origClient := newAudibleClient
	newAudibleClient = func() audible.AudibleClient { return fake }
	t.Cleanup(func() { newAudibleClient = origClient })

	_, err := executeCommand(t, "--config", cfgPath, "sync")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication expired")
	assert.Contains(t, err.Error(), "earworm auth")
}

func TestRunSync_JSON(t *testing.T) {
	cfgPath := setupSyncDB(t)

	runtime := 120
	fake := &fakeAudibleClient{
		libraryItems: []audible.LibraryItem{
			{
				ASIN:             "B003",
				Title:            "JSON Book",
				Authors:          "JSON Author",
				Narrators:        "JSON Narrator",
				RuntimeLengthMin: &runtime,
				PurchaseDate:     "2024-03-10",
			},
		},
	}

	origClient := newAudibleClient
	newAudibleClient = func() audible.AudibleClient { return fake }
	t.Cleanup(func() { newAudibleClient = origClient })

	out, err := executeCommand(t, "--config", cfgPath, "--quiet", "sync", "--json")
	require.NoError(t, err)

	var summary syncSummary
	err = json.Unmarshal([]byte(out), &summary)
	require.NoError(t, err, "JSON output should be valid: %s", out)
	assert.Equal(t, 1, summary.TotalSynced)
	assert.Equal(t, 1, summary.NewBooks)
	assert.Equal(t, 0, summary.AlreadyLocal)
}
