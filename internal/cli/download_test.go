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

// setupDownloadDB creates a temp HOME with config, DB, and optional books.
func setupDownloadDB(t *testing.T, books []db.Book) string {
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
		require.NoError(t, db.SyncRemoteBook(database, b))
	}

	return cfgPath
}

func TestDryRun_WithBooks(t *testing.T) {
	books := []db.Book{
		{
			ASIN:           "B08C6YJ1LS",
			Title:          "The Great Novel",
			Author:         "Jane Smith",
			Narrators:      "Bob Reader",
			RuntimeMinutes: 420,
			AudibleStatus:  "new",
			PurchaseDate:   "2024-01-15",
		},
		{
			ASIN:           "B09ABCDEF1",
			Title:          "Another Story",
			Author:         "John Doe",
			Narrators:      "Alice Voice",
			RuntimeMinutes: 90,
			AudibleStatus:  "in_progress",
			PurchaseDate:   "2024-02-20",
		},
	}

	cfgPath := setupDownloadDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "download", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "Jane Smith - The Great Novel [B08C6YJ1LS] (7h 0m)")
	assert.Contains(t, out, "John Doe - Another Story [B09ABCDEF1] (1h 30m)")
	assert.Contains(t, out, "2 books to download")
}

func TestDryRun_NoBooks(t *testing.T) {
	cfgPath := setupDownloadDB(t, nil)

	out, err := executeCommand(t, "--config", cfgPath, "download", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "No new books to download")
}

func TestDryRun_JSON(t *testing.T) {
	books := []db.Book{
		{
			ASIN:           "B08C6YJ1LS",
			Title:          "JSON Test Book",
			Author:         "JSON Author",
			Narrators:      "JSON Narrator",
			RuntimeMinutes: 180,
			AudibleStatus:  "new",
			PurchaseDate:   "2024-01-01",
			SeriesName:     "Test Series",
			SeriesPosition: "1",
		},
	}

	cfgPath := setupDownloadDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "download", "--dry-run", "--json")
	require.NoError(t, err)

	var result []dryRunBook
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(t, err, "JSON output should be valid: %s", out)
	require.Len(t, result, 1)
	assert.Equal(t, "B08C6YJ1LS", result[0].ASIN)
	assert.Equal(t, "JSON Test Book", result[0].Title)
	assert.Equal(t, "JSON Author", result[0].Author)
	assert.Equal(t, 180, result[0].RuntimeMinutes)
	assert.Equal(t, "Test Series", result[0].SeriesName)
	assert.Equal(t, "1", result[0].SeriesPosition)
}

func TestDownload_NoLibraryPath(t *testing.T) {
	// Setup with no library_path in config
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("audible_cli_path: audible\n"), 0644))

	_, err := executeCommand(t, "--config", cfgPath, "download")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library_path not configured")
	assert.Contains(t, err.Error(), "earworm config set library_path")
}

func TestDownload_LimitFlagRegistered(t *testing.T) {
	flag := downloadCmd.Flags().Lookup("limit")
	require.NotNil(t, flag)
	assert.Equal(t, "0", flag.DefValue)
	assert.Contains(t, flag.Usage, "maximum")
}

func TestDownload_ASINFlagRegistered(t *testing.T) {
	flag := downloadCmd.Flags().Lookup("asin")
	require.NotNil(t, flag)
	assert.Contains(t, flag.Usage, "ASIN")
}

func TestDryRun_WithLimit(t *testing.T) {
	books := []db.Book{
		{
			ASIN:           "B001",
			Title:          "Book One",
			Author:         "Author A",
			Narrators:      "Narrator",
			RuntimeMinutes: 120,
			AudibleStatus:  "new",
			PurchaseDate:   "2024-01-01",
		},
		{
			ASIN:           "B002",
			Title:          "Book Two",
			Author:         "Author B",
			Narrators:      "Narrator",
			RuntimeMinutes: 180,
			AudibleStatus:  "new",
			PurchaseDate:   "2024-02-01",
		},
		{
			ASIN:           "B003",
			Title:          "Book Three",
			Author:         "Author C",
			Narrators:      "Narrator",
			RuntimeMinutes: 240,
			AudibleStatus:  "new",
			PurchaseDate:   "2024-03-01",
		},
	}

	cfgPath := setupDownloadDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "download", "--dry-run", "--limit", "2")
	require.NoError(t, err)
	assert.Contains(t, out, "2 books to download")
}

func TestDryRun_WithASINFilter(t *testing.T) {
	books := []db.Book{
		{
			ASIN:           "B001",
			Title:          "Book One",
			Author:         "Author A",
			Narrators:      "Narrator",
			RuntimeMinutes: 120,
			AudibleStatus:  "new",
			PurchaseDate:   "2024-01-01",
		},
		{
			ASIN:           "B002",
			Title:          "Book Two",
			Author:         "Author B",
			Narrators:      "Narrator",
			RuntimeMinutes: 180,
			AudibleStatus:  "new",
			PurchaseDate:   "2024-02-01",
		},
	}

	cfgPath := setupDownloadDB(t, books)

	out, err := executeCommand(t, "--config", cfgPath, "download", "--dry-run", "--asin", "B001")
	require.NoError(t, err)
	assert.Contains(t, out, "Book One")
	assert.NotContains(t, out, "Book Two")
	assert.Contains(t, out, "1 books to download")
}
