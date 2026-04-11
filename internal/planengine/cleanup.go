package planengine

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/lovettbarron/earworm/internal/db"
)

// CleanupResult records the outcome of a cleanup execution.
type CleanupResult struct {
	Moved   int
	Skipped int
	Errors  []string
}

// CleanupExecutor processes delete operations by moving files to a trash directory.
type CleanupExecutor struct {
	DB       *sql.DB
	TrashDir string
}

// MoveToTrash moves a file to the trash directory with a unique timestamp-prefixed name.
// The trash directory is created if it does not exist. On cross-filesystem errors,
// falls back to copy+verify+delete.
func MoveToTrash(sourcePath, trashDir string) (string, error) {
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return "", fmt.Errorf("creating trash directory: %w", err)
	}

	basename := filepath.Base(sourcePath)
	trashPath := filepath.Join(trashDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), basename))

	err := os.Rename(sourcePath, trashPath)
	if err == nil {
		return trashPath, nil
	}

	// Cross-filesystem fallback
	if errors.Is(err, syscall.EXDEV) {
		if err := copyVerifyDelete(sourcePath, trashPath); err != nil {
			return "", fmt.Errorf("cross-filesystem trash move: %w", err)
		}
		return trashPath, nil
	}

	return "", fmt.Errorf("rename to trash: %w", err)
}

// copyVerifyDelete copies src to dst, verifies SHA-256 hashes match, then
// deletes source. Data is fsynced before close for NAS safety.
func copyVerifyDelete(src, dst string) error {
	// Hash source BEFORE copy
	srcHash, err := hashFileSHA256(src)
	if err != nil {
		return fmt.Errorf("hashing source: %w", err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return fmt.Errorf("copying data: %w", err)
	}

	// Flush to disk before close (NAS safety)
	if err := dstFile.Sync(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("syncing destination: %w", err)
	}

	if err := dstFile.Close(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("closing destination: %w", err)
	}

	// SHA-256 verification
	dstHash, err := hashFileSHA256(dst)
	if err != nil {
		os.Remove(dst)
		return fmt.Errorf("hashing destination: %w", err)
	}

	if srcHash != dstHash {
		os.Remove(dst)
		return fmt.Errorf("hash mismatch: src=%s dst=%s", srcHash, dstHash)
	}

	if err := srcFile.Close(); err != nil {
		return fmt.Errorf("closing source: %w", err)
	}

	if err := os.Remove(src); err != nil {
		return fmt.Errorf("removing source: %w", err)
	}

	return nil
}

// hashFileSHA256 computes the SHA-256 hex digest of the file at path.
func hashFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ListPending returns pending delete operations from completed plans.
// If planID > 0, filters to only that plan's operations.
func (c *CleanupExecutor) ListPending(planID int64) ([]db.PlanOperation, error) {
	ops, err := db.ListDeleteOperations(c.DB, "completed", planID)
	if err != nil {
		return nil, fmt.Errorf("list pending cleanup: %w", err)
	}

	// Filter to ops with status "pending" (not already processed)
	var pending []db.PlanOperation
	for _, op := range ops {
		if op.Status == "pending" {
			pending = append(pending, op)
		}
	}

	if pending == nil {
		pending = []db.PlanOperation{}
	}
	return pending, nil
}

// Execute processes the given delete operations by moving files to the trash directory.
// Missing files are skipped without aborting the batch.
func (c *CleanupExecutor) Execute(ops []db.PlanOperation) (*CleanupResult, error) {
	result := &CleanupResult{}

	for _, op := range ops {
		// Check if source file exists
		if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
			result.Skipped++
			slog.Warn("cleanup: file not found, skipping", "path", op.SourcePath)
			_ = db.UpdateOperationStatus(c.DB, op.ID, "skipped", "file not found")
			continue
		}

		// Move to trash
		trashPath, err := MoveToTrash(op.SourcePath, c.TrashDir)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", op.SourcePath, err))
			slog.Error("cleanup: trash move failed", "path", op.SourcePath, "error", err)
			_ = db.UpdateOperationStatus(c.DB, op.ID, "failed", err.Error())

			// Log audit with failure
			afterJSON, _ := json.Marshal(map[string]string{
				"error": err.Error(),
			})
			_ = db.LogAudit(c.DB, db.AuditEntry{
				EntityType:   "operation",
				EntityID:     strconv.FormatInt(op.ID, 10),
				Action:       "cleanup_trash",
				AfterState:   string(afterJSON),
				Success:      false,
				ErrorMessage: err.Error(),
			})
			continue
		}

		// Success
		result.Moved++
		_ = db.UpdateOperationStatus(c.DB, op.ID, "completed", "")

		// Log audit entry
		afterJSON, _ := json.Marshal(map[string]string{
			"trash_path": trashPath,
		})
		_ = db.LogAudit(c.DB, db.AuditEntry{
			EntityType: "operation",
			EntityID:   strconv.FormatInt(op.ID, 10),
			Action:     "cleanup_trash",
			AfterState: string(afterJSON),
			Success:    true,
		})
	}

	return result, nil
}
