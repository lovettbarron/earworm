package audible

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Download downloads an audiobook by ASIN to the given output directory using audible-cli.
// It invokes audible-cli with flags for AAXC format, cover art, chapters, and best quality.
// If progressWriter is non-nil, stderr (which contains tqdm progress bars) is tee'd to it
// for real-time display. Stderr is always captured for error classification.
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

	// Read stderr in a goroutine — parse tqdm progress and capture for error classification.
	// tqdm writes \r-delimited lines like: "44%|████▍     | 437M/998M [01:02<01:17, 7.65MB/s]"
	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		reader := bufio.NewReader(stderrPipe)
		for {
			// tqdm uses \r for in-place updates, so read until \r or \n
			line, err := reader.ReadBytes('\r')
			if len(line) > 0 {
				stderrBuf.Write(line)
				if c.progressFunc != nil {
					if p, ok := parseTqdmProgress(line); ok {
						c.progressFunc(p)
					}
				}
			}
			if err != nil {
				// Drain any remaining bytes after \r scanning ends
				remaining, _ := io.ReadAll(reader)
				stderrBuf.Write(remaining)
				if c.progressFunc != nil && len(remaining) > 0 {
					if p, ok := parseTqdmProgress(remaining); ok {
						c.progressFunc(p)
					}
				}
				break
			}
		}
	}()

	// Drain stdout
	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
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

// tqdmPattern matches tqdm output like "44%|...| 437M/998M [01:02<01:17, 7.65MB/s]"
var tqdmPattern = regexp.MustCompile(`(\d+)%\|.*?(\d+[\.\d]*\w+/s)\]`)

// parseTqdmProgress extracts percent and rate from a tqdm progress line.
func parseTqdmProgress(line []byte) (DownloadProgress, bool) {
	m := tqdmPattern.FindSubmatch(bytes.TrimSpace(line))
	if m == nil {
		return DownloadProgress{}, false
	}
	pct, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return DownloadProgress{}, false
	}
	return DownloadProgress{
		Percent: pct,
		Rate:    string(m[2]),
	}, true
}
