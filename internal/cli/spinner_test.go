package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpinner_StartStopIncrement(t *testing.T) {
	buf := new(bytes.Buffer)
	s := NewSpinner(buf, "Testing")

	s.Start()
	s.Increment()
	s.Increment()
	s.Increment()
	// Wait for at least one tick to write output
	time.Sleep(300 * time.Millisecond)
	count := s.Stop()

	assert.Equal(t, int64(3), count)
	assert.Greater(t, buf.Len(), 0, "spinner should have written output")
}

func TestSpinner_StopReturnsZeroIfNoIncrements(t *testing.T) {
	buf := new(bytes.Buffer)
	s := NewSpinner(buf, "Testing")

	s.Start()
	time.Sleep(300 * time.Millisecond)
	count := s.Stop()

	assert.Equal(t, int64(0), count)
}
