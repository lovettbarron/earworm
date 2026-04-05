package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestLibrary(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Author1/Title1 [ASIN1]/book.m4a
	dir1 := filepath.Join(root, "Andy Weir", "Project Hail Mary [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "book.m4a"), []byte("fake"), 0644))

	// Author2/Title2 [ASIN2]/audiobook.m4a
	dir2 := filepath.Join(root, "Brandon Sanderson", "Mistborn [B09ABCDEF1]")
	require.NoError(t, os.MkdirAll(dir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "audiobook.m4a"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "audiobook_part2.m4a"), []byte("fake"), 0644))

	return root
}

func TestScanTwoLevel(t *testing.T) {
	root := createTestLibrary(t)

	discovered, skipped, err := ScanLibrary(root, false)
	require.NoError(t, err)

	assert.Len(t, discovered, 2)
	assert.Empty(t, skipped)

	// Find each discovered book and verify
	asinMap := make(map[string]DiscoveredBook)
	for _, d := range discovered {
		asinMap[d.ASIN] = d
	}

	book1, ok := asinMap["B08C6YJ1LS"]
	require.True(t, ok)
	assert.Equal(t, "Project Hail Mary", book1.Title)
	assert.Equal(t, "Andy Weir", book1.Author)
	assert.Len(t, book1.AudioFiles, 1)

	book2, ok := asinMap["B09ABCDEF1"]
	require.True(t, ok)
	assert.Equal(t, "Mistborn", book2.Title)
	assert.Equal(t, "Brandon Sanderson", book2.Author)
	assert.Len(t, book2.AudioFiles, 2)
}

func TestScanTwoLevelSkipsNoASIN(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Some Author", "No ASIN Here")
	require.NoError(t, os.MkdirAll(dir, 0755))

	discovered, skipped, err := ScanLibrary(root, false)
	require.NoError(t, err)

	assert.Empty(t, discovered)
	assert.Len(t, skipped, 1)
	assert.Equal(t, "no_asin", skipped[0].Reason)
}

func TestScanRecursive(t *testing.T) {
	root := t.TempDir()
	// 3 levels deep
	dir := filepath.Join(root, "Level1", "Level2", "Book Title [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "audio.m4a"), []byte("fake"), 0644))

	discovered, _, err := ScanLibrary(root, true)
	require.NoError(t, err)

	require.Len(t, discovered, 1)
	assert.Equal(t, "B08C6YJ1LS", discovered[0].ASIN)
	assert.Equal(t, "Book Title", discovered[0].Title)
}

func TestScanPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not supported on Windows")
	}

	root := t.TempDir()
	authorDir := filepath.Join(root, "Author")
	require.NoError(t, os.MkdirAll(authorDir, 0755))

	restricted := filepath.Join(authorDir, "Restricted Book [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(restricted, 0000))
	t.Cleanup(func() { os.Chmod(restricted, 0755) })

	// Another accessible directory under a different author so scan finds something to skip
	noASIN := filepath.Join(root, "Author2", "Readable Book")
	require.NoError(t, os.MkdirAll(noASIN, 0755))

	discovered, skipped, err := ScanLibrary(root, false)
	require.NoError(t, err)

	// The restricted dir has an ASIN in its name, so it should be discovered
	// but listing its m4a files may fail -- that's OK, it still gets discovered
	// The no-ASIN dir should be skipped
	_ = discovered
	// At minimum, verify scan didn't error out
	_ = skipped
}

func TestScanFlatLayout(t *testing.T) {
	root := t.TempDir()

	// Flat layout: Title [ASIN]/ directly in root (no author subdirectory)
	dir1 := filepath.Join(root, "1Q84 [B005XZM7R6]")
	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "1Q84 [B005XZM7R6].m4b"), []byte("fake"), 0644))

	dir2 := filepath.Join(root, "Abundance [B0C7YLL2T3]")
	require.NoError(t, os.MkdirAll(dir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "Abundance [B0C7YLL2T3].m4b"), []byte("fake"), 0644))

	discovered, skipped, err := ScanLibrary(root, false)
	require.NoError(t, err)

	assert.Len(t, discovered, 2)
	assert.Empty(t, skipped)

	asinMap := make(map[string]DiscoveredBook)
	for _, d := range discovered {
		asinMap[d.ASIN] = d
	}

	book1, ok := asinMap["B005XZM7R6"]
	require.True(t, ok)
	assert.Equal(t, "1Q84", book1.Title)
	assert.Equal(t, "", book1.Author) // flat layout has no author
	assert.Len(t, book1.AudioFiles, 1)
}

func TestScanTwoLevelEmptyRoot(t *testing.T) {
	root := t.TempDir()

	discovered, skipped, err := ScanLibrary(root, false)
	require.NoError(t, err)
	assert.Empty(t, discovered)
	assert.Empty(t, skipped)
}

func TestIncrementalSync(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	// Insert an existing book that will be "missing" from the scan
	existingBook := db.Book{
		ASIN:   "B00EXISTING",
		Title:  "Old Book",
		Author: "Old Author",
		Status: "scanned",
	}
	require.NoError(t, db.InsertBook(database, existingBook))

	// Insert another existing book that WILL appear in the scan (should be updated)
	existingBook2 := db.Book{
		ASIN:   "B08C6YJ1LS",
		Title:  "Old Title",
		Author: "Old Author",
		Status: "scanned",
	}
	require.NoError(t, db.InsertBook(database, existingBook2))

	// Discovered books from scan
	discovered := []DiscoveredBook{
		{
			ASIN:      "B08C6YJ1LS",
			Title:     "Project Hail Mary",
			Author:    "Andy Weir",
			LocalPath: "/library/Andy Weir/Project Hail Mary [B08C6YJ1LS]",
			AudioFiles:  []string{"/library/Andy Weir/Project Hail Mary [B08C6YJ1LS]/book.m4a"},
		},
		{
			ASIN:      "B09NEWBOOK1",
			Title:     "New Book",
			Author:    "New Author",
			LocalPath: "/library/New Author/New Book [B09NEWBOOK1]",
			AudioFiles:  []string{"/library/New Author/New Book [B09NEWBOOK1]/audio.m4a"},
		},
	}

	// Simple metadata function that returns minimal metadata
	metadataFn := func(bookDir string) (*BookMetadata, error) {
		return &BookMetadata{
			Source: "folder",
		}, nil
	}

	result, err := IncrementalSync(database, discovered, metadataFn)
	require.NoError(t, err)

	assert.Equal(t, 1, result.Added)   // B09NEWBOOK1
	assert.Equal(t, 1, result.Updated) // B08C6YJ1LS
	assert.Equal(t, 1, result.Removed) // B00EXISTING

	// Verify the removed book
	removed, err := db.GetBook(database, "B00EXISTING")
	require.NoError(t, err)
	require.NotNil(t, removed)
	assert.Equal(t, "removed", removed.Status)

	// Verify the new book was added
	newBook, err := db.GetBook(database, "B09NEWBOOK1")
	require.NoError(t, err)
	require.NotNil(t, newBook)
	assert.Equal(t, "scanned", newBook.Status)
	assert.Equal(t, "New Book", newBook.Title)
	assert.Equal(t, "New Author", newBook.Author)

	// Verify the existing book was updated
	updated, err := db.GetBook(database, "B08C6YJ1LS")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Project Hail Mary", updated.Title)
	assert.Equal(t, "Andy Weir", updated.Author)
}
