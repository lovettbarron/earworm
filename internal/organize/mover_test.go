package organize

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveFile_SameFilesystem(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "dest.m4a")

	content := []byte("audiobook content here")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := MoveFile(src, dst)
	require.NoError(t, err)

	// Source should be gone
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source should not exist after move")

	// Destination should have same content
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestMoveFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "sub", "dir", "dest.m4a")

	content := []byte("audiobook content")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := MoveFile(src, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestMoveFile_CopyFallback(t *testing.T) {
	// Test copyVerifyDelete directly since we can't easily trigger EXDEV
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "copied.m4a")

	content := []byte("cross filesystem audiobook content")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := copyVerifyDelete(src, dst)
	require.NoError(t, err)

	// Source should be gone after successful copy+verify+delete
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source should be deleted after copy+verify+delete")

	// Destination should have same content
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestMoveFile_SizeVerification(t *testing.T) {
	// Test that copyVerifyDelete works with valid files of known sizes
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "dest.m4a")

	content := make([]byte, 1024) // 1KB file
	for i := range content {
		content[i] = byte(i % 256)
	}
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := copyVerifyDelete(src, dst)
	require.NoError(t, err)

	dstInfo, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, int64(1024), dstInfo.Size(), "destination size should match source")
}

func TestMoveFile_CleanupOnCopyFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nonexistent.m4a") // source doesn't exist
	dst := filepath.Join(dir, "dest.m4a")

	err := copyVerifyDelete(src, dst)
	require.Error(t, err)

	// Destination should not exist (cleaned up on failure)
	_, err = os.Stat(dst)
	assert.True(t, os.IsNotExist(err), "partial destination should be cleaned up on failure")
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "copy.m4a")

	content := []byte("test audiobook data for copy verification")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := copyFile(src, dst)
	require.NoError(t, err)

	// Both files should exist
	srcInfo, err := os.Stat(src)
	require.NoError(t, err)
	dstInfo, err := os.Stat(dst)
	require.NoError(t, err)

	assert.Equal(t, srcInfo.Size(), dstInfo.Size(), "file sizes should match")

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestCopyFile_UnreadableSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nonexistent.m4a")
	dst := filepath.Join(dir, "dest.m4a")

	err := copyFile(src, dst)
	require.Error(t, err)

	// Destination should not be created
	_, err = os.Stat(dst)
	assert.True(t, os.IsNotExist(err))
}

func TestCopyFile_Fsync(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "synced.m4a")

	content := []byte("audiobook data requiring fsync for NAS safety")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := copyFile(src, dst)
	require.NoError(t, err)

	// Verify destination exists and content matches (proves Sync()+Close() path works)
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestCopyVerifyDelete_SHA256(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "verified.m4a")

	content := []byte("audiobook content for SHA-256 verification")
	require.NoError(t, os.WriteFile(src, content, 0644))

	// Compute expected SHA-256
	h := sha256.New()
	h.Write(content)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	err := copyVerifyDelete(src, dst)
	require.NoError(t, err)

	// Source should be deleted
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source should be deleted after verified copy")

	// Destination should have correct SHA-256 hash
	dstData, err := os.ReadFile(dst)
	require.NoError(t, err)
	dstH := sha256.New()
	dstH.Write(dstData)
	actualHash := hex.EncodeToString(dstH.Sum(nil))
	assert.Equal(t, expectedHash, actualHash, "destination SHA-256 should match source")
}
