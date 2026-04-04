package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/organize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupOrganizeEnv creates a temp HOME with config, DB, staging dir, and library dir.
// Returns the config path. Books are inserted with InsertBook (not SyncRemoteBook)
// so we can set status directly.
func setupOrganizeEnv(t *testing.T, books []db.Book) (cfgPath string, stagingDir string, libraryDir string) {
	t.Helper()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	stagingDir = filepath.Join(tmpHome, "staging")
	libraryDir = filepath.Join(tmpHome, "library")
	require.NoError(t, os.MkdirAll(stagingDir, 0755))
	require.NoError(t, os.MkdirAll(libraryDir, 0755))

	cfgContent := "library_path: " + libraryDir + "\nstaging_path: " + stagingDir + "\n"
	cfgPath = filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

	dbPath := filepath.Join(cfgDir, "earworm.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	for _, b := range books {
		require.NoError(t, db.InsertBook(database, b))
	}

	return cfgPath, stagingDir, libraryDir
}

func TestOrganizeCommand_NoConfig(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Create config dir with empty config (no library_path)
	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(""), 0644))

	_, err := executeCommand(t, "--config", cfgPath, "organize")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library_path")
}

func TestOrganizeCommand_JSON(t *testing.T) {
	books := []db.Book{
		{
			ASIN:          "ORG001",
			Title:         "Test Book",
			Author:        "Author Name",
			Status:        "downloaded",
			AudibleStatus: "finished",
		},
	}
	cfgPath, stagingDir, _ := setupOrganizeEnv(t, books)

	// Create staging files
	asinDir := filepath.Join(stagingDir, "ORG001")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4a"), []byte("data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "cover.jpg"), []byte("img"), 0644))

	out, err := executeCommand(t, "--config", cfgPath, "organize", "--json")
	require.NoError(t, err)

	var result jsonOrganizeOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, 1, result.Organized)
	assert.Equal(t, 0, result.Errors)
	assert.Len(t, result.Results, 1)
	assert.True(t, result.Results[0].Success)
	assert.Equal(t, "ORG001", result.Results[0].ASIN)
}

func TestOrganizeCommand_NoBooksToOrganize(t *testing.T) {
	// No downloaded books
	books := []db.Book{
		{ASIN: "SCAN001", Title: "Scanned Book", Author: "Author", Status: "scanned"},
	}
	cfgPath, _, _ := setupOrganizeEnv(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "organize")
	require.NoError(t, err)
	assert.Contains(t, out, "Organized 0 books, 0 errors")
}

func TestOrganizeCommand_Registered(t *testing.T) {
	// Verify organizeCmd is registered as subcommand
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "organize" {
			found = true
			// Verify --json flag exists
			flag := cmd.Flags().Lookup("json")
			assert.NotNil(t, flag, "--json flag should exist")
			break
		}
	}
	assert.True(t, found, "organize command should be registered")
}

func TestOrganizeCommand_TextOutput(t *testing.T) {
	books := []db.Book{
		{
			ASIN:          "TXT001",
			Title:         "My Book",
			Author:        "Writer, Famous",
			Status:        "downloaded",
			AudibleStatus: "finished",
		},
	}
	cfgPath, stagingDir, _ := setupOrganizeEnv(t, books)

	// Create staging files
	asinDir := filepath.Join(stagingDir, "TXT001")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "book.m4a"), []byte("audio"), 0644))

	out, err := executeCommand(t, "--config", cfgPath, "organize")
	require.NoError(t, err)
	assert.Contains(t, out, "Organized:")
	assert.Contains(t, out, "Writer, Famous")
	assert.Contains(t, out, "My Book")
	assert.Contains(t, out, "Organized 1 books, 0 errors")
}

func TestOrganizeCommand_JSONWithOrganizeResult(t *testing.T) {
	// Verify JSON output uses the organize.OrganizeResult type correctly
	r := organize.OrganizeResult{
		ASIN:    "TEST",
		Title:   "Test",
		Author:  "Author",
		LibPath: "/lib/Author/Test [TEST]",
		Success: true,
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"asin":"TEST"`)
	assert.Contains(t, string(data), `"success":true`)
}
