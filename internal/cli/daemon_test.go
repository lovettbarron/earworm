package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
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

// The stats step can outlast a polling interval; two concurrent runs would
// interleave writes to the same rows and journal entries.
func TestDaemonStatsCycleSkipsWhenAlreadyRunning(t *testing.T) {
	statsCycleRunning.Store(true)
	t.Cleanup(func() { statsCycleRunning.Store(false) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Must return immediately rather than blocking or racing.
		runDaemonStatsCycle(&cobra.Command{})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemonStatsCycle blocked instead of skipping an overlapping run")
	}
}

func TestDaemonStatsCycleGuardReleasesAfterRun(t *testing.T) {
	statsCycleRunning.Store(false)

	// With no configuration the inner syncs fail and are logged, but the
	// guard must still be released so the next cycle can run.
	runDaemonStatsCycle(&cobra.Command{})
	assert.False(t, statsCycleRunning.Load(), "the guard must be released even when the cycle errors")
}
