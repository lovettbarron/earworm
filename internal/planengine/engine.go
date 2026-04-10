package planengine

import (
	"context"
	"database/sql"
)

// Executor applies plans by dispatching operations to fileops primitives.
type Executor struct {
	DB *sql.DB
	// afterOpHook is called after each operation for testing (e.g., context cancellation).
	afterOpHook func()
}

// OpResult records the outcome of a single plan operation execution.
type OpResult struct {
	OperationID int64
	Success     bool
	SHA256      string
	Error       string
}

// Apply executes all pending operations in a plan sequentially.
func (e *Executor) Apply(ctx context.Context, planID int64) ([]OpResult, error) {
	// TODO: implement
	return nil, nil
}
