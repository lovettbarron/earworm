package daemon

import (
	"context"
	"log/slog"
	"time"
)

// Run executes the polling loop. It runs one cycle immediately, then
// repeats at the given interval. Returns when ctx is cancelled.
// The cycle function receives the context so it can check for cancellation.
// If verbose is true, log heartbeat messages between cycles.
func Run(ctx context.Context, interval time.Duration, cycle func(ctx context.Context) error, verbose bool) error {
	slog.Info("daemon started", "interval", interval.String())

	// Run first cycle immediately (per D-12).
	if err := cycle(ctx); err != nil {
		slog.Error("cycle failed", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon stopping")
			return ctx.Err()
		case <-ticker.C:
			if verbose {
				slog.Info("starting poll cycle")
			}
			if err := cycle(ctx); err != nil {
				slog.Error("cycle failed", "error", err)
				// D-12: continue polling, don't exit on cycle errors
			}
			if verbose {
				slog.Info("poll cycle complete")
			}
		}
	}
}
