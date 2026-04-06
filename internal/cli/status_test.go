package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupStatusDB creates a temp HOME, initializes a DB, and inserts test books.
// Returns the config file path. The DB is at HOME/.config/earworm/earworm.db.
func setupStatusDB(t *testing.T, books []db.Book) string {
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

	// Open DB and insert books
	dbPath := filepath.Join(cfgDir, "earworm.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	for _, b := range books {
		require.NoError(t, db.UpsertBook(database, b))
	}

	return cfgPath
}

func TestStatusEmptyLibrary(t *testing.T) {
	cfgPath := setupStatusDB(t, nil)

	out, err := executeCommand(t, "--config", cfgPath, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "No books")
}

func TestStatusWithBooks(t *testing.T) {
	books := []db.Book{
		{
			ASIN:      "B08C6YJ1LS",
			Title:     "The Great Novel",
			Author:    "Jane Smith",
			Status:    "scanned",
			LocalPath: "/lib/Jane Smith/The Great Novel [B08C6YJ1LS]",
		},
		{
			ASIN:      "B09ABCDEF1",
			Title:     "Another Story",
			Author:    "John Doe",
			Status:    "downloaded",
			LocalPath: "/lib/John Doe/Another Story [B09ABCDEF1]",
		},
	}

	cfgPath := setupStatusDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "Jane Smith")
	assert.Contains(t, out, "The Great Novel")
	assert.Contains(t, out, "B08C6YJ1LS")
	assert.Contains(t, out, "John Doe")
	assert.Contains(t, out, "2 books total")
}

func TestStatusJSON(t *testing.T) {
	books := []db.Book{
		{
			ASIN:      "B08C6YJ1LS",
			Title:     "Test Book",
			Author:    "Test Author",
			Narrator:  "Test Narrator",
			Status:    "scanned",
			LocalPath: "/lib/Test Author/Test Book [B08C6YJ1LS]",
		},
	}

	cfgPath := setupStatusDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "status", "--json")
	require.NoError(t, err)

	// Validate it's parseable JSON
	var result []map[string]interface{}
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(t, err, "JSON output should be valid: %s", out)
	require.Len(t, result, 1)
	assert.Equal(t, "B08C6YJ1LS", result[0]["ASIN"])
	assert.Equal(t, "Test Book", result[0]["Title"])
	assert.Equal(t, "Test Author", result[0]["Author"])
}

func TestStatusJSONFields(t *testing.T) {
	books := []db.Book{
		{
			ASIN:      "B08C6YJ1LS",
			Title:     "Field Test",
			Author:    "Auth Name",
			Narrator:  "Narr Name",
			Status:    "scanned",
			LocalPath: "/lib/test",
		},
	}

	cfgPath := setupStatusDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "status", "--json")
	require.NoError(t, err)

	var result []map[string]interface{}
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// Check expected fields are present
	book := result[0]
	assert.Contains(t, book, "ASIN")
	assert.Contains(t, book, "Title")
	assert.Contains(t, book, "Author")
	assert.Contains(t, book, "Narrator")
	assert.Contains(t, book, "Status")
}

func TestStatusIndicator(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"scanned", "OK"},
		{"downloaded", "DL"},
		{"organized", "OK"},
		{"error", "ERR"},
		{"removed", "GONE"},
		{"anything_else", "?"},
		{"", "?"},
		{"unknown", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := statusIndicator(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStatusFilterAuthor(t *testing.T) {
	books := []db.Book{
		{
			ASIN:      "B08C6YJ1LS",
			Title:     "Book One",
			Author:    "Specific Author",
			Status:    "scanned",
			LocalPath: "/lib/test1",
		},
		{
			ASIN:      "B09ABCDEF1",
			Title:     "Book Two",
			Author:    "Other Writer",
			Status:    "scanned",
			LocalPath: "/lib/test2",
		},
	}

	cfgPath := setupStatusDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "status", "--author", "Specific Author")
	require.NoError(t, err)
	assert.Contains(t, out, "Specific Author")
	assert.Contains(t, out, "Book One")
	assert.NotContains(t, out, "Other Writer")
	assert.NotContains(t, out, "Book Two")
	assert.Contains(t, out, "1 books total")
}
