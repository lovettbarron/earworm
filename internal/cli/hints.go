package cli

import (
	"fmt"
	"io"
)

// hint prints a next-step suggestion to stderr when not in quiet mode.
// Hints are guidance, not data, so they never go to stdout (preserving
// JSON output and pipe-friendliness).
func hint(w io.Writer, format string, args ...any) {
	if quiet {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(w, "\nNext: %s\n", msg)
}
