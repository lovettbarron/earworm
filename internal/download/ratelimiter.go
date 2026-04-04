package download

import (
	"context"
	"time"
)

// RateLimiter enforces a configurable delay between download requests
// to avoid hammering Audible servers.
type RateLimiter struct {
	delay time.Duration
}

// NewRateLimiter creates a rate limiter that waits the given number of seconds
// between requests.
func NewRateLimiter(seconds int) *RateLimiter {
	return &RateLimiter{delay: time.Duration(seconds) * time.Second}
}

// Wait blocks for the configured delay, respecting context cancellation.
// Returns nil if delay completes, ctx.Err() if cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r.delay <= 0 {
		return nil
	}
	select {
	case <-time.After(r.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
