package planengine

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createCompletedPlanWithOps creates a plan, marks it completed, and adds the given ops.
func createCompletedPlanWithOps(t *testing.T, sqlDB *sql.DB, name string, ops []db.PlanOperation) int64 {
	t.Helper()
	planID, err := db.CreatePlan(sqlDB, name, "test plan")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(sqlDB, planID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(sqlDB, planID, "running"))
	require.NoError(t, db.UpdatePlanStatus(sqlDB, planID, "completed"))

	for _, op := range ops {
		op.PlanID = planID
		_, err := db.AddOperation(sqlDB, op)
		require.NoError(t, err)
	}
	return planID
}

func TestMoveToTrash_SameFS(t *testing.T) {
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, "trash")
	srcFile := filepath.Join(tmpDir, "deleteme.m4a")
	require.NoError(t, os.WriteFile(srcFile, []byte("audio content"), 0644))

	trashPath, err := MoveToTrash(srcFile, trashDir)
	require.NoError(t, err)

	// Source should no longer exist
	_, err = os.Stat(srcFile)
	assert.True(t, os.IsNotExist(err), "source file should be removed")

	// Trash path should exist with same content
	data, err := os.ReadFile(trashPath)
	require.NoError(t, err)
	assert.Equal(t, "audio content", string(data))

	// Trash path should contain the basename
	assert.Contains(t, filepath.Base(trashPath), "deleteme.m4a")
}

func TestMoveToTrash_CreatesTrashDir(t *testing.T) {
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, "nested", "trash", "dir")
	srcFile := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("data"), 0644))

	trashPath, err := MoveToTrash(srcFile, trashDir)
	require.NoError(t, err)

	// Trash dir should have been created
	info, err := os.Stat(trashDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// File should be in the trash dir
	assert.True(t, strings.HasPrefix(trashPath, trashDir))
}

func TestMoveToTrash_UniqueNames(t *testing.T) {
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, "trash")

	// Create two files with the same basename
	src1 := filepath.Join(tmpDir, "dir1", "same.m4a")
	src2 := filepath.Join(tmpDir, "dir2", "same.m4a")
	require.NoError(t, os.MkdirAll(filepath.Dir(src1), 0755))
	require.NoError(t, os.MkdirAll(filepath.Dir(src2), 0755))
	require.NoError(t, os.WriteFile(src1, []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(src2, []byte("content2"), 0644))

	// Small sleep to ensure different timestamps
	time.Sleep(2 * time.Millisecond)

	trashPath1, err := MoveToTrash(src1, trashDir)
	require.NoError(t, err)

	time.Sleep(2 * time.Millisecond)

	trashPath2, err := MoveToTrash(src2, trashDir)
	require.NoError(t, err)

	// Paths should be different
	assert.NotEqual(t, trashPath1, trashPath2)

	// Both files should exist in trash
	_, err = os.Stat(trashPath1)
	assert.NoError(t, err)
	_, err = os.Stat(trashPath2)
	assert.NoError(t, err)
}

func TestListDeleteOperations_OnlyCompleted(t *testing.T) {
	sqlDB := setupTestDB(t)

	// Create a completed plan with a delete op
	completedID := createCompletedPlanWithOps(t, sqlDB, "completed-plan", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: "/path/to/delete.m4a"},
	})

	// Create a draft plan with a delete op (should NOT be returned)
	draftID, err := db.CreatePlan(sqlDB, "draft-plan", "test")
	require.NoError(t, err)
	_, err = db.AddOperation(sqlDB, db.PlanOperation{
		PlanID: draftID, Seq: 1, OpType: "delete", SourcePath: "/path/to/draft-delete.m4a",
	})
	require.NoError(t, err)

	// Create a running plan with a delete op (should NOT be returned)
	runningID, err := db.CreatePlan(sqlDB, "running-plan", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(sqlDB, runningID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(sqlDB, runningID, "running"))
	_, err = db.AddOperation(sqlDB, db.PlanOperation{
		PlanID: runningID, Seq: 1, OpType: "delete", SourcePath: "/path/to/running-delete.m4a",
	})
	require.NoError(t, err)

	ops, err := db.ListDeleteOperations(sqlDB, "completed", 0)
	require.NoError(t, err)

	// Should only have the completed plan's op
	require.Len(t, ops, 1)
	assert.Equal(t, completedID, ops[0].PlanID)
	assert.Equal(t, "/path/to/delete.m4a", ops[0].SourcePath)
}

func TestListDeleteOperations_OnlyDeletes(t *testing.T) {
	sqlDB := setupTestDB(t)

	// Create a completed plan with multiple op types
	createCompletedPlanWithOps(t, sqlDB, "mixed-plan", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: "/a", DestPath: "/b"},
		{Seq: 2, OpType: "delete", SourcePath: "/path/to/delete.m4a"},
		{Seq: 3, OpType: "flatten", SourcePath: "/c"},
	})

	ops, err := db.ListDeleteOperations(sqlDB, "completed", 0)
	require.NoError(t, err)

	// Should only have the delete op
	require.Len(t, ops, 1)
	assert.Equal(t, "delete", ops[0].OpType)
	assert.Equal(t, "/path/to/delete.m4a", ops[0].SourcePath)
}

func TestListDeleteOperations_Empty(t *testing.T) {
	sqlDB := setupTestDB(t)

	ops, err := db.ListDeleteOperations(sqlDB, "completed", 0)
	require.NoError(t, err)
	assert.Empty(t, ops)
	assert.NotNil(t, ops) // Should be empty slice, not nil
}

