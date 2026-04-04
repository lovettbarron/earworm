package audible

import (
	"context"
	"os/exec"
)

// AudibleClient defines the interface for interacting with audible-cli.
type AudibleClient interface {
	// Quickstart runs interactive authentication (audible quickstart).
	Quickstart(ctx context.Context) error
	// CheckAuth verifies that authentication is valid by running a lightweight command.
	CheckAuth(ctx context.Context) error
	// LibraryExport exports the full Audible library as structured data.
	LibraryExport(ctx context.Context) ([]LibraryItem, error)
	// Download downloads a book by ASIN to the given output directory.
	Download(ctx context.Context, asin string, outputDir string) error
}

// ClientOption configures the audible client.
type ClientOption func(*client)

// WithProfilePath sets the audible-cli profile directory.
func WithProfilePath(path string) ClientOption {
	return func(c *client) { c.profilePath = path }
}

// WithCmdFactory overrides the command factory (used for testing).
func WithCmdFactory(f func(ctx context.Context, name string, args ...string) *exec.Cmd) ClientOption {
	return func(c *client) { c.cmdFactory = f }
}

// NewClient creates a new audible-cli wrapper client.
// audiblePath is the path to the audible-cli binary (e.g., "audible").
func NewClient(audiblePath string, opts ...ClientOption) AudibleClient {
	c := &client{
		audiblePath: audiblePath,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type client struct {
	audiblePath string
	profilePath string // optional, passed as --profile-dir flag if set
	cmdFactory  func(ctx context.Context, name string, args ...string) *exec.Cmd // for testing
}

// command builds an exec.Cmd using the command factory if set, otherwise os/exec.
func (c *client) command(ctx context.Context, args ...string) *exec.Cmd {
	if c.cmdFactory != nil {
		return c.cmdFactory(ctx, c.audiblePath, args...)
	}
	return exec.CommandContext(ctx, c.audiblePath, args...)
}

// profileArgs returns the --profile-dir flags if a profile path is configured.
func (c *client) profileArgs() []string {
	if c.profilePath != "" {
		return []string{"--profile-dir", c.profilePath}
	}
	return nil
}

// buildArgs prepends profile args to the given command args.
func (c *client) buildArgs(args ...string) []string {
	return append(c.profileArgs(), args...)
}
