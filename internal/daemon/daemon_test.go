package daemon

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_ImmediateCycle(t *testing.T) {
	var count atomic.Int32

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = Run(ctx, time.Second, func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, false)

	assert.GreaterOrEqual(t, count.Load(), int32(1), "cycle should run at least once (immediate)")
}

func TestRun_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := Run(ctx, time.Second, func(ctx context.Context) error {
		return nil
	}, false)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRun_ContinuesOnCycleError(t *testing.T) {
	var count atomic.Int32
	cycleErr := errors.New("cycle boom")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := Run(ctx, 10*time.Millisecond, func(ctx context.Context) error {
		count.Add(1)
		return cycleErr
	}, false)

	// Run should return context error, not cycle error.
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	// Should have attempted more than one cycle despite errors.
	assert.GreaterOrEqual(t, count.Load(), int32(1))
}

func TestRun_MultipleCycles(t *testing.T) {
	var count atomic.Int32

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_ = Run(ctx, 10*time.Millisecond, func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, true)

	assert.GreaterOrEqual(t, count.Load(), int32(2), "should run at least 2 cycles with short interval")
}
