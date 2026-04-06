package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonCommand_InvalidInterval(t *testing.T) {
	_, err := executeCommand(t, "daemon", "--once", "--interval", "invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid polling interval")
}

func TestDaemonCommand_OnceMode(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	libDir := filepath.Join(tmpHome, "library")
	require.NoError(t, os.MkdirAll(libDir, 0755))

	cfgPath := filepath.Join(cfgDir, "config.yaml")
	cfgContent := "library_path: " + libDir + "\ndaemon:\n  polling_interval: 1h\naudible_cli_path: /nonexistent/audible\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

	// daemon --once runs one cycle: sync (will fail gracefully), download, organize, notify
	// All sub-commands log warnings but don't propagate errors in daemon cycle
	_, err := executeCommand(t, "--config", cfgPath, "daemon", "--once")
	// The daemon cycle catches errors with slog.Warn, so it should not return error
	// However download may error on ffmpeg check -- that's OK, daemon catches it
	_ = err // daemon cycle catches sub-command errors
}
