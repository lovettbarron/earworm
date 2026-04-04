package download

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_New(t *testing.T) {
	rl := NewRateLimiter(5)
	assert.Equal(t, 5*time.Second, rl.delay)
}

func TestRateLimiter_Wait(t *testing.T) {
	tests := []struct {
		name      string
		seconds   int
		cancelCtx bool
		wantErr   error
	}{
		{
			name:    "returns nil after delay elapses",
			seconds: 0, // 0 seconds = immediate
			wantErr: nil,
		},
		{
			name:    "zero delay returns immediately",
			seconds: 0,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.seconds)
			ctx := context.Background()
			err := rl.Wait(ctx)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestRateLimiter_WaitWithDelay(t *testing.T) {
	// Use short delay for tests (construct directly to use milliseconds)
	rl := &RateLimiter{delay: 10 * time.Millisecond}
	ctx := context.Background()

	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond)
}

func TestRateLimiter_WaitCancelledContext(t *testing.T) {
	rl := &RateLimiter{delay: 5 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	err := rl.Wait(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRateLimiter_WaitContextTimeout(t *testing.T) {
	rl := &RateLimiter{delay: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
