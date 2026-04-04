package venv

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lovettbarron/earworm/internal/config"
)

// execCommand is the command factory, overridable for tests.
var execCommand = exec.CommandContext

// lookPathFunc wraps exec.LookPath for test seam injection.
var lookPathFunc = exec.LookPath

// venvDirFunc wraps VenvDir for test seam injection.
var venvDirFunc = VenvDir

// VenvDir returns the path to the managed Python venv directory
// (~/.config/earworm/venv/).
func VenvDir() (string, error) {
	configDir, err := config.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("venv dir: %w", err)
	}
	return filepath.Join(configDir, "venv"), nil
}

// AudibleCLIPath returns the expected path to the audible binary inside the
// managed venv.
func AudibleCLIPath() (string, error) {
	dir, err := VenvDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bin", "audible"), nil
}

// EnsureAudibleCLI ensures that audible-cli is installed in the managed venv.
// If the binary already exists and is executable, it returns the path immediately
// (fast path). Otherwise it creates the venv and pip-installs audible-cli.
// Progress messages are written to w.
func EnsureAudibleCLI(ctx context.Context, w io.Writer) (string, error) {
	venvDir, err := venvDirFunc()
	if err != nil {
		return "", fmt.Errorf("determine venv dir: %w", err)
	}

	binPath := filepath.Join(venvDir, "bin", "audible")

	// Fast path: binary already exists and is executable.
	if info, err := os.Stat(binPath); err == nil {
		if info.Mode()&0o111 != 0 {
			return binPath, nil
		}
	}

	// Need to bootstrap. Check for python3.
	python3, err := lookPathFunc("python3")
	if err != nil {
		return "", fmt.Errorf("python3 not found on PATH; audible-cli requires Python 3: %w", err)
	}

	// Create venv if directory doesn't exist.
	if _, err := os.Stat(venvDir); os.IsNotExist(err) {
		fmt.Fprintln(w, "Creating Python venv...")
		cmd := execCommand(ctx, python3, "-m", "venv", venvDir)
		cmd.Stdout = w
		cmd.Stderr = w
		if err := cmd.Run(); err != nil {
			// Clean up partial venv on failure.
			os.RemoveAll(venvDir)
			return "", fmt.Errorf("failed to create Python venv: %w", err)
		}
	}

	// Install audible-cli via pip.
	pipPath := filepath.Join(venvDir, "bin", "pip")
	fmt.Fprintln(w, "Installing audible-cli...")
	cmd := execCommand(ctx, pipPath, "install", "--upgrade", "audible-cli")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to install audible-cli: %w", err)
	}

	// Verify installation.
	if _, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("audible-cli binary not found after installation at %s", binPath)
	}

	return binPath, nil
}