func TestCleanupExecutor_MovesFiles(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, "trash")

	// Create temp files that will be "deleted"
	file1 := filepath.Join(tmpDir, "book1.m4a")
	file2 := filepath.Join(tmpDir, "book2.m4a")
	require.NoError(t, os.WriteFile(file1, []byte("audio1"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("audio2"), 0644))

	// Create completed plan with delete ops
	planID := createCompletedPlanWithOps(t, sqlDB, "cleanup-test", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: file1},
		{Seq: 2, OpType: "delete", SourcePath: file2},
	})

	executor := &CleanupExecutor{DB: sqlDB, TrashDir: trashDir}
	ops, err := executor.ListPending(planID)
	require.NoError(t, err)
	require.Len(t, ops, 2)

	result, err := executor.Execute(ops)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Moved)
	assert.Equal(t, 0, result.Skipped)
	assert.Empty(t, result.Errors)

	// Source files should be gone
	_, err = os.Stat(file1)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(file2)
	assert.True(t, os.IsNotExist(err))

	// Trash dir should have files
	entries, err := os.ReadDir(trashDir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestCleanupExecutor_AuditEntries(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, "trash")

	file1 := filepath.Join(tmpDir, "audit-test.m4a")
	require.NoError(t, os.WriteFile(file1, []byte("audio"), 0644))

	planID := createCompletedPlanWithOps(t, sqlDB, "audit-cleanup", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: file1},
	})

	executor := &CleanupExecutor{DB: sqlDB, TrashDir: trashDir}
	ops, err := executor.ListPending(planID)
	require.NoError(t, err)

	_, err = executor.Execute(ops)
	require.NoError(t, err)

	// Check audit entries
	allOps, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	require.Len(t, allOps, 1)

	entries, err := db.ListAuditEntries(sqlDB, "operation", "1")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	// Find the cleanup_trash entry
	var found bool
	for _, e := range entries {
		if e.Action == "cleanup_trash" {
			found = true
			assert.True(t, e.Success)
			// Check after_state contains trash_path
			var afterState map[string]string
			require.NoError(t, json.Unmarshal([]byte(e.AfterState), &afterState))
			assert.Contains(t, afterState, "trash_path")
			assert.NotEmpty(t, afterState["trash_path"])
		}
	}
	assert.True(t, found, "should have cleanup_trash audit entry")
}

func TestCleanupExecutor_SkipsMissing(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, "trash")

	// Create completed plan with delete op for nonexistent file
	planID := createCompletedPlanWithOps(t, sqlDB, "skip-test", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: "/nonexistent/file.m4a"},
	})

	executor := &CleanupExecutor{DB: sqlDB, TrashDir: trashDir}
	ops, err := executor.ListPending(planID)
	require.NoError(t, err)

	result, err := executor.Execute(ops)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Moved)
	assert.Equal(t, 1, result.Skipped)

	// Operation status should be updated to skipped
	allOps, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "skipped", allOps[0].Status)
	assert.Contains(t, allOps[0].ErrorMessage, "file not found")
}

func TestCleanupExecutor_UpdatesOpStatus(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()
	trashDir := filepath.Join(tmpDir, "trash")

	file1 := filepath.Join(tmpDir, "status-test.m4a")
	require.NoError(t, os.WriteFile(file1, []byte("audio"), 0644))

	planID := createCompletedPlanWithOps(t, sqlDB, "status-cleanup", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: file1},
	})

	executor := &CleanupExecutor{DB: sqlDB, TrashDir: trashDir}
	ops, err := executor.ListPending(planID)
	require.NoError(t, err)

	_, err = executor.Execute(ops)
	require.NoError(t, err)

	// Check operation status is "completed"
	allOps, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "completed", allOps[0].Status)
}

func TestCleanup_CopyVerifyDelete_SHA256(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.m4a")
	dst := filepath.Join(tmpDir, "verified.m4a")

	content := []byte("audiobook content for planengine SHA-256 verification")
	require.NoError(t, os.WriteFile(src, content, 0644))

	// Compute expected SHA-256
	h := sha256.New()
	h.Write(content)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	err := copyVerifyDelete(src, dst)
	require.NoError(t, err)

	// Source should be deleted
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source should be deleted after verified copy")

	// Destination should have correct SHA-256 hash
	dstData, err := os.ReadFile(dst)
	require.NoError(t, err)
	dstH := sha256.New()
	dstH.Write(dstData)
	actualHash := hex.EncodeToString(dstH.Sum(nil))
	assert.Equal(t, expectedHash, actualHash, "destination SHA-256 should match source")
}

func TestCleanupExecutor_PlanIDFilter(t *testing.T) {
	sqlDB := setupTestDB(t)

	// Create two completed plans with delete ops
	plan1ID := createCompletedPlanWithOps(t, sqlDB, "plan1", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: "/plan1/file.m4a"},
	})
	createCompletedPlanWithOps(t, sqlDB, "plan2", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: "/plan2/file.m4a"},
	})

	executor := &CleanupExecutor{DB: sqlDB, TrashDir: "/tmp/trash"}

	// Filter to plan1 only
	ops, err := executor.ListPending(plan1ID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "/plan1/file.m4a", ops[0].SourcePath)

	// Without filter, get both
	allOps, err := executor.ListPending(0)
	require.NoError(t, err)
	assert.Len(t, allOps, 2)
}
