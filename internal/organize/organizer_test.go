package organize

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func TestOrganizeBook_Success(t *testing.T) {
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// Create staging directory with files
	asinDir := filepath.Join(stagingDir, "B000000001")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "something.m4a"), []byte("audio data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "cover.jpg"), []byte("image data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "chapters.json"), []byte("{}"), 0644))

	book := db.Book{
		ASIN:   "B000000001",
		Author: "King, Stephen",
		Title:  "The Shining",
	}

	destDir, err := OrganizeBook(book, stagingDir, libraryDir)
	require.NoError(t, err)

	// Verify destination structure
	expectedDir := filepath.Join(libraryDir, "King", "The Shining [B000000001]")
	assert.Equal(t, expectedDir, destDir)

	// Verify files exist with correct names
	assert.FileExists(t, filepath.Join(expectedDir, "The Shining.m4a"))
	assert.FileExists(t, filepath.Join(expectedDir, "cover.jpg"))
	assert.FileExists(t, filepath.Join(expectedDir, "chapters.json"))

	// Verify file contents preserved
	data, err := os.ReadFile(filepath.Join(expectedDir, "The Shining.m4a"))
	require.NoError(t, err)
	assert.Equal(t, "audio data", string(data))

	// Verify staging ASIN dir removed
	_, err = os.Stat(asinDir)
	assert.True(t, os.IsNotExist(err), "staging ASIN dir should be removed")
}

func TestOrganizeBook_MissingAuthor(t *testing.T) {
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// Create staging files
	asinDir := filepath.Join(stagingDir, "B000000002")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4a"), []byte("data"), 0644))

	book := db.Book{
		ASIN:   "B000000002",
		Author: "",
		Title:  "Some Title",
	}

	_, err := OrganizeBook(book, stagingDir, libraryDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "author")

	// Verify no files moved (staging intact)
	assert.FileExists(t, filepath.Join(asinDir, "audio.m4a"))
}

func TestOrganizeBook_MissingTitle(t *testing.T) {
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	book := db.Book{
		ASIN:   "B000000003",
		Author: "Author",
		Title:  "",
	}

	_, err := OrganizeBook(book, stagingDir, libraryDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title")
}

func TestOrganizeBook_OverwriteExisting(t *testing.T) {
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// Pre-create destination with old files
	destDir := filepath.Join(libraryDir, "Author", "Title [B000000004]")
	require.NoError(t, os.MkdirAll(destDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "Title.m4a"), []byte("old audio"), 0644))

	// Create staging with new files
	asinDir := filepath.Join(stagingDir, "B000000004")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4a"), []byte("new audio"), 0644))

	book := db.Book{
		ASIN:   "B000000004",
		Author: "Author",
		Title:  "Title",
	}

	_, err := OrganizeBook(book, stagingDir, libraryDir)
	require.NoError(t, err)

	// Verify new file replaced old
	data, err := os.ReadFile(filepath.Join(destDir, "Title.m4a"))
	require.NoError(t, err)
	assert.Equal(t, "new audio", string(data))
}

func TestOrganizeBook_CoverRename(t *testing.T) {
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	asinDir := filepath.Join(stagingDir, "B000000005")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4a"), []byte("audio"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "album_art.png"), []byte("png data"), 0644))

	book := db.Book{
		ASIN:   "B000000005",
		Author: "Author",
		Title:  "Book",
	}

	destDir, err := OrganizeBook(book, stagingDir, libraryDir)
	require.NoError(t, err)

	// PNG should be renamed to cover.jpg
	assert.FileExists(t, filepath.Join(destDir, "cover.jpg"))
}

func TestOrganizeBook_ExtraFiles(t *testing.T) {
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	asinDir := filepath.Join(stagingDir, "B000000006")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4a"), []byte("audio"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "readme.txt"), []byte("info"), 0644))

	book := db.Book{
		ASIN:   "B000000006",
		Author: "Author",
		Title:  "Book",
	}

	destDir, err := OrganizeBook(book, stagingDir, libraryDir)
	require.NoError(t, err)

	// Extra files keep their original name
	assert.FileExists(t, filepath.Join(destDir, "readme.txt"))
}

