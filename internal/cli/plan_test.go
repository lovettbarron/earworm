package cli

import (
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

// setupPlanTestDB creates a temp directory with a config file pointing to a fresh SQLite DB,
// opens the DB, and returns the database handle. HOME is redirected for config resolution.
func setupPlanTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()

	// DB must be at ~/.config/earworm/earworm.db since config.DBPath() uses ConfigDir.
	cfgDir := filepath.Join(tmpDir, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	dbPath := filepath.Join(cfgDir, "earworm.db")

	// Write a minimal config file.
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("library_path: /tmp/lib\n"), 0644))

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Open DB so we can seed test data.
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	return database
}

func TestPlanList_Empty(t *testing.T) {
	_ = setupPlanTestDB(t)
	out, err := executeCommand(t, "plan", "list")
	assert.NoError(t, err)
	assert.Contains(t, out, "No plans found")
}

func TestPlanList_ShowsPlans(t *testing.T) {
	database := setupPlanTestDB(t)
	_, err := db.CreatePlan(database, "Fix metadata", "Fix all metadata")
	require.NoError(t, err)
	_, err = db.CreatePlan(database, "Reorganize folders", "Move folders")
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "list")
	assert.NoError(t, err)
	assert.Contains(t, out, "Fix metadata")
	assert.Contains(t, out, "Reorganize folders")
	assert.Contains(t, out, "draft")
}

func TestPlanReview_ShowsOperations(t *testing.T) {
	database := setupPlanTestDB(t)
	planID, err := db.CreatePlan(database, "Test plan", "desc")
	require.NoError(t, err)
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "move",
		SourcePath: "/old/path/book.m4a", DestPath: "/new/path/book.m4a",
	})
	require.NoError(t, err)
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 2, OpType: "delete",
		SourcePath: "/old/path/junk.txt",
	})
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "review", "1")
	assert.NoError(t, err)
	assert.Contains(t, out, "move")
	assert.Contains(t, out, "delete")
	assert.Contains(t, out, "/old/path/book.m4a")
	assert.Contains(t, out, "/new/path/book.m4a")
	assert.Contains(t, out, "/old/path/junk.txt")
	assert.Contains(t, out, "pending")
}

func TestPlanReview_NotFound(t *testing.T) {
	_ = setupPlanTestDB(t)
	_, err := executeCommand(t, "plan", "review", "999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPlanApply_DryRunByDefault(t *testing.T) {
	database := setupPlanTestDB(t)
	planID, err := db.CreatePlan(database, "Dry run plan", "desc")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "ready"))
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "move",
		SourcePath: "/src/file.m4a", DestPath: "/dst/file.m4a",
	})
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "apply", "1")
	assert.NoError(t, err)
	assert.Contains(t, out, "Dry run")
	assert.Contains(t, out, "--confirm")
}

func TestPlanApply_ConfirmExecutes(t *testing.T) {
	database := setupPlanTestDB(t)

	// Create source file to be moved.
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.m4a")
	dstFile := filepath.Join(tmpDir, "dest", "source.m4a")
	require.NoError(t, os.WriteFile(srcFile, []byte("audio data"), 0644))

	planID, err := db.CreatePlan(database, "Move plan", "desc")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "ready"))
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "move",
		SourcePath: srcFile, DestPath: dstFile,
	})
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "apply", "1", "--confirm")
	assert.NoError(t, err)
	assert.Contains(t, out, "completed")

	// Source should no longer exist; dest should exist.
	_, statErr := os.Stat(srcFile)
	assert.True(t, os.IsNotExist(statErr), "source file should be removed after move")
	_, statErr = os.Stat(dstFile)
	assert.NoError(t, statErr, "dest file should exist after move")
}

func TestPlanList_JSON(t *testing.T) {
	database := setupPlanTestDB(t)
	_, err := db.CreatePlan(database, "JSON plan", "desc")
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "list", "--json")
	assert.NoError(t, err)

	var plans []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &plans))
	assert.Len(t, plans, 1)
	assert.Equal(t, "JSON plan", plans[0]["name"])
}

func TestPlanReview_JSON(t *testing.T) {
	database := setupPlanTestDB(t)
	planID, err := db.CreatePlan(database, "Review JSON plan", "desc")
	require.NoError(t, err)
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "move",
		SourcePath: "/a", DestPath: "/b",
	})
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "review", "1", "--json")
	assert.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Contains(t, result, "plan")
	assert.Contains(t, result, "operations")
}

func TestPlanImport_Valid(t *testing.T) {
	_ = setupPlanTestDB(t)
	tmpFile := filepath.Join(t.TempDir(), "test.csv")
	require.NoError(t, os.WriteFile(tmpFile, []byte("op_type,source_path,dest_path\nmove,/src/a.m4a,/dst/a.m4a\ndelete,/src/b.m4a,\n"), 0644))

	out, err := executeCommand(t, "plan", "import", tmpFile)
	assert.NoError(t, err)
	assert.Contains(t, out, "Created plan")
	assert.Contains(t, out, "2 operations")
}

