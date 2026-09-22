package fileops

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultProbeTimeout is how long a availability probe waits before declaring
// a path unreachable.
//
// A healthy local or LAN mount answers a stat in microseconds. Ten seconds is
// far beyond that while staying short enough that a caller is not left waiting.
const DefaultProbeTimeout = 10 * time.Second

// Availability is the result of probing a path.
type Availability struct {
	Path string
	// Available is true only when the probe returned successfully in time.
	Available bool
	// TimedOut distinguishes a hung mount from one that answered with an
	// error. A hung mount is the dangerous case: the syscall is still running.
	TimedOut bool
	Reason   string
	Elapsed  time.Duration
}

// Error renders the failure for a caller to surface.
func (a Availability) Error() error {
	if a.Available {
		return nil
	}
	if a.TimedOut {
		return fmt.Errorf("library path %s did not respond within %s; "+
			"the mount is present but not answering", a.Path, a.Elapsed.Round(time.Second))
	}
	return fmt.Errorf("library path %s is unavailable: %s", a.Path, a.Reason)
}

// statFunc is the probe operation, replaceable in tests.
//
// Opening the directory rather than stat-ing it is deliberate: a stale SMB
// mount can answer a stat from cached metadata while any real access hangs, so
// a stat would report a dead mount as healthy.
var statFunc = func(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// One entry is enough to force an actual round trip to the server.
	_, err = f.Readdirnames(1)
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

// Probe reports whether a path can be reached within the timeout.
//
// The stat runs on its own goroutine and is ABANDONED if it does not return in
// time. This is deliberate and it leaks: a syscall blocked on a dead network
// mount cannot be cancelled, not by context, not by closing anything. The
// choice is between leaking one goroutine and blocking the caller forever, and
// for a daemon that also has unrelated work to do, leaking is the lesser harm.
//
// The abandoned goroutine ends by itself if the mount ever recovers. Callers
// that probe repeatedly should stop calling after a failure rather than
// stacking up blocked goroutines — see ShouldSkip.
func Probe(path string, timeout time.Duration) Availability {
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	if path == "" {
		return Availability{Path: path, Reason: "no library path configured"}
	}

	start := time.Now()
	// Buffered so the abandoned goroutine can always finish its send and exit
	// rather than blocking on an unread channel forever.
	done := make(chan error, 1)
	go func() { done <- statFunc(path) }()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		if err != nil {
			return Availability{Path: path, Elapsed: elapsed, Reason: err.Error()}
		}
		return Availability{Path: path, Available: true, Elapsed: elapsed}
	case <-time.After(timeout):
		return Availability{
			Path: path, TimedOut: true, Elapsed: timeout,
			Reason: "timed out waiting for the filesystem to respond",
		}
	}
}

// RemountFunc runs a shell command to restore a mount.
var RemountFunc = func(ctx context.Context, command string) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("no remount command configured")
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("remount command failed: %s", msg)
	}
	return nil
}

// EnsureOptions configures EnsureAvailable.
type EnsureOptions struct {
	// Timeout bounds each probe.
	Timeout time.Duration
	// RemountCommand is run when the path is unavailable. Empty disables the
	// attempt, which is the default: remounting needs credentials and a
	// command specific to the host, and guessing at one risks mounting the
	// wrong thing over the right place.
	RemountCommand string
	// RemountTimeout bounds the remount command.
	RemountTimeout time.Duration
}

// EnsureAvailable probes a path and, if configured, tries once to remount it.
//
// One attempt only. A remount that failed will keep failing while the
// underlying cause persists, and retrying inside a poll cycle just delays the
// report without changing it.
func EnsureAvailable(ctx context.Context, path string, opts EnsureOptions) (Availability, bool) {
	av := Probe(path, opts.Timeout)
	if av.Available {
		return av, false
	}
	if strings.TrimSpace(opts.RemountCommand) == "" {
		return av, false
	}

	remountTimeout := opts.RemountTimeout
	if remountTimeout <= 0 {
		remountTimeout = 60 * time.Second
	}
	rctx, cancel := context.WithTimeout(ctx, remountTimeout)
	defer cancel()

	if err := RemountFunc(rctx, opts.RemountCommand); err != nil {
		av.Reason = fmt.Sprintf("%s; remount attempt failed: %v", av.Reason, err)
		return av, true
	}

	// Re-probe: a remount command can exit zero without the path becoming
	// usable, so success is measured by the path answering, not by the exit code.
	return Probe(path, opts.Timeout), true
}
