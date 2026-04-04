package download

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProgressTracker_FormatBookProgress(t *testing.T) {
	tests := []struct {
		name    string
		quiet   bool
		current int
		total   int
		author  string
		title   string
		asin    string
		pct     int
		want    string
	}{
		{
			name:    "basic format without percentage",
			quiet:   false,
			current: 3,
			total:   12,
			author:  "Author",
			title:   "Title",
			asin:    "B08ASIN",
			pct:     -1,
			want:    "[3/12] Downloading: Author - Title [B08ASIN]",
		},
		{
			name:    "format with percentage",
			quiet:   false,
			current: 3,
			total:   12,
			author:  "Author",
			title:   "Title",
			asin:    "B08ASIN",
			pct:     45,
			want:    "[3/12] Downloading: Author - Title [B08ASIN]... 45%",
		},
		{
			name:    "quiet mode returns empty string",
			quiet:   true,
			current: 3,
			total:   12,
			author:  "Author",
			title:   "Title",
			asin:    "B08ASIN",
			pct:     -1,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewProgressTracker(&buf, tt.quiet)
			got := p.FormatBookProgress(tt.current, tt.total, tt.author, tt.title, tt.asin, tt.pct)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProgressTracker_FormatSummary(t *testing.T) {
	tests := []struct {
		name      string
		quiet     bool
		succeeded int
		total     int
		failed    int
		elapsed   time.Duration
		want      string
	}{
		{
			name:      "with failures",
			quiet:     false,
			succeeded: 10,
			total:     12,
			failed:    2,
			elapsed:   47*time.Minute + 23*time.Second,
			want:      "Downloaded 10/12 books (2 failed, 47m 23s elapsed)",
		},
		{
			name:      "zero failed omits failed count",
			quiet:     false,
			succeeded: 12,
			total:     12,
			failed:    0,
			elapsed:   5*time.Minute + 10*time.Second,
			want:      "Downloaded 12/12 books (5m 10s elapsed)",
		},
		{
			name:      "quiet mode still returns summary",
			quiet:     true,
			succeeded: 10,
			total:     12,
			failed:    2,
			elapsed:   47*time.Minute + 23*time.Second,
			want:      "Downloaded 10/12 books (2 failed, 47m 23s elapsed)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewProgressTracker(&buf, tt.quiet)
			got := p.FormatSummary(tt.succeeded, tt.total, tt.failed, tt.elapsed)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProgressTracker_FormatResume(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressTracker(&buf, false)
	got := p.FormatResume(8, 4)
	assert.Equal(t, "Resuming: 8 of 12 remaining (4 completed previously)", got)
}

func TestProgressTracker_PrintBookProgress(t *testing.T) {
	t.Run("normal mode writes to writer", func(t *testing.T) {
		var buf bytes.Buffer
		p := NewProgressTracker(&buf, false)
		p.PrintBookProgress(1, 5, "Auth", "Book", "B123", -1)
		assert.Contains(t, buf.String(), "[1/5] Downloading: Auth - Book [B123]")
	})

	t.Run("quiet mode writes nothing", func(t *testing.T) {
		var buf bytes.Buffer
		p := NewProgressTracker(&buf, true)
		p.PrintBookProgress(1, 5, "Auth", "Book", "B123", -1)
		assert.Empty(t, buf.String())
	})
}

func TestProgressTracker_PrintSummary(t *testing.T) {
	t.Run("always prints even in quiet mode", func(t *testing.T) {
		var buf bytes.Buffer
		p := NewProgressTracker(&buf, true)
		p.PrintSummary(5, 5, 0, 2*time.Minute)
		assert.Contains(t, buf.String(), "Downloaded 5/5 books")
	})
}
