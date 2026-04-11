package planengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/fileops"
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
// It validates plan status, supports resume by skipping completed operations
// and resetting running ones, and continues past individual operation failures.
// SHA-256 hashes are recorded in the audit trail for move and flatten operations.
func (e *Executor) Apply(ctx context.Context, planID int64) ([]OpResult, error) {
	plan, err := db.GetPlan(e.DB, planID)
	if err != nil {
		return nil, fmt.Errorf("apply plan: %w", err)
	}
	if plan == nil {
		return nil, fmt.Errorf("apply plan: plan %d not found", planID)
	}

	// Only "ready", "running", and "failed" plans can be applied
	switch plan.Status {
	case "ready", "running", "failed":
		// OK
	default:
		return nil, fmt.Errorf("cannot apply plan with status %q", plan.Status)
	}

	// Resume support: reset any "running" operations to "pending"
	if plan.Status == "running" || plan.Status == "failed" {
		if err := e.prepareResume(planID); err != nil {
			return nil, fmt.Errorf("apply plan resume: %w", err)
		}
	}

	// Transition plan to "running"
	if err := db.UpdatePlanStatusAudited(e.DB, planID, "running"); err != nil {
		return nil, fmt.Errorf("apply plan set running: %w", err)
	}

	ops, err := db.ListOperations(e.DB, planID)
	if err != nil {
		return nil, fmt.Errorf("apply plan list ops: %w", err)
	}

	var results []OpResult
	anyFailed := false
	cancelled := false

	for _, op := range ops {
		// Skip already completed operations (resume support)
		if op.Status == "completed" {
			continue
		}

		// Check for context cancellation before each operation
		if ctx.Err() != nil {
			cancelled = true
			break
		}

		// Mark operation as running
		if err := db.UpdateOperationStatus(e.DB, op.ID, "running", ""); err != nil {
			results = append(results, OpResult{
				OperationID: op.ID,
				Success:     false,
				Error:       fmt.Sprintf("update op status to running: %v", err),
			})
			anyFailed = true
			continue
		}

		// Execute the operation
		result := e.executeOp(ctx, op)

		// Update operation status in DB
		if result.Success {
			if err := db.UpdateOperationStatus(e.DB, op.ID, "completed", ""); err != nil {
				result.Success = false
				result.Error = fmt.Sprintf("update op status to completed: %v", err)
			}
		} else {
			_ = db.UpdateOperationStatus(e.DB, op.ID, "failed", result.Error)
			anyFailed = true
		}

		// Log audit entry
		beforeState, _ := json.Marshal(map[string]string{
			"source_path": op.SourcePath,
			"dest_path":   op.DestPath,
			"op_type":     op.OpType,
		})
		afterState, _ := json.Marshal(map[string]string{
			"sha256": result.SHA256,
			"status": statusStr(result.Success),
			"error":  result.Error,
		})
		_ = db.LogAudit(e.DB, db.AuditEntry{
			EntityType:   "operation",
			EntityID:     fmt.Sprintf("%d", op.ID),
			Action:       "execute",
			BeforeState:  string(beforeState),
			AfterState:   string(afterState),
			Success:      result.Success,
			ErrorMessage: result.Error,
		})

		results = append(results, result)

		// Call test hook if set
		if e.afterOpHook != nil {
			e.afterOpHook()
		}
	}

	// Set final plan status
	finalStatus := "completed"
	if anyFailed || cancelled {
		finalStatus = "failed"
	}
	_ = db.UpdatePlanStatusAudited(e.DB, planID, finalStatus)

	return results, nil
}

// prepareResume resets any "running" operations back to "pending" so they
// can be re-executed after a crash or interruption.
func (e *Executor) prepareResume(planID int64) error {
	_, err := e.DB.Exec(
		`UPDATE plan_operations SET status = 'pending', updated_at = CURRENT_TIMESTAMP WHERE plan_id = ? AND status = 'running'`,
		planID,
	)
	if err != nil {
		return fmt.Errorf("prepare resume for plan %d: %w", planID, err)
	}
	return nil
}

