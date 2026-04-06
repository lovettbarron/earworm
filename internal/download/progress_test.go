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

func TestFormatBookProgress(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, false)
	result := pt.FormatBookProgress(2, 5, "Author", "Title", "B001", 50)
	assert.Contains(t, result, "[2/5]")
	assert.Contains(t, result, "Author")
	assert.Contains(t, result, "Title")
	assert.Contains(t, result, "50%")
}

func TestFormatBookProgress_NoPct(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, false)
	result := pt.FormatBookProgress(1, 3, "Author", "Title", "B001", -1)
	assert.Contains(t, result, "Author")
	assert.NotContains(t, result, "%")
}

func TestFormatBookProgress_QuietMode(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, true)
	result := pt.FormatBookProgress(1, 3, "Author", "Title", "B001", 50)
	assert.Empty(t, result)
}

func TestFormatSummary(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, false)
	result := pt.FormatSummary(5, 6, 1, 10*time.Minute)
	assert.Contains(t, result, "5/6")
	assert.Contains(t, result, "1 failed")
}

func TestFormatSummary_NoFailures(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, false)
	result := pt.FormatSummary(3, 3, 0, 5*time.Minute)
	assert.Contains(t, result, "3/3")
	assert.NotContains(t, result, "failed")
}

func TestFormatResume(t *testing.T) {
	pt := NewProgressTracker(&bytes.Buffer{}, false)
	result := pt.FormatResume(5, 3)
	assert.Contains(t, result, "5 of 8 remaining")
	assert.Contains(t, result, "3 completed previously")
}

func TestPrintBookProgress(t *testing.T) {
	var buf bytes.Buffer
	pt := NewProgressTracker(&buf, false)
	pt.PrintBookProgress(1, 3, "Author", "Title", "B001", 75)
	assert.NotEmpty(t, buf.String())
}

func TestPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	pt := NewProgressTracker(&buf, false)
	pt.PrintSummary(3, 4, 1, 5*time.Minute)
	assert.NotEmpty(t, buf.String())
}
