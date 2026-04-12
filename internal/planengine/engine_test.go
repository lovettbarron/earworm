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
	// Create src2 that exists (passes preflight) but dest is under a read-only dir (fails at execution)
	src2 := createTempFile(t, tmpDir, "src2.m4a", "content 2")
	src3 := createTempFile(t, tmpDir, "src3.m4a", "content 3")
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	// Use a read-only directory so the move will fail at execution time
	noWriteDir := filepath.Join(tmpDir, "readonly")
	require.NoError(t, os.MkdirAll(noWriteDir, 0555))
	t.Cleanup(func() { os.Chmod(noWriteDir, 0755) })
	dst2 := filepath.Join(noWriteDir, "subdir", "dst2.m4a")
	dst3 := filepath.Join(tmpDir, "dst3.m4a")

	planID := createReadyPlan(t, sqlDB, "fail-continue-test", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
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

func TestApplyPlan_ResumeAlreadyMoved(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Create source and destination paths
	srcPath := filepath.Join(tmpDir, "src", "audio.m4a")
	dstPath := filepath.Join(tmpDir, "dst", "audio.m4a")

	// Only create the destination (simulating a prior successful move where source is gone)
	require.NoError(t, os.MkdirAll(filepath.Dir(dstPath), 0755))
	require.NoError(t, os.WriteFile(dstPath, []byte("audio content"), 0644))

	planID := createReadyPlan(t, sqlDB, "resume-moved-test", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: srcPath, DestPath: dstPath},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success, "should succeed when dest exists with valid hash")
	assert.NotEmpty(t, results[0].SHA256, "should have SHA256 from destination")
	assert.Empty(t, results[0].Error)

	// Operation should be marked completed
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "completed", ops[0].Status)
}

func TestApplyPlan_ResumeAlreadyMoved_Split(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "src", "audio.m4a")
	dstPath := filepath.Join(tmpDir, "dst", "audio.m4a")

	// Only create the destination
	require.NoError(t, os.MkdirAll(filepath.Dir(dstPath), 0755))
	require.NoError(t, os.WriteFile(dstPath, []byte("split audio content"), 0644))

	planID := createReadyPlan(t, sqlDB, "resume-split-test", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: srcPath, DestPath: dstPath},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success, "should succeed when dest exists with valid hash for split")
	assert.NotEmpty(t, results[0].SHA256, "should have SHA256 from destination")
	assert.Empty(t, results[0].Error)

	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	assert.Equal(t, "completed", ops[0].Status)
}

func TestApplyPlan_ResumeMissingBoth(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Neither source nor destination exist -- preflight catches this
	srcPath := filepath.Join(tmpDir, "nonexistent", "audio.m4a")
	dstPath := filepath.Join(tmpDir, "also-nonexistent", "audio.m4a")

	planID := createReadyPlan(t, sqlDB, "resume-missing-both", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: srcPath, DestPath: dstPath},
	})

	executor := &Executor{DB: sqlDB}
	_, err := executor.Apply(context.Background(), planID)
	require.Error(t, err, "should fail at preflight when both source and dest are missing")
	assert.Contains(t, err.Error(), "preflight check failed")
	assert.Contains(t, err.Error(), "missing source files")
}

func TestApplyPlan_PreflightMissingSource(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Create one source file but not the other
	src1 := createTempFile(t, tmpDir, "exists.m4a", "audio content")
	src2 := filepath.Join(tmpDir, "missing.m4a") // does NOT exist
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	dst2 := filepath.Join(tmpDir, "dst2.m4a")

	planID := createReadyPlan(t, sqlDB, "preflight-missing", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
	})

	executor := &Executor{DB: sqlDB}
	_, err := executor.Apply(context.Background(), planID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preflight check failed")
	assert.Contains(t, err.Error(), "missing.m4a")

	// Verify NO operations were executed -- src1 should still be at original location
	_, statErr := os.Stat(src1)
	assert.NoError(t, statErr, "source file should NOT have been moved (preflight should prevent execution)")
	_, statErr = os.Stat(dst1)
	assert.True(t, os.IsNotExist(statErr), "destination should NOT exist (preflight should prevent execution)")
}

