package planengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates an in-memory SQLite database with all migrations applied.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

// createReadyPlan creates a plan with the given operations and sets it to "ready".
func createReadyPlan(t *testing.T, sqlDB *sql.DB, name string, ops []db.PlanOperation) int64 {
	t.Helper()
	planID, err := db.CreatePlan(sqlDB, name, "test plan")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(sqlDB, planID, "ready"))

	for _, op := range ops {
		op.PlanID = planID
		_, err := db.AddOperation(sqlDB, op)
		require.NoError(t, err)
	}
	return planID
}

// createTempFile creates a temporary file with the given content and returns its path.
func createTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestApplyPlan_SequentialExecution(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src1 := createTempFile(t, tmpDir, "src1.m4a", "audio content 1")
	src2 := createTempFile(t, tmpDir, "src2.m4a", "audio content 2")
	src3 := createTempFile(t, tmpDir, "src3.m4a", "audio content 3")
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	dst2 := filepath.Join(tmpDir, "dst2.m4a")
	dst3 := filepath.Join(tmpDir, "dst3.m4a")

	planID := createReadyPlan(t, sqlDB, "seq-test", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
		{Seq: 3, OpType: "move", SourcePath: src3, DestPath: dst3},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for _, r := range results {
		assert.True(t, r.Success, "op %d should succeed", r.OperationID)
		assert.NotEmpty(t, r.SHA256)
		assert.Empty(t, r.Error)
	}

	// Verify plan status
	plan, err := db.GetPlan(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "completed", plan.Status)

	// Verify all operations completed
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	for _, op := range ops {
		assert.Equal(t, "completed", op.Status)
	}

	// Verify destination files exist
	for _, dst := range []string{dst1, dst2, dst3} {
		_, err := os.Stat(dst)
		assert.NoError(t, err, "destination file should exist: %s", dst)
	}
}

func TestApplyPlan_ResumeSkipsCompleted(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src1 := createTempFile(t, tmpDir, "src1.m4a", "content 1")
	src2 := createTempFile(t, tmpDir, "src2.m4a", "content 2")
	src3 := createTempFile(t, tmpDir, "src3.m4a", "content 3")
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	dst2 := filepath.Join(tmpDir, "dst2.m4a")
	dst3 := filepath.Join(tmpDir, "dst3.m4a")

	planID := createReadyPlan(t, sqlDB, "resume-test", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
		{Seq: 3, OpType: "move", SourcePath: src3, DestPath: dst3},
	})

	// Simulate partial execution: op 1 completed, op 2 was running (crash)
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOperationStatus(sqlDB, ops[0].ID, "completed", ""))
	require.NoError(t, db.UpdateOperationStatus(sqlDB, ops[1].ID, "running", ""))
	// Manually move file 1 to dst1 since op 1 is "completed"
	require.NoError(t, os.Rename(src1, dst1))
	// Set plan to running (simulating crash mid-execution)
	require.NoError(t, db.UpdatePlanStatus(sqlDB, planID, "running"))

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	// Only ops 2 and 3 should be executed
	require.Len(t, results, 2)

	for _, r := range results {
		assert.True(t, r.Success)
	}

	// All ops should be completed now
	ops, err = db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	for _, op := range ops {
		assert.Equal(t, "completed", op.Status, "op seq %d should be completed", op.Seq)
	}
}

func TestApplyPlan_FailedOpContinues(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src1 := createTempFile(t, tmpDir, "src1.m4a", "content 1")
	src3 := createTempFile(t, tmpDir, "src3.m4a", "content 3")
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	dst3 := filepath.Join(tmpDir, "dst3.m4a")

	planID := createReadyPlan(t, sqlDB, "fail-continue-test", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: "/nonexistent/file.m4a", DestPath: filepath.Join(tmpDir, "dst2.m4a")},
		{Seq: 3, OpType: "move", SourcePath: src3, DestPath: dst3},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 3)

	assert.True(t, results[0].Success, "op 1 should succeed")
	assert.False(t, results[1].Success, "op 2 should fail")
	assert.NotEmpty(t, results[1].Error)
	assert.True(t, results[2].Success, "op 3 should succeed")

	// Plan should be "failed" because one op failed
	plan, err := db.GetPlan(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "failed", plan.Status)

	// Verify op statuses
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "completed", ops[0].Status)
	assert.Equal(t, "failed", ops[1].Status)
	assert.Equal(t, "completed", ops[2].Status)
}

