package audible

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownload_BuildsCorrectCommand(t *testing.T) {
	var capturedArgs []string
	factory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		capturedArgs = append([]string{name}, args...)
		// Return a command that exits 0
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--")
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_SCENARIO=auth_ok")
		return cmd
	}

	c := NewClient("audible", WithCmdFactory(factory))
	err := c.Download(context.Background(), "B08C6YJ1LS", "/tmp/output")
	require.NoError(t, err)

	// Verify command args
	joined := strings.Join(capturedArgs, " ")
	assert.Contains(t, joined, "download")
	assert.Contains(t, joined, "--asin")
	assert.Contains(t, joined, "B08C6YJ1LS")
	assert.Contains(t, joined, "--aaxc")
	assert.Contains(t, joined, "--cover")
	assert.Contains(t, joined, "--cover-size")
	assert.Contains(t, joined, "500")
	assert.Contains(t, joined, "--chapter")
	assert.Contains(t, joined, "--output-dir")
	assert.Contains(t, joined, "/tmp/output")
	assert.Contains(t, joined, "--no-confirm")
	assert.Contains(t, joined, "--quality")
	assert.Contains(t, joined, "best")
}

func TestDownload_WithProfilePath(t *testing.T) {
	var capturedArgs []string
	factory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		capturedArgs = append([]string{name}, args...)
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--")
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_SCENARIO=auth_ok")
		return cmd
	}

	c := NewClient("audible", WithProfilePath("/home/user/.audible"), WithCmdFactory(factory))
	err := c.Download(context.Background(), "B08C6YJ1LS", "/tmp/output")
	require.NoError(t, err)

	joined := strings.Join(capturedArgs, " ")
	assert.Contains(t, joined, "--profile-dir")
	assert.Contains(t, joined, "/home/user/.audible")
}

func TestDownload_Success(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("auth_ok")))
	err := c.Download(context.Background(), "B08C6YJ1LS", "/tmp/output")
	assert.NoError(t, err)
}

func TestDownload_AuthError(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("auth_fail")))
	err := c.Download(context.Background(), "B08C6YJ1LS", "/tmp/output")
	require.Error(t, err)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr), "expected *AuthError, got %T", err)
	assert.Contains(t, authErr.Message, "unauthorized")
}

func TestDownload_RateLimitError(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("rate_limit")))
	err := c.Download(context.Background(), "B08C6YJ1LS", "/tmp/output")
	require.Error(t, err)

	var rlErr *RateLimitError
	assert.True(t, errors.As(err, &rlErr), "expected *RateLimitError, got %T", err)
}

func TestDownload_GenericError(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("generic_fail")))
	err := c.Download(context.Background(), "B08C6YJ1LS", "/tmp/output")
	require.Error(t, err)

	var cmdErr *CommandError
	assert.True(t, errors.As(err, &cmdErr), "expected *CommandError, got %T", err)
}

func TestDownload_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := NewClient("audible", WithCmdFactory(fakeCommand("slow")))
	err := c.Download(ctx, "B08C6YJ1LS", "/tmp/output")
	// Should error due to context timeout killing the slow process
	assert.Error(t, err)
}
