package download

import (
	"math"
	"time"
)

// BackoffCalculator computes exponentially increasing delays for retry logic,
// capped at a maximum delay to prevent unreasonably long waits.
type BackoffCalculator struct {
	baseDelay  time.Duration
	multiplier float64
	maxDelay   time.Duration
}

// NewBackoffCalculator creates a calculator with the given base delay (in seconds)
// and multiplier. The maximum delay is capped at 5 minutes.
func NewBackoffCalculator(baseSeconds int, multiplier float64) *BackoffCalculator {
	return &BackoffCalculator{
		baseDelay:  time.Duration(baseSeconds) * time.Second,
		multiplier: multiplier,
		maxDelay:   5 * time.Minute,
	}
}

// Delay returns the backoff duration for attempt N (0-indexed).
// Formula: baseDelay * multiplier^attempt, capped at maxDelay.
func (b *BackoffCalculator) Delay(attempt int) time.Duration {
	delay := float64(b.baseDelay) * math.Pow(b.multiplier, float64(attempt))
	if delay > float64(b.maxDelay) {
		return b.maxDelay
	}
	return time.Duration(delay)
}
