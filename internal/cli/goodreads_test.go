package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoodreadsCommand_FileOutput(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("library_path: /tmp/test\n"), 0644))

	// Insert test books
	dbPath := filepath.Join(cfgDir, "earworm.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)

	require.NoError(t, db.UpsertBook(database, db.Book{
		ASIN:      "B001",
		Title:     "First Book",
		Author:    "Author One",
		Status:    "scanned",
		LocalPath: "/lib/test1",
	}))
	require.NoError(t, db.UpsertBook(database, db.Book{
		ASIN:      "B002",
		Title:     "Second Book",
		Author:    "Author Two",
		Status:    "scanned",
		LocalPath: "/lib/test2",
	}))
	database.Close()

	// Export to file
	outFile := filepath.Join(tmpHome, "goodreads.csv")
	out, err := executeCommand(t, "--config", cfgPath, "goodreads", "--output", outFile)
	require.NoError(t, err)
	assert.Contains(t, out, "Exported 2 books")

	// Verify file contents
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Title")
	assert.Contains(t, content, "Author")
	assert.Contains(t, content, "First Book")
	assert.Contains(t, content, "Second Book")
}

func TestGoodreadsCommand_DBError(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// No config dir -- DB can't be opened
	_, err := executeCommand(t, "goodreads")
	require.Error(t, err)
}
