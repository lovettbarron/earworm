package download

import (
	"fmt"
	"io"
	"time"
)

// ProgressTracker formats download progress messages per the D-01/D-03/D-04
// compact status line and summary conventions.
type ProgressTracker struct {
	quiet bool
	w     io.Writer
}

// NewProgressTracker creates a progress tracker that writes to w.
// In quiet mode, per-book progress is suppressed but summaries still print (D-03, D-04).
func NewProgressTracker(w io.Writer, quiet bool) *ProgressTracker {
	return &ProgressTracker{quiet: quiet, w: w}
}

// FormatBookProgress returns the compact status line per D-01.
// Format: "[current/total] Downloading: Author - Title [ASIN]"
// If pct >= 0, appends "... {pct}%".
// In quiet mode, returns empty string (D-03).
func (p *ProgressTracker) FormatBookProgress(current, total int, author, title, asin string, pct int) string {
	if p.quiet {
		return ""
	}
	s := fmt.Sprintf("[%d/%d] Downloading: %s - %s [%s]", current, total, author, title, asin)
	if pct >= 0 {
		s += fmt.Sprintf("... %d%%", pct)
	}
	return s
}

// FormatSummary returns completion summary per D-04.
// Format: "Downloaded X/Y books (Z failed, Nm Ns elapsed)"
// Always returns non-empty even in quiet mode.
func (p *ProgressTracker) FormatSummary(succeeded, total, failed int, elapsed time.Duration) string {
	elapsedStr := formatDuration(elapsed)
	if failed > 0 {
		return fmt.Sprintf("Downloaded %d/%d books (%d failed, %s elapsed)", succeeded, total, failed, elapsedStr)
	}
	return fmt.Sprintf("Downloaded %d/%d books (%s elapsed)", succeeded, total, elapsedStr)
}

// FormatResume returns resume message per D-06.
// Format: "Resuming: X of Y remaining (Z completed previously)"
func (p *ProgressTracker) FormatResume(remaining, previouslyDone int) string {
	total := remaining + previouslyDone
	return fmt.Sprintf("Resuming: %d of %d remaining (%d completed previously)", remaining, total, previouslyDone)
}

// PrintBookProgress writes progress to writer on its own line.
// In quiet mode, does nothing (per D-03).
func (p *ProgressTracker) PrintBookProgress(current, total int, author, title, asin string, pct int) {
	s := p.FormatBookProgress(current, total, author, title, asin, pct)
	if s == "" {
		return
	}
	fmt.Fprintln(p.w, s)
}

// PrintSummary writes summary to writer. Always prints, even in quiet mode (per D-04).
func (p *ProgressTracker) PrintSummary(succeeded, total, failed int, elapsed time.Duration) {
	s := p.FormatSummary(succeeded, total, failed, elapsed)
	fmt.Fprintln(p.w, s)
}

// FormatElapsed returns the elapsed-time progress string.
// Format: "[current/total] Downloading: Author - Title [ASIN]... Xm Ys"
// In quiet mode, returns empty string.
func (p *ProgressTracker) FormatElapsed(current, total int, author, title, asin string, elapsed time.Duration) string {
	if p.quiet {
		return ""
	}
	return fmt.Sprintf("[%d/%d] Downloading: %s - %s [%s]... %s", current, total, author, title, asin, formatDuration(elapsed))
}

// PrintElapsed writes the elapsed-time progress using \r for in-place update.
// In quiet mode, does nothing.
func (p *ProgressTracker) PrintElapsed(current, total int, author, title, asin string, elapsed time.Duration) {
	s := p.FormatElapsed(current, total, author, title, asin, elapsed)
	if s == "" {
		return
	}
	fmt.Fprintf(p.w, "\r%s", s)
}

// formatDuration formats a duration as "Xm Ys" for human readability.
func formatDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
