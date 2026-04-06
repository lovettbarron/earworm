package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSkipDB creates a temp HOME with config dir, DB, and optional books.
// Returns the config file path.
func setupSkipDB(t *testing.T, books []db.Book) string {
	t.Helper()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("library_path: /tmp/test\n"), 0644))

	dbPath := filepath.Join(cfgDir, "earworm.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	for _, b := range books {
		require.NoError(t, db.UpsertBook(database, b))
	}

	return cfgPath
}

func TestSkipCommand_SkipBook(t *testing.T) {
	books := []db.Book{
		{
			ASIN:      "B08TEST001",
			Title:     "Test Book",
			Author:    "Test Author",
			Status:    "unknown",
			LocalPath: "/lib/test",
		},
	}
	cfgPath := setupSkipDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "skip", "B08TEST001")
	require.NoError(t, err)
	assert.Contains(t, out, "Skipped: Test Author - Test Book")

	// Verify DB updated
	dbPath := filepath.Join(filepath.Dir(cfgPath), "earworm.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	book, err := db.GetBook(database, "B08TEST001")
	require.NoError(t, err)
	require.NotNil(t, book)
	assert.Equal(t, "skipped", book.Status)
}

func TestSkipCommand_UndoSkip(t *testing.T) {
	books := []db.Book{
		{
			ASIN:      "B08TEST001",
			Title:     "Test Book",
			Author:    "Test Author",
			Status:    "skipped",
			LocalPath: "/lib/test",
		},
	}
	cfgPath := setupSkipDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "skip", "--undo", "B08TEST001")
	require.NoError(t, err)
	assert.Contains(t, out, "Un-skipped")

	// Verify DB updated
	dbPath := filepath.Join(filepath.Dir(cfgPath), "earworm.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	book, err := db.GetBook(database, "B08TEST001")
	require.NoError(t, err)
	require.NotNil(t, book)
	assert.Equal(t, "unknown", book.Status)
}

func TestSkipCommand_UnknownASIN(t *testing.T) {
	cfgPath := setupSkipDB(t, nil)

	out, err := executeCommand(t, "--config", cfgPath, "skip", "B08NONEXIST")
	require.NoError(t, err)
	assert.Contains(t, out, "Warning")
	assert.Contains(t, out, "not found")
}
