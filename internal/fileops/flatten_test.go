package fileops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestFlattenDir_MovesNestedFiles(t *testing.T) {
	bookDir := t.TempDir()
	createFile(t, filepath.Join(bookDir, "sub", "track1.m4a"), "audio1")
	createFile(t, filepath.Join(bookDir, "sub", "track2.m4a"), "audio2")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Len(t, result.FilesMoved, 2)

	for _, fm := range result.FilesMoved {
		assert.True(t, fm.Success, "file move should succeed: %s", fm.SourcePath)
	}

	// Files should be at root
	assert.FileExists(t, filepath.Join(bookDir, "track1.m4a"))
	assert.FileExists(t, filepath.Join(bookDir, "track2.m4a"))

	// Subdir should be removed
	_, err = os.Stat(filepath.Join(bookDir, "sub"))
	assert.True(t, os.IsNotExist(err), "empty subdir should be removed")
}

func TestFlattenDir_SkipsRootFiles(t *testing.T) {
	bookDir := t.TempDir()
	createFile(t, filepath.Join(bookDir, "root.m4a"), "root audio")
	createFile(t, filepath.Join(bookDir, "sub", "nested.m4a"), "nested audio")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Len(t, result.FilesMoved, 1, "only nested file should be moved")

	// Root file should still be there
	assert.FileExists(t, filepath.Join(bookDir, "root.m4a"))
	// Nested file should now be at root
	assert.FileExists(t, filepath.Join(bookDir, "nested.m4a"))
}

func TestFlattenDir_HandlesNameCollision(t *testing.T) {
	bookDir := t.TempDir()
	createFile(t, filepath.Join(bookDir, "sub1", "track.m4a"), "audio from sub1")
	createFile(t, filepath.Join(bookDir, "sub2", "track.m4a"), "audio from sub2")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Len(t, result.FilesMoved, 2)

	for _, fm := range result.FilesMoved {
		assert.True(t, fm.Success, "move should succeed: %s -> %s", fm.SourcePath, fm.DestPath)
	}

	// Both files should be at root with unique names
	assert.FileExists(t, filepath.Join(bookDir, "track.m4a"))
	assert.FileExists(t, filepath.Join(bookDir, "track_1.m4a"))
}

func TestFlattenDir_CleansEmptyDirs(t *testing.T) {
	bookDir := t.TempDir()
	createFile(t, filepath.Join(bookDir, "nested", "deep", "subdir", "track.m4a"), "deep audio")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Len(t, result.FilesMoved, 1)

	// File at root
	assert.FileExists(t, filepath.Join(bookDir, "track.m4a"))

	// All 3 levels of empty subdirs should be removed
	for _, sub := range []string{"nested/deep/subdir", "nested/deep", "nested"} {
		_, err := os.Stat(filepath.Join(bookDir, sub))
		assert.True(t, os.IsNotExist(err), "empty dir %s should be removed", sub)
	}

	assert.GreaterOrEqual(t, len(result.DirsRemoved), 3)
}

func TestFlattenDir_MovesMP3Files(t *testing.T) {
	bookDir := t.TempDir()
	createFile(t, filepath.Join(bookDir, "sub", "chapter01.mp3"), "mp3audio")
	createFile(t, filepath.Join(bookDir, "sub", "chapter02.mp3"), "mp3audio2")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Len(t, result.FilesMoved, 2)

	assert.FileExists(t, filepath.Join(bookDir, "chapter01.mp3"))
	assert.FileExists(t, filepath.Join(bookDir, "chapter02.mp3"))
}

func TestFlattenDir_IgnoresNonAudioFiles(t *testing.T) {
	bookDir := t.TempDir()
	createFile(t, filepath.Join(bookDir, "sub", "notes.txt"), "not audio")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Empty(t, result.FilesMoved, "non-audio files should not be moved")

	// txt file should remain where it was
	assert.FileExists(t, filepath.Join(bookDir, "sub", "notes.txt"))
}

func TestFlattenDir_EmptyDir(t *testing.T) {
	bookDir := t.TempDir()

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Empty(t, result.FilesMoved)
}

func TestFlattenDir_DeeplyNested(t *testing.T) {
	bookDir := t.TempDir()
	createFile(t, filepath.Join(bookDir, "a", "b", "c", "track.m4b"), "deep m4b audio")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)
	assert.Len(t, result.FilesMoved, 1)
	assert.True(t, result.FilesMoved[0].Success)

	assert.FileExists(t, filepath.Join(bookDir, "track.m4b"))
}

func TestFlattenDir_SkipsCleanupOnError(t *testing.T) {
	bookDir := t.TempDir()
	subdir := filepath.Join(bookDir, "sub")
	createFile(t, filepath.Join(subdir, "track1.m4a"), "audio1")
	createFile(t, filepath.Join(subdir, "track2.m4a"), "audio2")

	// Make one file unreadable so VerifiedMove fails
	unreadable := filepath.Join(subdir, "track2.m4a")
	require.NoError(t, os.Chmod(unreadable, 0000))
	t.Cleanup(func() { os.Chmod(unreadable, 0644) })

	result, err := FlattenDir(bookDir)
	require.NoError(t, err) // FlattenDir itself doesn't error, it records per-file errors

	// Should have at least one error
	assert.NotEmpty(t, result.Errors, "should have errors from unreadable file")

	// Cleanup should be skipped -- DirsRemoved should be empty
	assert.Empty(t, result.DirsRemoved, "should skip directory cleanup when errors occurred")

	// Subdirectory should still exist
	_, err = os.Stat(subdir)
	assert.NoError(t, err, "subdirectory should be preserved when errors occurred")
}

func TestFlattenDir_CleansUpOnSuccess(t *testing.T) {
	bookDir := t.TempDir()
	subdir := filepath.Join(bookDir, "sub")
	createFile(t, filepath.Join(subdir, "track.m4a"), "audio content")

	result, err := FlattenDir(bookDir)
	require.NoError(t, err)

	// No errors
	assert.Empty(t, result.Errors, "should have no errors")

	// Cleanup should have run
	assert.NotEmpty(t, result.DirsRemoved, "should remove empty dirs on success")

	// File should be at root
	assert.FileExists(t, filepath.Join(bookDir, "track.m4a"))
}
