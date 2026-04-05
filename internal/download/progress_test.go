package download

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatElapsed(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, false)

	tests := []struct {
		name     string
		elapsed  time.Duration
		expected string
	}{
		{
			name:     "10 seconds",
			elapsed:  10 * time.Second,
			expected: "[1/3] Downloading: Author A - Book One [B000000001]... 10s",
		},
		{
			name:     "25 seconds",
			elapsed:  25 * time.Second,
			expected: "[1/3] Downloading: Author A - Book One [B000000001]... 25s",
		},
		{
			name:     "1 minute 30 seconds",
			elapsed:  90 * time.Second,
			expected: "[1/3] Downloading: Author A - Book One [B000000001]... 1m 30s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pt.FormatElapsed(1, 3, "Author A", "Book One", "B000000001", tt.elapsed)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatElapsed_QuietMode(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, true)
	result := pt.FormatElapsed(1, 3, "Author A", "Book One", "B000000001", 10*time.Second)
	assert.Empty(t, result, "quiet mode should produce no output")
}

func TestPrintElapsed(t *testing.T) {
	var buf bytes.Buffer
	pt := NewProgressTracker(&buf, false)

	pt.PrintElapsed(1, 3, "Author A", "Book One", "B000000001", 10*time.Second)

	output := buf.String()
	assert.Contains(t, output, "Downloading: Author A - Book One [B000000001]... 10s")
	assert.Contains(t, output, "\r")
}

func TestPrintElapsed_QuietMode(t *testing.T) {
	var buf bytes.Buffer
	pt := NewProgressTracker(&buf, true)

	pt.PrintElapsed(1, 3, "Author A", "Book One", "B000000001", 10*time.Second)

	assert.Empty(t, buf.String(), "quiet mode should produce no output")
}
