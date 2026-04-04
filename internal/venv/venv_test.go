package venv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVenvDir(t *testing.T) {
	dir, err := VenvDir()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(dir, filepath.Join(".config", "earworm", "venv")),
		"VenvDir should end with .config/earworm/venv, got: %s", dir)
}

func TestAudibleCLIPath(t *testing.T) {
	path, err := AudibleCLIPath()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, filepath.Join("venv", "bin", "audible")),
		"AudibleCLIPath should end with venv/bin/audible, got: %s", path)
}

func TestEnsureAudibleCLI_FastPath(t *testing.T) {
	// Create a temp dir mimicking the venv structure with a fake executable
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))

	fakeBin := filepath.Join(binDir, "audible")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755))

	// Override venvDir for test
	origVenvDir := venvDirFunc
	venvDirFunc = func() (string, error) { return tmpDir, nil }
	defer func() { venvDirFunc = origVenvDir }()

	var buf bytes.Buffer
	path, err := EnsureAudibleCLI(context.Background(), &buf)
	require.NoError(t, err)
	assert.Equal(t, fakeBin, path)
	// Fast path should not print anything about creating venv or installing
	assert.Empty(t, buf.String(), "fast path should produce no output")
}

func TestEnsureAudibleCLI_NoPython(t *testing.T) {
	tmpDir := t.TempDir()

	// Override venvDir to point to a dir without audible binary
	origVenvDir := venvDirFunc
	venvDirFunc = func() (string, error) { return tmpDir, nil }
	defer func() { venvDirFunc = origVenvDir }()

	// Override lookPath to simulate python3 not found
	origLookPath := lookPathFunc
	lookPathFunc = func(file string) (string, error) {
		return "", &os.PathError{Op: "lookpath", Path: file, Err: os.ErrNotExist}
	}
	defer func() { lookPathFunc = origLookPath }()

	var buf bytes.Buffer
	_, err := EnsureAudibleCLI(context.Background(), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "python3")
}
