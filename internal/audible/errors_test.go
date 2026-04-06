package audible

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthError(t *testing.T) {
	inner := errors.New("connection refused")
	err := &AuthError{Message: "auth failed", Err: inner}
	assert.Equal(t, "audible auth error: auth failed", err.Error())
	assert.Equal(t, inner, err.Unwrap())
}

func TestRateLimitError(t *testing.T) {
	inner := errors.New("too many requests")
	err := &RateLimitError{Message: "slow down", Err: inner}
	assert.Equal(t, "audible rate limit: slow down", err.Error())
	assert.Equal(t, inner, err.Unwrap())
}

func TestNotAvailableError(t *testing.T) {
	inner := errors.New("book removed")
	err := &NotAvailableError{Message: "license denied", Err: inner}
	assert.Equal(t, "audible not available: license denied", err.Error())
	assert.Equal(t, inner, err.Unwrap())
}

func TestCommandError(t *testing.T) {
	inner := errors.New("exit status 1")
	err := &CommandError{
		Command:  "download",
		Stderr:   "something broke",
		ExitCode: 1,
		Err:      inner,
	}
	assert.Contains(t, err.Error(), "download")
	assert.Contains(t, err.Error(), "exit 1")
	assert.Contains(t, err.Error(), "something broke")
	assert.Equal(t, inner, err.Unwrap())
}

func TestClassifyError_Auth(t *testing.T) {
	err := classifyError("auth", "Unauthorized access", 1, errors.New("fail"))
	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr))
}

func TestClassifyError_RateLimit(t *testing.T) {
	err := classifyError("download", "rate limit exceeded", 1, errors.New("fail"))
	var rlErr *RateLimitError
	assert.True(t, errors.As(err, &rlErr))
}

func TestClassifyError_NotAvailable(t *testing.T) {
	err := classifyError("download", "License denied for this title", 1, errors.New("fail"))
	var naErr *NotAvailableError
	assert.True(t, errors.As(err, &naErr))
}

func TestClassifyError_Generic(t *testing.T) {
	err := classifyError("download", "unknown error occurred", 2, errors.New("fail"))
	var cmdErr *CommandError
	assert.True(t, errors.As(err, &cmdErr))
	assert.Equal(t, 2, cmdErr.ExitCode)
}

func TestWithProgressFunc(t *testing.T) {
	var called bool
	opt := WithProgressFunc(func(p DownloadProgress) { called = true })
	c := &client{}
	opt(c)
	assert.NotNil(t, c.progressFunc)
	c.progressFunc(DownloadProgress{Percent: 50})
	assert.True(t, called)
}

func TestSetProgressFunc(t *testing.T) {
	c := &client{}
	c.SetProgressFunc(func(p DownloadProgress) {})
	assert.NotNil(t, c.progressFunc)
}

func TestCommandFallback(t *testing.T) {
	// Test the command method without cmdFactory (uses os/exec fallback)
	c := &client{audiblePath: "/usr/bin/echo"}
	cmd := c.command(t.Context(), "hello")
	assert.NotNil(t, cmd)
}