func TestOrganizeAll_Integration(t *testing.T) {
	database := setupTestDB(t)
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// Insert 2 books with 'downloaded' status, 1 with 'scanned'
	require.NoError(t, db.InsertBook(database, db.Book{
		ASIN: "ALL001", Title: "Book One", Author: "Author A", Status: "downloaded", AudibleStatus: "finished",
	}))
	require.NoError(t, db.InsertBook(database, db.Book{
		ASIN: "ALL002", Title: "Book Two", Author: "Author B", Status: "downloaded", AudibleStatus: "new",
	}))
	require.NoError(t, db.InsertBook(database, db.Book{
		ASIN: "ALL003", Title: "Book Three", Author: "Author C", Status: "scanned",
	}))

	// Create staging dirs for the 2 downloaded books
	for _, asin := range []string{"ALL001", "ALL002"} {
		asinDir := filepath.Join(stagingDir, asin)
		require.NoError(t, os.MkdirAll(asinDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4a"), []byte("audio "+asin), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(asinDir, "cover.jpg"), []byte("cover "+asin), 0644))
	}

	results, err := OrganizeAll(database, stagingDir, libraryDir)
	require.NoError(t, err)
	assert.Len(t, results, 2, "should process only downloaded books")

	for _, r := range results {
		assert.True(t, r.Success, "book %s should succeed", r.ASIN)
		assert.NotEmpty(t, r.LibPath)
	}

	// Verify DB status updated to 'organized'
	b1, err := db.GetBook(database, "ALL001")
	require.NoError(t, err)
	assert.Equal(t, "organized", b1.Status)
	assert.NotEmpty(t, b1.LocalPath)

	b2, err := db.GetBook(database, "ALL002")
	require.NoError(t, err)
	assert.Equal(t, "organized", b2.Status)

	// Verify 'scanned' book untouched
	b3, err := db.GetBook(database, "ALL003")
	require.NoError(t, err)
	assert.Equal(t, "scanned", b3.Status)
}

func TestOrganizeAll_PartialFailure(t *testing.T) {
	database := setupTestDB(t)
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// Book with valid metadata
	require.NoError(t, db.InsertBook(database, db.Book{
		ASIN: "PF001", Title: "Good Book", Author: "Author", Status: "downloaded", AudibleStatus: "finished",
	}))
	// Book with empty author (will fail)
	require.NoError(t, db.InsertBook(database, db.Book{
		ASIN: "PF002", Title: "Bad Book", Author: "", Status: "downloaded", AudibleStatus: "new",
	}))

	// Create staging for the good book
	asinDir := filepath.Join(stagingDir, "PF001")
	require.NoError(t, os.MkdirAll(asinDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4a"), []byte("audio"), 0644))

	// Create staging for the bad book too
	asinDir2 := filepath.Join(stagingDir, "PF002")
	require.NoError(t, os.MkdirAll(asinDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(asinDir2, "audio.m4a"), []byte("audio"), 0644))

	results, err := OrganizeAll(database, stagingDir, libraryDir)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Find results by ASIN
	resultMap := make(map[string]OrganizeResult)
	for _, r := range results {
		resultMap[r.ASIN] = r
	}

	// Good book should succeed
	assert.True(t, resultMap["PF001"].Success)
	b1, err := db.GetBook(database, "PF001")
	require.NoError(t, err)
	assert.Equal(t, "organized", b1.Status)

	// Bad book should fail
	assert.False(t, resultMap["PF002"].Success)
	assert.Contains(t, resultMap["PF002"].Error, "author")
	b2, err := db.GetBook(database, "PF002")
	require.NoError(t, err)
	assert.Equal(t, "error", b2.Status)
	assert.Contains(t, b2.LastError, "author")
}

func TestOrganizeAll_NoBooksToOrganize(t *testing.T) {
	database := setupTestDB(t)
	stagingDir := t.TempDir()
	libraryDir := t.TempDir()

	// No downloaded books
	require.NoError(t, db.InsertBook(database, db.Book{
		ASIN: "NB001", Title: "Scanned", Author: "A", Status: "scanned",
	}))

	results, err := OrganizeAll(database, stagingDir, libraryDir)
	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Empty(t, results)
}

func TestDestinationFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		title    string
		expected string
	}{
		{"m4a renamed to title", "something.m4a", "The Shining", "The Shining.m4a"},
		{"jpg becomes cover.jpg", "album_art.jpg", "Title", "cover.jpg"},
		{"jpeg becomes cover.jpg", "photo.jpeg", "Title", "cover.jpg"},
		{"png becomes cover.jpg", "image.png", "Title", "cover.jpg"},
		{"json becomes chapters.json", "metadata.json", "Title", "chapters.json"},
		{"txt keeps name", "readme.txt", "Title", "readme.txt"},
		{"unknown keeps name", "data.bin", "Title", "data.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := destinationFilename(tt.filename, tt.title)
			assert.Equal(t, tt.expected, result)
		})
	}
}
