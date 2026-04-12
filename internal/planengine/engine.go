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
	"github.com/lovettbarron/earworm/internal/metadata"
	"github.com/lovettbarron/earworm/internal/scanner"
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

	// Pre-flight validation: check all sources exist and space is sufficient
	if err := e.preflightCheck(ops); err != nil {
		_ = db.UpdatePlanStatusAudited(e.DB, planID, "failed")
		return nil, fmt.Errorf("apply plan: %w", err)
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

// preflightCheck validates all pending operations before execution:
// - Verifies source files exist for non-completed operations
// - For move/split ops where source is missing but dest exists, allows idempotent resume
// - Checks destination filesystem has enough free space for copy/move ops
// Returns an error listing all issues found, or nil if everything is OK.
func (e *Executor) preflightCheck(ops []db.PlanOperation) error {
	var missing []string
	destDirs := make(map[string]uint64) // dir -> bytes needed

	for _, op := range ops {
		// Skip already completed operations (resume support)
		if op.Status == "completed" {
			continue
		}

		// Check source exists for operations that read from source
		switch op.OpType {
		case "move", "split":
			info, err := os.Stat(op.SourcePath)
			if os.IsNotExist(err) {
				// Idempotent resume: source missing but dest exists is OK
				if op.DestPath != "" {
					if _, destErr := os.Stat(op.DestPath); destErr == nil {
						continue // dest exists, will be handled by executeOp resume logic
					}
				}
				missing = append(missing, op.SourcePath)
				continue
			}
			if err != nil {
				missing = append(missing, fmt.Sprintf("%s (stat error: %v)", op.SourcePath, err))
				continue
			}

			// Accumulate space needed at destination for move/split
			if op.DestPath != "" {
				dir := existingAncestor(filepath.Dir(op.DestPath))
				destDirs[dir] += uint64(info.Size())
			}
		case "flatten", "write_metadata":
			_, err := os.Stat(op.SourcePath)
			if os.IsNotExist(err) {
				missing = append(missing, op.SourcePath)
				continue
			}
			if err != nil {
				missing = append(missing, fmt.Sprintf("%s (stat error: %v)", op.SourcePath, err))
			}
		case "delete":
			// Delete ops: source missing is not fatal (idempotent)
			// The executeOp will handle the error per-op
		}
	}

	var errs []string
	if len(missing) > 0 {
		errs = append(errs, fmt.Sprintf("missing source files (%d):\n  %s", len(missing), strings.Join(missing, "\n  ")))
	}

	// Check free space at each unique destination directory
	for dir, needed := range destDirs {
		// Add 10% buffer for filesystem overhead
		needed = needed + needed/10
		if err := fileops.CheckFreeSpace(dir, needed); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("preflight check failed:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// existingAncestor walks up from path until it finds a directory that exists.
// Used to find a valid path for disk space checks when dest directories
// haven't been created yet.
func existingAncestor(path string) string {
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path // reached root
		}
		path = parent
	}
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

// bookToMetadata converts a db.Book to a metadata.BookMetadata.
func bookToMetadata(book *db.Book) *metadata.BookMetadata {
	return &metadata.BookMetadata{
		Title:        book.Title,
		Author:       book.Author,
		Narrator:     book.Narrator,
		Genre:        book.Genre,
		Year:         book.Year,
		Series:       book.Series,
		HasCover:     book.HasCover,
		Duration:     book.Duration,
		ChapterCount: book.ChapterCount,
		FileCount:    book.FileCount,
	}
}

// resolveBookMetadata resolves metadata for a book directory using a layered fallback chain:
// 1. DB lookup by local_path
// 2. Library items -> books lookup by ASIN
// 3. File-based metadata extraction
// 4. Empty metadata with ASIN from folder name
func (e *Executor) resolveBookMetadata(bookDir string) (*metadata.BookMetadata, string) {
	// 1. Try DB lookup by local_path
	book, err := db.GetBookByLocalPath(e.DB, bookDir)
	if err == nil && book != nil {
		return bookToMetadata(book), book.ASIN
	}

	// 2. Try library_items -> books lookup
	item, err := db.GetLibraryItem(e.DB, bookDir)
	if err == nil && item != nil && item.ASIN != "" {
		book, err := db.GetBook(e.DB, item.ASIN)
		if err == nil && book != nil {
			return bookToMetadata(book), book.ASIN
		}
	}

	// 3. Fallback to file-based extraction
	meta, err := metadata.ExtractMetadata(bookDir)
	if err == nil && meta != nil {
		asin := ""
		if extracted, ok := scanner.ExtractASIN(filepath.Base(bookDir)); ok {
			asin = extracted
		}
		return meta, asin
	}

	// 4. Empty metadata with ASIN from folder name
	asin := ""
	if extracted, ok := scanner.ExtractASIN(filepath.Base(bookDir)); ok {
		asin = extracted
	}
	return &metadata.BookMetadata{}, asin
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
		var bookMeta *metadata.BookMetadata
		var asin string

		// Prefer metadata from operation (CSV-provided)
		if op.Metadata != "" {
			bookMeta, asin = parseOperationMetadata(op.Metadata)
		}
		// Fallback to existing resolution chain
		if bookMeta == nil {
			bookMeta, asin = e.resolveBookMetadata(op.SourcePath)
		}

		absMeta := fileops.BuildABSMetadata(bookMeta, asin)
		if err := fileops.WriteMetadataSidecar(op.SourcePath, absMeta); err != nil {
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

// parseOperationMetadata extracts BookMetadata and ASIN from a JSON metadata string
// stored in a plan operation. Returns nil, "" if the string is empty or unparseable.
func parseOperationMetadata(metadataJSON string) (*metadata.BookMetadata, string) {
	if metadataJSON == "" {
		return nil, ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(metadataJSON), &raw); err != nil {
		return nil, ""
	}
	meta := &metadata.BookMetadata{}
	if v, ok := raw["title"].(string); ok {
		meta.Title = v
	}
	if v, ok := raw["author"].(string); ok {
		meta.Author = v
	}
	if v, ok := raw["narrator"].(string); ok {
		meta.Narrator = v
	}
	if v, ok := raw["genre"].(string); ok {
		meta.Genre = v
	}
	if v, ok := raw["series"].(string); ok {
		meta.Series = v
	}
	if v, ok := raw["year"].(float64); ok {
		meta.Year = int(v)
	}
	asin := ""
	if v, ok := raw["asin"].(string); ok {
		asin = v
	}
	return meta, asin
}

// statusStr returns "completed" or "failed" based on success boolean.
func statusStr(success bool) string {
	if success {
		return "completed"
	}
	return "failed"
}
