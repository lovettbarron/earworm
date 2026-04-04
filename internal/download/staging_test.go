package download

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyM4A_NonExistentFile(t *testing.T) {
	err := VerifyM4A("/nonexistent/file.m4a")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "opening file")
}

func TestVerifyM4A_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.m4a")
	require.NoError(t, os.WriteFile(emptyFile, []byte{}, 0644))

	err := VerifyM4A(emptyFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "zero size")
}

func TestVerifyM4A_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.m4a")
	require.NoError(t, os.WriteFile(badFile, []byte("not an m4a file at all"), 0644))

	err := VerifyM4A(badFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading metadata")
}

// TestVerifyM4A_ValidFile tests with a minimal valid M4A.
// Since creating a real M4A fixture is complex, we test the error paths above
// and trust dhowden/tag for the happy path. Integration tests can use real files.

func TestMoveToLibrary(t *testing.T) {
	t.Run("moves files to destination creating dirs", func(t *testing.T) {
		staging := t.TempDir()
		library := t.TempDir()

		asin := "B001234567"
		stagingASIN := filepath.Join(staging, asin)
		require.NoError(t, os.MkdirAll(stagingASIN, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(stagingASIN, "book.m4a"), []byte("audio data"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(stagingASIN, "cover.jpg"), []byte("image data"), 0644))

		err := MoveToLibrary(staging, library, asin)
		require.NoError(t, err)

		// Verify files exist in library
		assert.FileExists(t, filepath.Join(library, asin, "book.m4a"))
		assert.FileExists(t, filepath.Join(library, asin, "cover.jpg"))

		// Verify staging directory was removed
		assert.NoDirExists(t, stagingASIN)
	})

	t.Run("handles destination already exists (overwrite)", func(t *testing.T) {
		staging := t.TempDir()
		library := t.TempDir()

		asin := "B001234567"
		stagingASIN := filepath.Join(staging, asin)
		require.NoError(t, os.MkdirAll(stagingASIN, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(stagingASIN, "book.m4a"), []byte("new audio"), 0644))

		// Pre-create destination with old file
		libASIN := filepath.Join(library, asin)
		require.NoError(t, os.MkdirAll(libASIN, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(libASIN, "book.m4a"), []byte("old audio"), 0644))

		err := MoveToLibrary(staging, library, asin)
		require.NoError(t, err)

		// Verify new content replaced old
		data, err := os.ReadFile(filepath.Join(library, asin, "book.m4a"))
		require.NoError(t, err)
		assert.Equal(t, "new audio", string(data))
	})
}

func TestCleanOrphans(t *testing.T) {
	t.Run("removes orphaned ASIN directories", func(t *testing.T) {
		staging := t.TempDir()

		// Create some ASIN-like directories
		orphan := filepath.Join(staging, "B001234567")
		require.NoError(t, os.MkdirAll(orphan, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(orphan, "partial.m4a"), []byte("partial"), 0644))

		kept := filepath.Join(staging, "B009876543")
		require.NoError(t, os.MkdirAll(kept, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(kept, "book.m4a"), []byte("complete"), 0644))

		downloadedASINs := map[string]bool{
			"B009876543": true,
		}

		err := CleanOrphans(staging, downloadedASINs)
		require.NoError(t, err)

		// Orphan removed
		assert.NoDirExists(t, orphan)
		// Downloaded kept
		assert.DirExists(t, kept)
	})

	t.Run("does not remove non-ASIN directories", func(t *testing.T) {
		staging := t.TempDir()

		// Non-ASIN directory (not 10 alphanumeric chars)
		nonASIN := filepath.Join(staging, "temp-work")
		require.NoError(t, os.MkdirAll(nonASIN, 0755))

		// Short name (not ASIN-like)
		shortDir := filepath.Join(staging, "abc")
		require.NoError(t, os.MkdirAll(shortDir, 0755))

		downloadedASINs := map[string]bool{}

		err := CleanOrphans(staging, downloadedASINs)
		require.NoError(t, err)

		// Non-ASIN dirs preserved
		assert.DirExists(t, nonASIN)
		assert.DirExists(t, shortDir)
	})

	t.Run("does not remove staging root", func(t *testing.T) {
		staging := t.TempDir()
		downloadedASINs := map[string]bool{}

		err := CleanOrphans(staging, downloadedASINs)
		require.NoError(t, err)

		assert.DirExists(t, staging)
	})
}
