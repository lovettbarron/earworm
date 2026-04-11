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

func TestVerifiedCopy_Fsync(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.m4a")
	dst := filepath.Join(dir, "copy.m4a")

	content := []byte("audiobook data requiring fsync in VerifiedCopy")
	require.NoError(t, os.WriteFile(src, content, 0644))

	err := VerifiedCopy(src, dst)
	require.NoError(t, err)

	// Verify destination content matches
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	// Verify SHA-256 hash matches
	h := sha256.New()
	h.Write(content)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	dstHash, err := HashFile(dst)
	require.NoError(t, err)
	assert.Equal(t, expectedHash, dstHash)

	// Source should still exist (VerifiedCopy is copy, not move)
	assert.FileExists(t, src)
}
