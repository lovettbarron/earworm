package audible

import (
	"bytes"
	"context"
	"fmt"
	"os"
)

// LibraryExport runs `audible library export --format json --output <tmpfile>`
// and parses the result. Uses a temp file because audible-cli's stdout output
// may include non-JSON progress text.
func (c *client) LibraryExport(ctx context.Context) ([]LibraryItem, error) {
	tmpFile, err := os.CreateTemp("", "earworm-library-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	args := c.buildArgs("library", "export", "--format", "json", "--output", tmpPath)
	cmd := c.command(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, classifyError("library export", stderr.String(), cmd.ProcessState.ExitCode(), err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read export file: %w", err)
	}
	return ParseLibraryExport(data)
}
