package cli

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// Spinner provides a goroutine-based spinner with live counter for stderr.
// It animates during long-running operations (e.g., NAS mount scanning)
// to provide visual feedback that the process is running.
type Spinner struct {
	w       io.Writer
	message string
	count   atomic.Int64
	done    chan struct{}
}

// NewSpinner creates a new Spinner that writes to w with the given message prefix.
func NewSpinner(w io.Writer, message string) *Spinner {
	return &Spinner{
		w:       w,
		message: message,
		done:    make(chan struct{}),
	}
}

// Start launches the spinner goroutine. It cycles through frames every 200ms,
// writing a carriage-return-overwritten line showing progress.
func (s *Spinner) Start() {
	frames := []string{"|", "/", "-", "\\"}
	go func() {
		i := 0
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				fmt.Fprintf(s.w, "\r%s %s... %d books found", frames[i%len(frames)], s.message, s.count.Load())
				i++
			}
		}
	}()
}

// Increment adds one to the counter. Safe for concurrent use.
func (s *Spinner) Increment() {
	s.count.Add(1)
}

// Stop halts the spinner goroutine and clears the spinner line.
// Returns the final count.
func (s *Spinner) Stop() int64 {
	close(s.done)
	// Clear the spinner line
	fmt.Fprintf(s.w, "\r%-60s\r", "")
	return s.count.Load()
}
