package fileops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFreeSpace_ValidPath(t *testing.T) {
	tmpDir := t.TempDir()
	space, err := FreeSpace(tmpDir)
	require.NoError(t, err)
	assert.True(t, space > 0, "free space should be positive, got %d", space)
}

func TestFreeSpace_InvalidPath(t *testing.T) {
	_, err := FreeSpace("/nonexistent/path/that/does/not/exist")
	assert.Error(t, err)
}

func TestCheckFreeSpace_Sufficient(t *testing.T) {
	tmpDir := t.TempDir()
	// Request 1 byte -- should always be available
	err := CheckFreeSpace(tmpDir, 1)
	assert.NoError(t, err)
}

func TestCheckFreeSpace_Insufficient(t *testing.T) {
	tmpDir := t.TempDir()
	// Request an impossibly large amount
	err := CheckFreeSpace(tmpDir, ^uint64(0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient disk space")
}

func TestCheckFreeSpace_InvalidPath(t *testing.T) {
	err := CheckFreeSpace("/nonexistent/path", 1)
	assert.Error(t, err)
}