// executeOp dispatches a single operation to the appropriate fileops function.
func (e *Executor) executeOp(ctx context.Context, op db.PlanOperation) OpResult {
	result := OpResult{OperationID: op.ID}

	switch op.OpType {
	case "move":
		// Idempotent resume: if source is gone but dest exists with valid hash, skip
		if _, statErr := os.Stat(op.SourcePath); os.IsNotExist(statErr) {
			hash, hashErr := fileops.HashFile(op.DestPath)
			if hashErr == nil && hash != "" {
				result.Success = true
				result.SHA256 = hash
				return result
			}
			result.Error = fmt.Sprintf("source missing, dest not valid: %s -> %s", op.SourcePath, op.DestPath)
			return result
		}
		if err := fileops.VerifiedMove(op.SourcePath, op.DestPath); err != nil {
			result.Error = err.Error()
			return result
		}
		// Get SHA-256 of the moved file
		hash, err := fileops.HashFile(op.DestPath)
		if err != nil {
			result.Error = fmt.Sprintf("hash after move: %v", err)
			return result
		}
		result.Success = true
		result.SHA256 = hash

	case "flatten":
		flatResult, err := fileops.FlattenDir(op.SourcePath)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		if len(flatResult.Errors) > 0 {
			result.Error = flatResult.Errors[0].Error()
			return result
		}
		result.Success = true
		// Collect SHA-256 from first moved file if available
		if len(flatResult.FilesMoved) > 0 && flatResult.FilesMoved[0].Success {
			result.SHA256 = flatResult.FilesMoved[0].SHA256
		}

	case "delete":
		if err := os.Remove(op.SourcePath); err != nil {
			result.Error = err.Error()
			return result
		}
		result.Success = true

	case "write_metadata":
		if err := fileops.WriteMetadataSidecar(op.SourcePath, fileops.ABSMetadata{
			Tags:      []string{},
			Chapters:  []fileops.ABSChapter{},
			Authors:   []string{},
			Narrators: []string{},
			Series:    []string{},
			Genres:    []string{},
		}); err != nil {
			result.Error = err.Error()
			return result
		}
		result.Success = true

	case "split":
		ext := strings.ToLower(filepath.Ext(op.SourcePath))
		isAudio := isAudioExt(ext)
		if isAudio {
			// Idempotent resume: if source is gone but dest exists with valid hash, skip
			if _, statErr := os.Stat(op.SourcePath); os.IsNotExist(statErr) {
				hash, hashErr := fileops.HashFile(op.DestPath)
				if hashErr == nil && hash != "" {
					result.Success = true
					result.SHA256 = hash
					return result
				}
				result.Error = fmt.Sprintf("source missing, dest not valid: %s -> %s", op.SourcePath, op.DestPath)
				return result
			}
			if err := fileops.VerifiedMove(op.SourcePath, op.DestPath); err != nil {
				result.Error = err.Error()
				return result
			}
		} else {
			if err := fileops.VerifiedCopy(op.SourcePath, op.DestPath); err != nil {
				result.Error = err.Error()
				return result
			}
		}
		hash, err := fileops.HashFile(op.DestPath)
		if err != nil {
			result.Error = fmt.Sprintf("hash after split: %v", err)
			return result
		}
		result.Success = true
		result.SHA256 = hash

	default:
		result.Error = fmt.Sprintf("unknown operation type: %s", op.OpType)
	}

	return result
}

// isAudioExt returns true for audio file extensions that should be moved (not copied) during split.
func isAudioExt(ext string) bool {
	switch ext {
	case ".m4a", ".m4b", ".mp3", ".ogg", ".flac", ".wma", ".aac", ".opus":
		return true
	}
	return false
}

// statusStr returns "completed" or "failed" based on success boolean.
func statusStr(success bool) string {
	if success {
		return "completed"
	}
	return "failed"
}
