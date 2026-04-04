package audible

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Download downloads an audiobook by ASIN to the given output directory using audible-cli.
// It invokes audible-cli with flags for AAXC format, cover art, chapters, and best quality.
// Stderr is captured for error classification. Stdout is drained line-by-line for future
// progress parsing support.
func (c *client) Download(ctx context.Context, asin string, outputDir string) error {
	args := c.buildArgs(
		"download",
		"--asin", asin,
		"--aaxc",
		"--cover",
		"--cover-size", "500",
		"--chapter",
		"--output-dir", outputDir,
		"--no-confirm",
		"--quality", "best",
	)

	cmd := c.command(ctx, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start download command: %w", err)
	}

	// Read stderr in a goroutine to avoid deadlock
	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		io.Copy(&stderrBuf, stderrPipe)
		close(stderrDone)
	}()

	// Drain stdout line-by-line (for future progress parsing)
	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		// Currently just drain; future: parse progress
	}

	// Wait for stderr goroutine to finish before calling Wait
	<-stderrDone

	// Wait for the command to complete (after pipes are fully drained)
	if err := cmd.Wait(); err != nil {
		exitCode := 1
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		}
		return classifyError("download", stderrBuf.String(), exitCode, err)
	}

	return nil
}
