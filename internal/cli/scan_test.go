package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestConfig creates a temp config file with the given library_path and returns
// the config file path. It also sets HOME to a temp dir so DBPath resolves there.
func writeTestConfig(t *testing.T, libPath string) string {
	t.Helper()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Create config dir
	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := "library_path: " + libPath + "\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))

	return cfgPath
}

// createTestLibrary creates a Libation-compatible library structure in a temp dir.
// Returns the library root path.
func createTestLibrary(t *testing.T) string {
	t.Helper()
	tmpLib := t.TempDir()

	// Author Name/Book Title [B08C6YJ1LS]/book.m4a
	bookDir := filepath.Join(tmpLib, "Test Author", "Great Book [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), []byte("fake"), 0644))

	// Second book
	bookDir2 := filepath.Join(tmpLib, "Another Writer", "Second Book [B09ABCDEF1]")
	require.NoError(t, os.MkdirAll(bookDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir2, "part1.m4a"), []byte("fake"), 0644))

	return tmpLib
}

func TestScanNoLibraryPath(t *testing.T) {
	// Set HOME to temp so no existing config interferes
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	_, err := executeCommand(t, "scan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library path not configured")
}

func TestScanNonexistentPath(t *testing.T) {
	cfgPath := writeTestConfig(t, "/nonexistent/path/that/does/not/exist")

	_, err := executeCommand(t, "--config", cfgPath, "scan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestScanValidLibrary(t *testing.T) {
	tmpLib := createTestLibrary(t)
	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan")
	require.NoError(t, err)
	assert.Contains(t, out, "Scan complete")
	assert.Contains(t, out, "Added:")
	// Should have found 2 books
	assert.Contains(t, out, "Added:   2")
}

func TestScanRecursive(t *testing.T) {
	tmpLib := t.TempDir()

	// Create a deeply nested structure
	deepDir := filepath.Join(tmpLib, "Level1", "Level2", "Deep Title [B09ABCDEF1]")
	require.NoError(t, os.MkdirAll(deepDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(deepDir, "book.m4a"), []byte("fake"), 0644))

	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan", "--recursive")
	require.NoError(t, err)
	assert.Contains(t, out, "Scan complete")
	assert.Contains(t, out, "Added:")
	// The recursive scan should find the deep book
	assert.True(t, strings.Contains(out, "Added:   1"), "expected 1 book added, got: %s", out)
}

func TestScanRescan(t *testing.T) {
	tmpLib := createTestLibrary(t)
	cfgPath := writeTestConfig(t, tmpLib)

	// First scan
	out1, err := executeCommand(t, "--config", cfgPath, "scan")
	require.NoError(t, err)
	assert.Contains(t, out1, "Added:   2")

	// Second scan -- same books should be updated, not added again
	out2, err := executeCommand(t, "--config", cfgPath, "scan")
	require.NoError(t, err)
	assert.Contains(t, out2, "Scan complete")
	assert.Contains(t, out2, "Updated:")
	// No new additions on rescan
	assert.Contains(t, out2, "Added:   0")
	assert.Contains(t, out2, "Updated: 2")
}

func TestScanCommand(t *testing.T) {
	// Verify scan command is registered with expected flags
	out, err := executeCommand(t, "scan", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--recursive")
	assert.Contains(t, out, "-r")
}
