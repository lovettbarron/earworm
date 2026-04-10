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
	dbPath := filepath.Join(tmpDir, "earworm.db")

	// Write a minimal config so earworm can find the DB.
	cfgDir := filepath.Join(tmpDir, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("db_path: "+dbPath+"\nlibrary_path: /tmp/lib\n"), 0644))

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
