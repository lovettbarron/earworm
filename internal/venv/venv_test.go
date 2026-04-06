package venv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// TestHelperProcess is used by fakeVenvExecCommand to simulate subprocesses.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// If we need to create a file as a side effect (e.g., the audible binary)
	if createPath := os.Getenv("GO_HELPER_CREATE_FILE"); createPath != "" {
		os.MkdirAll(filepath.Dir(createPath), 0o755)
		os.WriteFile(createPath, []byte("#!/bin/sh\n"), 0o755)
	}
	fmt.Fprint(os.Stdout, os.Getenv("GO_HELPER_OUTPUT"))
	code, _ := strconv.Atoi(os.Getenv("GO_HELPER_EXIT_CODE"))
	os.Exit(code)
}

func fakeVenvExecCommand(output string, exitCode int, createFile string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		env := append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			fmt.Sprintf("GO_HELPER_OUTPUT=%s", output),
			fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", exitCode),
		)
		if createFile != "" {
			env = append(env, fmt.Sprintf("GO_HELPER_CREATE_FILE=%s", createFile))
		}
		cmd.Env = env
		return cmd
	}
}

func TestEnsureAudibleCLI_BootstrapSuccess(t *testing.T) {
	tmpBase := t.TempDir()
	// Use a subdir that does NOT exist yet so the code hits the venv creation path
	tmpDir := filepath.Join(tmpBase, "venv")

	origVenvDir := venvDirFunc
	origLookPath := lookPathFunc
	origExecCmd := execCommand
	defer func() {
		venvDirFunc = origVenvDir
		lookPathFunc = origLookPath
		execCommand = origExecCmd
	}()

	venvDirFunc = func() (string, error) { return tmpDir, nil }
	lookPathFunc = func(file string) (string, error) { return "/usr/bin/python3", nil }

	binPath := filepath.Join(tmpDir, "bin", "audible")
	// Use a command that creates the audible binary as a side effect
	execCommand = fakeVenvExecCommand("", 0, binPath)

	var buf bytes.Buffer
	path, err := EnsureAudibleCLI(context.Background(), &buf)
	require.NoError(t, err)
	assert.Equal(t, binPath, path)
	assert.Contains(t, buf.String(), "Creating Python venv")
	assert.Contains(t, buf.String(), "Installing audible-cli")
}

func TestEnsureAudibleCLI_VenvCreationFails(t *testing.T) {
	tmpBase := t.TempDir()
	// Use a subdir that does NOT exist so the code tries to create venv
	tmpDir := filepath.Join(tmpBase, "venv")

	origVenvDir := venvDirFunc
	origLookPath := lookPathFunc
	origExecCmd := execCommand
	defer func() {
		venvDirFunc = origVenvDir
		lookPathFunc = origLookPath
		execCommand = origExecCmd
	}()

	venvDirFunc = func() (string, error) { return tmpDir, nil }
	lookPathFunc = func(file string) (string, error) { return "/usr/bin/python3", nil }
	execCommand = fakeVenvExecCommand("error creating venv", 1, "")

	var buf bytes.Buffer
	_, err := EnsureAudibleCLI(context.Background(), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Python venv")

	// Verify cleanup happened -- the venv dir should not exist
	_, statErr := os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(statErr), "venv dir should be cleaned up after failure")
}

func TestEnsureAudibleCLI_PipInstallFails(t *testing.T) {
	tmpDir := t.TempDir()
	// Pre-create the venv dir so it skips venv creation
	binDir := filepath.Join(tmpDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	// Create pip so the path exists
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "pip"), []byte("#!/bin/sh\n"), 0o755))

	origVenvDir := venvDirFunc
	origLookPath := lookPathFunc
	origExecCmd := execCommand
	defer func() {
		venvDirFunc = origVenvDir
		lookPathFunc = origLookPath
		execCommand = origExecCmd
	}()

	venvDirFunc = func() (string, error) { return tmpDir, nil }
	lookPathFunc = func(file string) (string, error) { return "/usr/bin/python3", nil }
	execCommand = fakeVenvExecCommand("pip install failed", 1, "")

	var buf bytes.Buffer
	_, err := EnsureAudibleCLI(context.Background(), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install audible-cli")
}

func TestEnsureAudibleCLI_BinaryNotFoundAfterInstall(t *testing.T) {
	tmpDir := t.TempDir()
	// Pre-create the venv dir so it skips venv creation
	binDir := filepath.Join(tmpDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))

	origVenvDir := venvDirFunc
	origLookPath := lookPathFunc
	origExecCmd := execCommand
	defer func() {
		venvDirFunc = origVenvDir
		lookPathFunc = origLookPath
		execCommand = origExecCmd
	}()

	venvDirFunc = func() (string, error) { return tmpDir, nil }
	lookPathFunc = func(file string) (string, error) { return "/usr/bin/python3", nil }
	// Command succeeds but doesn't create the binary
	execCommand = fakeVenvExecCommand("", 0, "")

	var buf bytes.Buffer
	_, err := EnsureAudibleCLI(context.Background(), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audible-cli binary not found after installation")
}

func TestEnsureAudibleCLI_VenvDirError(t *testing.T) {
	origVenvDir := venvDirFunc
	defer func() { venvDirFunc = origVenvDir }()

	venvDirFunc = func() (string, error) { return "", fmt.Errorf("config error") }

	var buf bytes.Buffer
	_, err := EnsureAudibleCLI(context.Background(), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "determine venv dir")
}