func TestApplyPlan_PreflightAllPresent(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src1 := createTempFile(t, tmpDir, "a.m4a", "content a")
	src2 := createTempFile(t, tmpDir, "b.m4a", "content b")
	dst1 := filepath.Join(tmpDir, "dst_a.m4a")
	dst2 := filepath.Join(tmpDir, "dst_b.m4a")

	planID := createReadyPlan(t, sqlDB, "preflight-ok", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.True(t, r.Success)
	}
}

func TestApplyPlan_PreflightSkipsCompleted(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Source for op1 will be deleted (simulating completed op)
	src1 := filepath.Join(tmpDir, "completed.m4a")
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	// Create destination for the "completed" op
	require.NoError(t, os.WriteFile(dst1, []byte("already moved"), 0644))

	// Source for op2 exists (pending op)
	src2 := createTempFile(t, tmpDir, "pending.m4a", "pending content")
	dst2 := filepath.Join(tmpDir, "dst2.m4a")

	planID := createReadyPlan(t, sqlDB, "preflight-skip-completed", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
	})

	// Manually mark op1 as completed and plan as running
	ops, err := db.ListOperations(sqlDB, planID)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOperationStatus(sqlDB, ops[0].ID, "completed", ""))
	require.NoError(t, db.UpdatePlanStatus(sqlDB, planID, "running"))

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err, "preflight should skip completed op whose source is missing")
	// Only op2 should be executed
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
}

func TestApplyPlan_PreflightAggregatesAllMissing(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Both sources are missing
	src1 := filepath.Join(tmpDir, "gone1.m4a")
	src2 := filepath.Join(tmpDir, "gone2.m4a")
	dst1 := filepath.Join(tmpDir, "dst1.m4a")
	dst2 := filepath.Join(tmpDir, "dst2.m4a")

	planID := createReadyPlan(t, sqlDB, "preflight-multi-missing", []db.PlanOperation{
		{Seq: 1, OpType: "move", SourcePath: src1, DestPath: dst1},
		{Seq: 2, OpType: "move", SourcePath: src2, DestPath: dst2},
	})

	executor := &Executor{DB: sqlDB}
	_, err := executor.Apply(context.Background(), planID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing source files (2)")
	assert.Contains(t, err.Error(), "gone1.m4a")
	assert.Contains(t, err.Error(), "gone2.m4a")
}

func TestWriteMetadata_WithDBBook(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Create a book directory
	bookDir := filepath.Join(tmpDir, "Author Name", "My Book [B08XYZ1234]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	// Insert a book with local_path matching the temp dir
	err := db.UpsertBook(sqlDB, db.Book{
		ASIN:      "B08XYZ1234",
		Title:     "My Book",
		Author:    "Author Name",
		Narrator:  "Narrator Person",
		Genre:     "Fiction",
		Year:      2023,
		Status:    "organized",
		LocalPath: bookDir,
	})
	require.NoError(t, err)

	planID := createReadyPlan(t, sqlDB, "write-meta-db", []db.PlanOperation{
		{Seq: 1, OpType: "write_metadata", SourcePath: bookDir},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)

	// Read and verify metadata.json
	metaPath := filepath.Join(bookDir, "metadata.json")
	data, err := os.ReadFile(metaPath)
	require.NoError(t, err)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &meta))
	assert.Equal(t, "My Book", meta["title"])
	assert.Equal(t, "B08XYZ1234", meta["asin"])
	authors, ok := meta["authors"].([]interface{})
	require.True(t, ok)
	require.Len(t, authors, 1)
	assert.Equal(t, "Author Name", authors[0])
}

func TestWriteMetadata_FallbackEmpty(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	// Create a directory with no DB match and no audio files
	bookDir := filepath.Join(tmpDir, "Unknown", "No Match")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	planID := createReadyPlan(t, sqlDB, "write-meta-fallback", []db.PlanOperation{
		{Seq: 1, OpType: "write_metadata", SourcePath: bookDir},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success, "should succeed with graceful degradation")

	// Read and verify metadata.json has valid structure
	metaPath := filepath.Join(bookDir, "metadata.json")
	data, err := os.ReadFile(metaPath)
	require.NoError(t, err)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &meta))
	// Tags, chapters, authors, narrators should be arrays (not null)
	assert.IsType(t, []interface{}{}, meta["tags"])
	assert.IsType(t, []interface{}{}, meta["chapters"])
}