func TestApplyPlan_AuditTrailWithHash(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src := createTempFile(t, tmpDir, "audio.m4a", "test audio data")
	dst := filepath.Join(tmpDir, "moved.m4a")

	planID := createReadyPlan(t, sqlDB, "audit-test", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src, DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)

	// Check audit log for the operation
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	entries, err := db.ListAuditEntries(sqlDB, "operation", fmt.Sprintf("%d", ops[0].ID))
	require.NoError(t, err)
	require.NotEmpty(t, entries, "should have audit entry for operation")

	// Parse AfterState JSON and check for sha256 key
	var afterState map[string]interface{}
	err = json.Unmarshal([]byte(entries[0].AfterState), &afterState)
	require.NoError(t, err)
	assert.Contains(t, afterState, "sha256")
	assert.NotEmpty(t, afterState["sha256"])
}

func TestApplyPlan_StatusValidation(t *testing.T) {
	sqlDB := setupTestDB(t)

	// Test "draft" status rejection
	planID, err := db.CreatePlan(sqlDB, "draft-plan", "test")
	require.NoError(t, err)
	// Status is "draft" by default

	executor := &Executor{DB: sqlDB}
	_, err = executor.Apply(context.Background(), planID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "draft")

	// Test "cancelled" status rejection
	planID2, err := db.CreatePlan(sqlDB, "cancelled-plan", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(sqlDB, planID2, "cancelled"))

	_, err = executor.Apply(context.Background(), planID2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

func TestApplyPlan_ContextCancellation(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src1 := createTempFile(t, tmpDir, "src1.m4a", "content 1")
	src2 := createTempFile(t, tmpDir, "src2.m4a", "content 2")
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	dst2 := filepath.Join(tmpDir, "dst2.m4a")

	planID := createReadyPlan(t, sqlDB, "cancel-test", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Use a hook to cancel context after the first operation
	executor := &Executor{
		DB:          sqlDB,
		afterOpHook: func() { cancel() },
	}
	results, err := executor.Apply(ctx, planID)
	require.NoError(t, err)
	// First op should have executed
	require.GreaterOrEqual(t, len(results), 1)
	assert.True(t, results[0].Success)

	// Plan should be failed due to cancellation
	plan, err := db.GetPlan(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "failed", plan.Status)

	// Second operation should remain pending
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "pending", ops[1].Status)
}

func TestApplyPlan_DeleteOperation(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	target := createTempFile(t, tmpDir, "to_delete.txt", "delete me")

	planID := createReadyPlan(t, sqlDB, "delete-test", []db.PlanOperation{
		{Seq: 1, OpType: "delete", SourcePath: target},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)

	// File should be gone
	_, err = os.Stat(target)
	assert.True(t, os.IsNotExist(err))

	// Audit should exist
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	entries, err := db.ListAuditEntries(sqlDB, "operation", fmt.Sprintf("%d", ops[0].ID))
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	assert.True(t, entries[0].Success)
}

func TestApplyPlan_FlattenOperation(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Create a nested directory structure
	bookDir := filepath.Join(tmpDir, "Author", "Book [B123]")
	subDir := filepath.Join(bookDir, "CD1")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	createTempFile(t, subDir, "track01.m4a", "audio track 1")
	createTempFile(t, subDir, "track02.m4a", "audio track 2")

	planID := createReadyPlan(t, sqlDB, "flatten-test", []db.PlanOperation{
		{Seq: 1, OpType: "flatten", SourcePath: bookDir},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)

	// Audio files should be at root level of bookDir
	_, err = os.Stat(filepath.Join(bookDir, "track01.m4a"))
	assert.NoError(t, err, "track01.m4a should be at book root")
	_, err = os.Stat(filepath.Join(bookDir, "track02.m4a"))
	assert.NoError(t, err, "track02.m4a should be at book root")

	// Subdirectory should be removed (empty after flatten)
	_, err = os.Stat(subDir)
	assert.True(t, os.IsNotExist(err), "CD1 subdirectory should be removed")
}
