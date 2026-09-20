package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// dayoneTimeout bounds a single CLI invocation.
const dayoneTimeout = 60 * time.Second

// Writer delivers entries to a journal.
type Writer interface {
	// Write creates or updates one entry.
	Write(ctx context.Context, journalID string, e Entry) error
	// Flush pushes queued writes to the journal's backing store.
	Flush(ctx context.Context) error
}

// DayOneOption configures the Day One client.
type DayOneOption func(*DayOne)

// WithCmdFactory overrides how commands are constructed, for testing. It
// matches the seam used by the audible client so both are mocked the same way.
func WithCmdFactory(f func(ctx context.Context, name string, args ...string) *exec.Cmd) DayOneOption {
	return func(d *DayOne) { d.cmdFactory = f }
}

// DayOne writes entries through the `dayone` command-line tool.
//
// The CLI queues writes to a local outbox and only pushes them when `sync` is
// run, so Flush is a distinct, separately failable step rather than something
// Write can imply.
type DayOne struct {
	// Path is the dayone binary, usually just "dayone".
	Path string

	cmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewDayOne creates a Day One journal writer.
func NewDayOne(path string, opts ...DayOneOption) *DayOne {
	if path == "" {
		path = "dayone"
	}
	d := &DayOne{Path: path}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (d *DayOne) command(ctx context.Context, args ...string) *exec.Cmd {
	if d.cmdFactory != nil {
		return d.cmdFactory(ctx, d.Path, args...)
	}
	return exec.CommandContext(ctx, d.Path, args...)
}

// Write creates or updates one entry.
//
// The entry's deterministic ID is passed to the CLI, which treats a repeat as
// an update. That is what makes a re-run safe: without it, every sync would
// append another copy of the same day to the journal.
//
// The body is piped on stdin rather than passed as an argument, so entry text
// never appears in the process list.
func (d *DayOne) Write(ctx context.Context, journalID string, e Entry) error {
	if journalID == "" {
		return fmt.Errorf("journal id is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, dayoneTimeout)
	defer cancel()

	args := []string{
		"entry", "write",
		"--journal-id", journalID,
		"--entry-id", e.ID,
		"--date", e.Date.Format(time.RFC3339),
		"--body-stdin",
	}

	cmd := d.command(ctx, args...)
	cmd.Stdin = strings.NewReader(e.Body)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &CLIError{
			Op:     "entry write",
			Stderr: strings.TrimSpace(firstNonEmptyString(stderr.String(), stdout.String())),
			Err:    err,
		}
	}

	// The CLI reports failures as JSON with ok=false while still exiting zero.
	return checkOK("entry write", stdout.Bytes())
}

// Flush runs `dayone sync`, pushing the queued outbox to the journal.
//
// Kept separate from Write because it fails independently: entries can be
// written locally while the sync fails, and reporting that as a write failure
// would send someone looking in the wrong place.
func (d *DayOne) Flush(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, dayoneTimeout)
	defer cancel()

	cmd := d.command(ctx, "sync")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &CLIError{
			Op:     "sync",
			Stderr: strings.TrimSpace(firstNonEmptyString(stderr.String(), stdout.String())),
			Err:    err,
		}
	}
	return checkOK("sync", stdout.Bytes())
}

// Journal is one journal the account can write to.
type Journal struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Encryption string `json:"encryption"`
	State      string `json:"state"`
}

// ListJournals returns the journals available to the signed-in account.
func (d *DayOne) ListJournals(ctx context.Context) ([]Journal, error) {
	ctx, cancel := context.WithTimeout(ctx, dayoneTimeout)
	defer cancel()

	cmd := d.command(ctx, "list", "journals")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &CLIError{
			Op:     "list journals",
			Stderr: strings.TrimSpace(firstNonEmptyString(stderr.String(), stdout.String())),
			Err:    err,
		}
	}

	var resp struct {
		OK    bool      `json:"ok"`
		Items []Journal `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode journal list: %w", err)
	}
	return resp.Items, nil
}

// CLIError wraps a failed journal CLI invocation.
type CLIError struct {
	Op     string
	Stderr string
	Err    error
}

func (e *CLIError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("dayone %s failed: %s", e.Op, e.Stderr)
	}
	return fmt.Sprintf("dayone %s failed: %v", e.Op, e.Err)
}

func (e *CLIError) Unwrap() error { return e.Err }

// checkOK inspects a CLI response that may report failure in its body.
func checkOK(op string, out []byte) error {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		// Not a JSON response; the exit code is the only signal available and
		// it already indicated success.
		return nil
	}
	var resp struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return nil
	}
	if resp.OK != nil && !*resp.OK {
		return &CLIError{Op: op, Stderr: firstNonEmptyString(resp.Error, string(trimmed))}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
