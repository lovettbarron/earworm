package fileops

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello world\n")
	require.NoError(t, os.WriteFile(path, content, 0644))

	// Compute expected hash
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	got, err := HashFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestHashFile_NotFound(t *testing.T) {
	_, err := HashFile("/nonexistent/path/file.txt")
	require.Error(t, err)
}

func TestVerifiedMove_SameFS(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "dest.m4a")

	content := []byte("audiobook content for verified move")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := VerifiedMove(src, dst)
	require.NoError(t, err)

	// Source should be gone
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source should not exist after move")

	// Destination should have same content
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestVerifiedMove_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "sub", "deep", "dest.m4a")

	content := []byte("audiobook with parent dir creation")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := VerifiedMove(src, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	// Source should be gone
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err))
}

func TestVerifiedCopy_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "dest.m4a")

	content := []byte("audiobook content for verified copy")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := VerifiedCopy(src, dst)
	require.NoError(t, err)

	// Source should still exist
	_, err = os.Stat(src)
	assert.NoError(t, err, "source should still exist after copy")

	// Destination should have same content
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	// SHA-256 should match
	srcHash, _ := HashFile(src)
	dstHash, _ := HashFile(dst)
	assert.Equal(t, srcHash, dstHash)
}

func TestVerifiedCopy_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "sub", "deep", "dest.m4a")

	content := []byte("audiobook with parent dir creation for copy")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := VerifiedCopy(src, dst)
	require.NoError(t, err)

	// Source still exists
	_, err = os.Stat(src)
	assert.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestVerifiedCopy_SourceNotFound(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nonexistent.m4a")
	dst := filepath.Join(dir, "dest.m4a")

	err := VerifiedCopy(src, dst)
	require.Error(t, err)
}

func TestVerifiedMove_SourceNotFound(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nonexistent.m4a")
	dst := filepath.Join(dir, "dest.m4a")

	err := VerifiedMove(src, dst)
	require.Error(t, err)

	// Destination should not be created
	_, err = os.Stat(dst)
	assert.True(t, os.IsNotExist(err))
}