func TestResolveBookMetadata_DBLookup(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	bookDir := filepath.Join(tmpDir, "Author", "Title [BABC123456]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	err := db.UpsertBook(sqlDB, db.Book{
		ASIN:      "BABC123456",
		Title:     "Title",
		Author:    "Author",
		Narrator:  "Narrator",
		Genre:     "SciFi",
		Year:      2020,
		Status:    "organized",
		LocalPath: bookDir,
	})
	require.NoError(t, err)

	executor := &Executor{DB: sqlDB}
	meta, asin := executor.resolveBookMetadata(bookDir)
	require.NotNil(t, meta)
	assert.Equal(t, "Title", meta.Title)
	assert.Equal(t, "Author", meta.Author)
	assert.Equal(t, "Narrator", meta.Narrator)
	assert.Equal(t, "SciFi", meta.Genre)
	assert.Equal(t, 2020, meta.Year)
	assert.Equal(t, "BABC123456", asin)
}

func TestExecuteOp_WriteMetadataFromOp(t *testing.T) {
	sqlDB := setupTestDB(t)
	dir := t.TempDir()
	bookDir := filepath.Join(dir, "Author", "Test Book [B0TEST]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	// Create a dummy audio file so the directory is valid
	createTempFile(t, bookDir, "audio.m4a", "fake audio")

	metaJSON := `{"title":"Test Book","author":"Test Author","narrator":"Narrator One","genre":"Fiction","year":2023,"series":"Series A","asin":"B0TEST"}`

	planID := createReadyPlan(t, sqlDB, "meta-op-test", []db.PlanOperation{
		{Seq: 1, OpType: "write_metadata", SourcePath: bookDir, Metadata: metaJSON},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success, "expected success, got error: %s", results[0].Error)

	// Verify metadata.json was written with CSV-provided values
	data, err := os.ReadFile(filepath.Join(bookDir, "metadata.json"))
	require.NoError(t, err)
	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &meta))
	assert.Equal(t, "Test Book", meta["title"])
	assert.Equal(t, "Test Author", meta["authors"].([]interface{})[0])
	assert.Equal(t, "Narrator One", meta["narrators"].([]interface{})[0])
	assert.Equal(t, "B0TEST", meta["asin"])
	// Verify year was passed through
	assert.Equal(t, "2023", meta["publishedYear"])
}

func TestParseOperationMetadata(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantASIN  string
		wantTitle string
	}{
		{"empty string", "", true, "", ""},
		{"invalid JSON", "{bad", true, "", ""},
		{"valid full", `{"title":"Book","author":"Auth","asin":"B0X","year":2023}`, false, "B0X", "Book"},
		{"partial", `{"title":"Partial"}`, false, "", "Partial"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, asin := parseOperationMetadata(tt.input)
			if tt.wantNil {
				assert.Nil(t, meta)
				assert.Empty(t, asin)
			} else {
				require.NotNil(t, meta)
				assert.Equal(t, tt.wantASIN, asin)
				assert.Equal(t, tt.wantTitle, meta.Title)
			}
		})
	}
}

func TestResolveBookMetadata_ASINFromFolder(t *testing.T) {
	// No DB match, no audio files, just ASIN in folder name
	executor := &Executor{DB: setupTestDB(t)}
	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "Author", "Title [B08XYZ1234]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	meta, asin := executor.resolveBookMetadata(bookDir)
	require.NotNil(t, meta)
	assert.Equal(t, "B08XYZ1234", asin)
}
