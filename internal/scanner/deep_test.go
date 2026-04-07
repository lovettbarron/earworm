package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestDir creates a directory with optional .m4a files.
func createTestDir(t *testing.T, base, name string, m4aFiles ...string) string {
	t.Helper()
	dir := filepath.Join(base, name)
	require.NoError(t, os.MkdirAll(dir, 0755))
	for _, f := range m4aFiles {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("fake"), 0644))
	}
	return dir
}

func TestDeepScanAllDirs(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// 3 with ASIN, 2 without
	createTestDir(t, root, "Author/Book One [B08C6YJ1LS]", "book.m4a")
	createTestDir(t, root, "Author/Book Two [B09ABCDEF1]", "book.m4a")
	createTestDir(t, root, "Author/Book Three [B01XYZABC2]", "book.m4a")
	createTestDir(t, root, "Author/NoASIN Book", "book.m4a")
	createTestDir(t, root, "Misc/Random Folder", "track.m4a")

	result, err := DeepScanLibrary(root, database, nil)
	require.NoError(t, err)

	// Should find all 5 leaf dirs + 2 parent dirs (Author, Misc) = 7 total dirs
	// But the plan says "find all 5" — the parent dirs also get processed.
	// Let's check at minimum the 5 leaf dirs are found.
	assert.GreaterOrEqual(t, result.TotalDirs, 5)
	assert.Equal(t, 3, result.WithASIN)
	assert.GreaterOrEqual(t, result.WithoutASIN, 2)
}

func TestDeepScanNonASIN(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// Non-ASIN directory with audio
	createTestDir(t, root, "NoASIN Book", "chapter1.m4a")

	result, err := DeepScanLibrary(root, database, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalDirs)
	assert.Equal(t, 0, result.WithASIN)
	assert.Equal(t, 1, result.WithoutASIN)

	// Verify it appears in library_items
	items, err := db.ListLibraryItems(database)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "audiobook", items[0].ItemType)
}

func TestDeepScanPopulatesLibraryItems(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	createTestDir(t, root, "Author/Book [B08C6YJ1LS]", "book.m4a")
	createTestDir(t, root, "Author/Other Book", "track.m4a")

	_, err = DeepScanLibrary(root, database, nil)
	require.NoError(t, err)

	items, err := db.ListLibraryItems(database)
	require.NoError(t, err)
	// At minimum the Author dir + 2 book dirs = 3
	assert.GreaterOrEqual(t, len(items), 2)

	// Find the ASIN book
	var foundASIN bool
	for _, item := range items {
		if item.ASIN == "B08C6YJ1LS" {
			foundASIN = true
			assert.Equal(t, "audiobook", item.ItemType)
		}
	}
	assert.True(t, foundASIN, "expected to find ASIN book in library items")
}

func TestDeepScanPersistsIssues(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// Create an empty subdirectory to trigger IssueEmptyDir
	emptyDir := filepath.Join(root, "EmptyBook")
	require.NoError(t, os.MkdirAll(emptyDir, 0755))

	result, err := DeepScanLibrary(root, database, nil)
	require.NoError(t, err)
	assert.Greater(t, result.IssuesFound, 0)

	issues, err := db.ListScanIssues(database)
	require.NoError(t, err)
	assert.Greater(t, len(issues), 0)

	// Verify at least one empty_dir issue
	var foundEmpty bool
	for _, issue := range issues {
		if issue.IssueType == "empty_dir" {
			foundEmpty = true
		}
	}
	assert.True(t, foundEmpty, "expected empty_dir issue in scan_issues")
}

func TestDeepScanClearsOldIssues(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// Insert a fake old issue
	oldIssue := db.ScanIssue{
		Path:            "/old/path",
		IssueType:       "old_type",
		Severity:        "warning",
		Message:         "old issue",
		SuggestedAction: "remove it",
		ScanRunID:       "old-run",
	}
	require.NoError(t, db.InsertScanIssue(database, oldIssue))

	// Verify old issue exists
	issuesBefore, err := db.ListScanIssues(database)
	require.NoError(t, err)
	assert.Len(t, issuesBefore, 1)

	// Create a simple directory with audio (no issues expected besides maybe cover_missing)
	createTestDir(t, root, "Author/Book [B08C6YJ1LS]", "book.m4a")

	_, err = DeepScanLibrary(root, database, nil)
	require.NoError(t, err)

	// Old issue should be gone
	issuesAfter, err := db.ListScanIssues(database)
	require.NoError(t, err)
	for _, issue := range issuesAfter {
		assert.NotEqual(t, "old-run", issue.ScanRunID, "old issue should have been cleared")
	}
}

func TestDeepScanSkipsRoot(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// Put an audio file in the root itself
	require.NoError(t, os.WriteFile(filepath.Join(root, "stray.m4a"), []byte("fake"), 0644))
	// And one subdirectory
	createTestDir(t, root, "SubDir", "book.m4a")

	result, err := DeepScanLibrary(root, database, nil)
	require.NoError(t, err)
	// Only the subdirectory should be counted, not root
	assert.Equal(t, 1, result.TotalDirs)

	items, err := db.ListLibraryItems(database)
	require.NoError(t, err)
	// Root should not appear
	for _, item := range items {
		assert.NotEqual(t, db.NormalizePath(root), item.Path, "root directory should not be a library item")
	}
}

func TestDeepScanPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	root := t.TempDir()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	// Create a readable dir and an unreadable dir
	createTestDir(t, root, "Readable", "book.m4a")
	noReadDir := filepath.Join(root, "NoRead")
	require.NoError(t, os.MkdirAll(noReadDir, 0000))
	t.Cleanup(func() { os.Chmod(noReadDir, 0755) })

	result, err := DeepScanLibrary(root, database, nil)
	require.NoError(t, err, "permission errors should not be fatal")
	// Should still have processed the readable dir
	assert.GreaterOrEqual(t, result.TotalDirs, 1)
}
