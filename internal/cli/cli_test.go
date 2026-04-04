package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })

	// Reset package-level flag variables to prevent cross-test contamination.
	// Cobra flags bind to package vars which persist across tests.
	cfgFile = ""
	quiet = false
	jsonOutput = false
	filterAuthor = ""
	filterStatus = ""
	scanRecursive = false
	syncJSON = false
	dryRun = false
	downloadJSON = false

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestRootHelp(t *testing.T) {
	out, err := executeCommand(t, "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "earworm")
	assert.Contains(t, out, "audiobook")
}

func TestVersionCommand(t *testing.T) {
	out, err := executeCommand(t, "version")
	assert.NoError(t, err)
	assert.Contains(t, out, "dev")
}

func TestConfigShowCommand(t *testing.T) {
	out, err := executeCommand(t, "config", "show")
	assert.NoError(t, err)
	assert.Contains(t, out, "library_path")
}

func TestConfigInitCommand(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	out, err := executeCommand(t, "config", "init")
	assert.NoError(t, err)
	assert.Contains(t, out, "Configuration file created at")

	// Config file should exist.
	cfgPath := filepath.Join(tmpDir, ".config", "earworm", "config.yaml")
	_, statErr := os.Stat(cfgPath)
	assert.NoError(t, statErr)

	// Running again should not overwrite.
	out2, err := executeCommand(t, "config", "init")
	assert.NoError(t, err)
	assert.Contains(t, out2, "already exists")
}

func TestConfigSetValidKey(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	out, err := executeCommand(t, "config", "set", "library_path", "/tmp/test")
	assert.NoError(t, err)
	assert.Contains(t, out, "Set library_path = /tmp/test")
}

func TestConfigSetInvalidKey(t *testing.T) {
	_, err := executeCommand(t, "config", "set", "nonexistent_key", "value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key")
}
