package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/organize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadOrganizeHandoff(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// Simulate post-download state: book in DB as 'downloaded', files in staging
	require.NoError(t, db.SyncRemoteBook(database, db.Book{
		ASIN: "B000TEST01", Title: "Test Book", Author: "Test Author",
		AudibleStatus: "finished",
	}))
	require.NoError(t, db.UpdateDownloadComplete(database, "B000TEST01", ""))

	// Create staging files (as download pipeline leaves them after verifyStaged)
	asinDir := filepath.Join(stagingDir, "B000TEST01")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "B000TEST01-AAX_44_128.m4b"), []byte("audio-data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "cover(500).jpg"), []byte("image-data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "chapter.json"), []byte(`{"chapters":[]}`), 0644))

	// Run organize
	results, err := organize.OrganizeAll(database, stagingDir, libraryDir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)

	// Verify Libation-compatible structure: Author/Title [ASIN]/
	expectedDir := filepath.Join(libraryDir, "Test Author", "Test Book [B000TEST01]")
	assert.DirExists(t, expectedDir)

	// ORG-02: Audio renamed to Title.m4b
	assert.FileExists(t, filepath.Join(expectedDir, "Test Book.m4b"))
	// ORG-02: Cover renamed to cover.jpg
	assert.FileExists(t, filepath.Join(expectedDir, "cover.jpg"))
	// ORG-02: Chapters renamed to chapters.json
	assert.FileExists(t, filepath.Join(expectedDir, "chapters.json"))

	// Verify DB status = 'organized' with correct local_path
	book, err := db.GetBook(database, "B000TEST01")
	require.NoError(t, err)
	assert.Equal(t, "organized", book.Status)
	assert.Equal(t, expectedDir, book.LocalPath)

	// Verify staging directory cleaned up
	_, statErr := os.Stat(asinDir)
	assert.True(t, os.IsNotExist(statErr), "staging dir should be removed after organize")
}

func TestDownloadOrganizeHandoff_M4A(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	require.NoError(t, db.SyncRemoteBook(database, db.Book{
		ASIN: "B000TEST02", Title: "Another Book", Author: "Jane Smith",
		AudibleStatus: "finished",
	}))
	require.NoError(t, db.UpdateDownloadComplete(database, "B000TEST02", ""))

	asinDir := filepath.Join(stagingDir, "B000TEST02")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "B000TEST02.m4a"), []byte("audio"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "cover(500).jpg"), []byte("img"), 0644))

	results, err := organize.OrganizeAll(database, stagingDir, libraryDir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)

	expectedDir := filepath.Join(libraryDir, "Jane Smith", "Another Book [B000TEST02]")
	assert.FileExists(t, filepath.Join(expectedDir, "Another Book.m4a"))
	assert.FileExists(t, filepath.Join(expectedDir, "cover.jpg"))
}

func TestDownloadOrganizeHandoff_MissingStagingDir(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// Book marked as downloaded but staging dir missing (pre-fix scenario)
	require.NoError(t, db.SyncRemoteBook(database, db.Book{
		ASIN: "B000MISS01", Title: "Missing Book", Author: "Author X",
		AudibleStatus: "finished",
	}))
	require.NoError(t, db.UpdateDownloadComplete(database, "B000MISS01", ""))
	// Note: no staging files created

	results, err := organize.OrganizeAll(database, stagingDir, libraryDir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Success)
	assert.Contains(t, results[0].Error, "staging directory")

	// DB should be in error state
	book, err := db.GetBook(database, "B000MISS01")
	require.NoError(t, err)
	assert.Equal(t, "error", book.Status)
}
