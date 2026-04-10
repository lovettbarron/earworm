package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupCommand_NoPending(t *testing.T) {
	_ = setupPlanTestDB(t)
	out, err := executeCommand(t, "cleanup")
	assert.NoError(t, err)
	assert.Contains(t, out, "No files pending cleanup")
}

func TestCleanupCommand_ListsFiles(t *testing.T) {
	database := setupPlanTestDB(t)

	planID, err := db.CreatePlan(database, "list-test", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "running"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "completed"))

	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "delete", SourcePath: "/library/Author/Book/file.m4a",
	})
	require.NoError(t, err)

	// Provide "n" to reject confirmation
	stdinReader = strings.NewReader("n\n")

	out, err := executeCommand(t, "cleanup")
	assert.NoError(t, err)
	assert.Contains(t, out, "/library/Author/Book/file.m4a")
	assert.Contains(t, out, "Files pending cleanup")
}

func TestCleanupCommand_ConfirmReject(t *testing.T) {
	database := setupPlanTestDB(t)

	planID, err := db.CreatePlan(database, "reject-test", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "running"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "completed"))

	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "delete", SourcePath: "/some/file.m4a",
	})
	require.NoError(t, err)

	// First "y" then "n" -- second confirmation rejected
	stdinReader = strings.NewReader("y\nn\n")

	out, err := executeCommand(t, "cleanup")
	assert.NoError(t, err)
	assert.Contains(t, out, "Cleanup cancelled")
}

func TestCleanupCommand_ConfirmAccept(t *testing.T) {
	database := setupPlanTestDB(t)
	tmpDir := t.TempDir()

	// Create temp file to be cleaned up
	targetFile := filepath.Join(tmpDir, "cleanup-target.m4a")
	require.NoError(t, os.WriteFile(targetFile, []byte("audio data"), 0644))

	planID, err := db.CreatePlan(database, "accept-test", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "running"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "completed"))

	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "delete", SourcePath: targetFile,
	})
	require.NoError(t, err)

	// Double "y" to confirm
	stdinReader = strings.NewReader("y\ny\n")

	out, err := executeCommand(t, "cleanup")
	assert.NoError(t, err)
	assert.Contains(t, out, "Moved:")

	// File should be gone from original location
	_, statErr := os.Stat(targetFile)
	assert.True(t, os.IsNotExist(statErr), "target file should be moved to trash")
}

func TestCleanupCommand_PlanIDFilter(t *testing.T) {
	database := setupPlanTestDB(t)

	// Create two completed plans
	plan1ID, err := db.CreatePlan(database, "plan1", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, plan1ID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(database, plan1ID, "running"))
	require.NoError(t, db.UpdatePlanStatus(database, plan1ID, "completed"))
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: plan1ID, Seq: 1, OpType: "delete", SourcePath: "/plan1/file.m4a",
	})
	require.NoError(t, err)

	plan2ID, err := db.CreatePlan(database, "plan2", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, plan2ID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(database, plan2ID, "running"))
	require.NoError(t, db.UpdatePlanStatus(database, plan2ID, "completed"))
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: plan2ID, Seq: 1, OpType: "delete", SourcePath: "/plan2/file.m4a",
	})
	require.NoError(t, err)

	// Filter to plan1 only; reject confirmation
	stdinReader = strings.NewReader("n\n")

	out, err := executeCommand(t, "cleanup", "--plan-id", "1")
	assert.NoError(t, err)
	assert.Contains(t, out, "/plan1/file.m4a")
	assert.NotContains(t, out, "/plan2/file.m4a")
}

func TestCleanupCommand_JSON(t *testing.T) {
	database := setupPlanTestDB(t)
	tmpDir := t.TempDir()

	targetFile := filepath.Join(tmpDir, "json-test.m4a")
	require.NoError(t, os.WriteFile(targetFile, []byte("audio"), 0644))

	planID, err := db.CreatePlan(database, "json-test", "test")
	require.NoError(t, err)
	require.NoError(t, db.UpdatePlanStatus(database, planID, "ready"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "running"))
	require.NoError(t, db.UpdatePlanStatus(database, planID, "completed"))
	_, err = db.AddOperation(database, db.PlanOperation{
		PlanID: planID, Seq: 1, OpType: "delete", SourcePath: targetFile,
	})
	require.NoError(t, err)

	// Confirm cleanup
	stdinReader = strings.NewReader("y\ny\n")

	out, err := executeCommand(t, "cleanup", "--json")
	assert.NoError(t, err)

	// Extract JSON portion from output (after confirmation text)
	jsonStart := strings.Index(out, "{")
	require.True(t, jsonStart >= 0, "output should contain JSON object: %s", out)
	jsonStr := out[jsonStart:]

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &result))
	assert.Equal(t, float64(1), result["moved"])
	assert.Equal(t, float64(0), result["skipped"])
}
