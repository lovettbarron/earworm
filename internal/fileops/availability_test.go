package fileops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeAvailablePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("x"), 0o644))

	av := Probe(dir, time.Second)
	assert.True(t, av.Available)
	assert.False(t, av.TimedOut)
	assert.NoError(t, av.Error())
}

func TestProbeEmptyDirectoryIsAvailable(t *testing.T) {
	av := Probe(t.TempDir(), time.Second)
	assert.True(t, av.Available, "an empty directory is reachable, just empty")
}

func TestProbeMissingPath(t *testing.T) {
	av := Probe(filepath.Join(t.TempDir(), "nope"), time.Second)
	assert.False(t, av.Available)
	assert.False(t, av.TimedOut, "a missing path answers immediately; it is not a hang")
	assert.Error(t, av.Error())
}

func TestProbeEmptyPath(t *testing.T) {
	av := Probe("", time.Second)
	assert.False(t, av.Available)
	assert.Contains(t, av.Error().Error(), "no library path configured")
}

// The behaviour this exists for: a filesystem call that never returns must not
// hold the caller. A real dead SMB mount cannot be simulated, so the probe
// operation is replaced with one that blocks the same way.
func TestProbeAbandonsAHangingFilesystem(t *testing.T) {
	release := make(chan struct{})
	orig := statFunc
	statFunc = func(string) error {
		<-release // never returns until the test lets it
		return nil
	}
	t.Cleanup(func() { statFunc = orig; close(release) })

	start := time.Now()
	av := Probe("/some/hung/mount", 100*time.Millisecond)
	elapsed := time.Since(start)

	assert.False(t, av.Available)
	assert.True(t, av.TimedOut, "a hang must be reported as a timeout, not a plain error")
	assert.Less(t, elapsed, 2*time.Second, "the caller must not be held by the blocked call")
	assert.Contains(t, av.Error().Error(), "not answering")
}

func TestProbeTimeoutDefaultsWhenUnset(t *testing.T) {
	av := Probe(t.TempDir(), 0)
	assert.True(t, av.Available, "a zero timeout should fall back to the default, not fail instantly")
}

func TestEnsureAvailableSkipsRemountWhenHealthy(t *testing.T) {
	called := false
	orig := RemountFunc
	RemountFunc = func(context.Context, string) error { called = true; return nil }
	t.Cleanup(func() { RemountFunc = orig })

	av, attempted := EnsureAvailable(context.Background(), t.TempDir(),
		EnsureOptions{Timeout: time.Second, RemountCommand: "true"})

	assert.True(t, av.Available)
	assert.False(t, attempted)
	assert.False(t, called, "a healthy path must not be remounted")
}

func TestEnsureAvailableWithoutRemountCommand(t *testing.T) {
	av, attempted := EnsureAvailable(context.Background(),
		filepath.Join(t.TempDir(), "missing"), EnsureOptions{Timeout: time.Second})

	assert.False(t, av.Available)
	assert.False(t, attempted, "no command configured means no attempt")
}

// A remount command can exit zero without the path becoming usable, so success
// is measured by re-probing rather than by the exit code.
func TestEnsureAvailableRemountsAndReprobes(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mounted")

	orig := RemountFunc
	RemountFunc = func(context.Context, string) error {
		return os.MkdirAll(target, 0o755) // the "mount" appears
	}
	t.Cleanup(func() { RemountFunc = orig })

	av, attempted := EnsureAvailable(context.Background(), target,
		EnsureOptions{Timeout: time.Second, RemountCommand: "mount-it"})

	assert.True(t, attempted)
	assert.True(t, av.Available, "the re-probe should see the restored path")
}

func TestEnsureAvailableReportsFailedRemount(t *testing.T) {
	orig := RemountFunc
	RemountFunc = func(context.Context, string) error { return fmt.Errorf("mount: permission denied") }
	t.Cleanup(func() { RemountFunc = orig })

	av, attempted := EnsureAvailable(context.Background(),
		filepath.Join(t.TempDir(), "missing"),
		EnsureOptions{Timeout: time.Second, RemountCommand: "mount-it"})

	assert.True(t, attempted)
	assert.False(t, av.Available)
	assert.Contains(t, av.Error().Error(), "permission denied",
		"the remount failure is the actionable part and must reach the user")
}

// A remount that exits zero but leaves the path unusable must not be reported
// as success.
func TestEnsureAvailableRejectsSilentlyIneffectiveRemount(t *testing.T) {
	orig := RemountFunc
	RemountFunc = func(context.Context, string) error { return nil } // claims success, does nothing
	t.Cleanup(func() { RemountFunc = orig })

	av, attempted := EnsureAvailable(context.Background(),
		filepath.Join(t.TempDir(), "still-missing"),
		EnsureOptions{Timeout: time.Second, RemountCommand: "pretend"})

	assert.True(t, attempted)
	assert.False(t, av.Available, "exit code zero is not evidence the path works")
}

func TestRemountFuncRejectsEmptyCommand(t *testing.T) {
	assert.Error(t, RemountFunc(context.Background(), "   "))
}
