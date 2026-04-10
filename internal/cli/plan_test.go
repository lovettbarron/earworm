package cli

import (
	"database/sql"
	"encoding/json"
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
