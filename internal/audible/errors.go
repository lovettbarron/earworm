package audible

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotImplemented is returned by stubbed methods awaiting future implementation.
var ErrNotImplemented = errors.New("not implemented")

// AuthError indicates an authentication failure with audible-cli.
type AuthError struct {
	Message string
	Err     error
}

func (e *AuthError) Error() string { return "audible auth error: " + e.Message }
func (e *AuthError) Unwrap() error { return e.Err }

// RateLimitError indicates Audible is rate-limiting requests.
type RateLimitError struct {
	Message string
	Err     error
}

func (e *RateLimitError) Error() string { return "audible rate limit: " + e.Message }
func (e *RateLimitError) Unwrap() error { return e.Err }

// NotAvailableError indicates a book is no longer accessible (e.g., expired subscription).
type NotAvailableError struct {
	Message string
	Err     error
}

func (e *NotAvailableError) Error() string { return "audible not available: " + e.Message }
func (e *NotAvailableError) Unwrap() error { return e.Err }

// CommandError wraps a failed audible-cli subprocess execution.
type CommandError struct {
	Command  string
	Stderr   string
	ExitCode int
	Err      error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("audible command %q failed (exit %d): %s", e.Command, e.ExitCode, e.Stderr)
}

func (e *CommandError) Unwrap() error { return e.Err }

// classifyError examines stderr output and exit code to return a typed error.
func classifyError(command string, stderr string, exitCode int, err error) error {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "expired") || strings.Contains(lower, "auth") {
		return &AuthError{Message: stderr, Err: err}
	}
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests") {
		return &RateLimitError{Message: stderr, Err: err}
	}
	if strings.Contains(lower, "not available") || strings.Contains(lower, "no longer available") ||
		strings.Contains(lower, "access denied") || strings.Contains(lower, "not owned") ||
		strings.Contains(lower, "license denied") || strings.Contains(lower, "not in library") {
		return &NotAvailableError{Message: stderr, Err: err}
	}
	return &CommandError{Command: command, Stderr: stderr, ExitCode: exitCode, Err: err}
}
