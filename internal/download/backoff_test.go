package download

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoffCalculator_Delay(t *testing.T) {
	tests := []struct {
		name       string
		baseSec    int
		multiplier float64
		attempt    int
		want       time.Duration
	}{
		{
			name:       "attempt 0 returns base delay",
			baseSec:    5,
			multiplier: 2.0,
			attempt:    0,
			want:       5 * time.Second,
		},
		{
			name:       "attempt 1 returns base * multiplier",
			baseSec:    5,
			multiplier: 2.0,
			attempt:    1,
			want:       10 * time.Second,
		},
		{
			name:       "attempt 2 returns base * multiplier^2",
			baseSec:    5,
			multiplier: 2.0,
			attempt:    2,
			want:       20 * time.Second,
		},
		{
			name:       "attempt 10 capped at max delay (5 minutes)",
			baseSec:    5,
			multiplier: 2.0,
			attempt:    10,
			want:       5 * time.Minute,
		},
		{
			name:       "multiplier 1.0 always returns base delay",
			baseSec:    5,
			multiplier: 1.0,
			attempt:    5,
			want:       5 * time.Second,
		},
		{
			name:       "large attempt capped at max",
			baseSec:    10,
			multiplier: 3.0,
			attempt:    20,
			want:       5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := NewBackoffCalculator(tt.baseSec, tt.multiplier)
			got := bc.Delay(tt.attempt)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBackoffCalculator_New(t *testing.T) {
	bc := NewBackoffCalculator(5, 2.0)
	assert.Equal(t, 5*time.Second, bc.baseDelay)
	assert.Equal(t, 2.0, bc.multiplier)
	assert.Equal(t, 5*time.Minute, bc.maxDelay)
}
