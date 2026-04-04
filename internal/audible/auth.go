package audible

import (
	"bytes"
	"context"
	"os"
)

// Quickstart runs `audible quickstart` with stdin/stdout/stderr connected to terminal.
// Interactive mode: passes through terminal I/O for the audible-cli auth flow.
func (c *client) Quickstart(ctx context.Context) error {
	args := c.buildArgs("quickstart")
	cmd := c.command(ctx, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return classifyError("quickstart", "", cmd.ProcessState.ExitCode(), err)
	}
	return nil
}

// CheckAuth verifies auth is valid by running `audible library list --bunch-size 1`.
// This makes a real API call to verify the token works.
func (c *client) CheckAuth(ctx context.Context) error {
	args := c.buildArgs("library", "list", "--bunch-size", "1")
	cmd := c.command(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return classifyError("library list", stderr.String(), cmd.ProcessState.ExitCode(), err)
	}
	return nil
}