func TestPlanImport_WithName(t *testing.T) {
	_ = setupPlanTestDB(t)
	tmpFile := filepath.Join(t.TempDir(), "test.csv")
	require.NoError(t, os.WriteFile(tmpFile, []byte("op_type,source_path,dest_path\nmove,/src/a.m4a,/dst/a.m4a\n"), 0644))

	out, err := executeCommand(t, "plan", "import", tmpFile, "--name", "my plan")
	assert.NoError(t, err)
	assert.Contains(t, out, "my plan")
}

func TestPlanImport_InvalidCSV(t *testing.T) {
	_ = setupPlanTestDB(t)
	tmpFile := filepath.Join(t.TempDir(), "bad.csv")
	require.NoError(t, os.WriteFile(tmpFile, []byte("op_type,source_path,dest_path\nrename,/src/a.m4a,/dst/a.m4a\n"), 0644))

	_, err := executeCommand(t, "plan", "import", tmpFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation errors")
}

func TestPlanImport_MissingFile(t *testing.T) {
	_ = setupPlanTestDB(t)
	_, err := executeCommand(t, "plan", "import", "/nonexistent/file.csv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open CSV file")
}

func TestPlanImport_NoArgs(t *testing.T) {
	_, err := executeCommand(t, "plan", "import")
	require.Error(t, err)
}

func TestPlanApprove_DraftToReady(t *testing.T) {
	database := setupPlanTestDB(t)
	_, err := db.CreatePlan(database, "Draft plan", "needs approval")
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "approve", "1")
	assert.NoError(t, err)
	assert.Contains(t, out, "Approved plan 1")
	assert.Contains(t, out, "ready")
}

func TestPlanApprove_NotDraft(t *testing.T) {
	database := setupPlanTestDB(t)
	planID, err := db.CreatePlan(database, "Ready plan", "already ready")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "ready"))

	_, err = executeCommand(t, "plan", "approve", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can only approve draft plans, current status: ready")
}

func TestPlanApprove_CompletedPlan(t *testing.T) {
	database := setupPlanTestDB(t)
	planID, err := db.CreatePlan(database, "Done plan", "completed")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "completed"))

	_, err = executeCommand(t, "plan", "approve", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can only approve draft plans, current status: completed")
}

func TestPlanApprove_NotFound(t *testing.T) {
	_ = setupPlanTestDB(t)
	_, err := executeCommand(t, "plan", "approve", "999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPlanApprove_InvalidID(t *testing.T) {
	_ = setupPlanTestDB(t)
	_, err := executeCommand(t, "plan", "approve", "abc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid plan ID")
}

func TestPlanApprove_JSON(t *testing.T) {
	database := setupPlanTestDB(t)
	_, err := db.CreatePlan(database, "JSON approve", "for json output")
	require.NoError(t, err)

	out, err := executeCommand(t, "plan", "approve", "1", "--json")
	assert.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, float64(1), result["id"])
	assert.Equal(t, "ready", result["status"])
}

func TestPlanImport_Approve_Apply(t *testing.T) {
	database := setupPlanTestDB(t)

	// Create source files that the plan will operate on.
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "book.m4a")
	dstDir := filepath.Join(tmpDir, "dest")
	dstFile := filepath.Join(dstDir, "book.m4a")
	require.NoError(t, os.WriteFile(srcFile, []byte("audio content"), 0644))

	// Write a CSV with a move operation using real paths.
	csvFile := filepath.Join(t.TempDir(), "import.csv")
	csvContent := fmt.Sprintf("op_type,source_path,dest_path\nmove,%s,%s\n", srcFile, dstFile)
	require.NoError(t, os.WriteFile(csvFile, []byte(csvContent), 0644))

	// Step 1: Import creates a draft plan.
	out, err := executeCommand(t, "plan", "import", csvFile)
	require.NoError(t, err)
	assert.Contains(t, out, "Created plan")

	// Verify plan is in draft status.
	out, err = executeCommand(t, "plan", "list", "--json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status": "draft"`)

	// Use the DB directly to verify status is still draft.
	plan, err := db.GetPlan(database, 1)
	require.NoError(t, err)
	assert.Equal(t, "draft", plan.Status)

	// Step 2: Approve the plan.
	out, err = executeCommand(t, "plan", "approve", "1")
	require.NoError(t, err)
	assert.Contains(t, out, "Approved plan 1")

	// Verify status changed to ready.
	plan, err = db.GetPlan(database, 1)
	require.NoError(t, err)
	assert.Equal(t, "ready", plan.Status)

	// Step 3: Apply with --confirm now succeeds.
	out, err = executeCommand(t, "plan", "apply", "1", "--confirm")
	require.NoError(t, err)
	assert.Contains(t, out, "completed")

	// Verify the file was actually moved.
	_, statErr := os.Stat(srcFile)
	assert.True(t, os.IsNotExist(statErr), "source should be gone after apply")
	_, statErr = os.Stat(dstFile)
	assert.NoError(t, statErr, "dest should exist after apply")
}
