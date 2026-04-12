package scanner

import (
	"database/sql"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBridgeDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func insertTestIssue(t *testing.T, database *sql.DB, issueType, path string) {
	t.Helper()
	err := db.InsertScanIssue(database, db.ScanIssue{
		Path:            path,
		IssueType:       issueType,
		Severity:        "warning",
		Message:         "test issue: " + issueType,
		SuggestedAction: "test action",
		ScanRunID:       "test-run-1",
	})
	require.NoError(t, err)
}

func TestCreatePlanFromIssues_ActionableOnly(t *testing.T) {
	database := setupBridgeDB(t)

	// Insert 5 issues: 3 actionable, 2 non-actionable
	insertTestIssue(t, database, string(IssueNestedAudio), "/lib/Author/Book1")
	insertTestIssue(t, database, string(IssueEmptyDir), "/lib/Author/EmptyBook")
	insertTestIssue(t, database, string(IssueOrphanFiles), "/lib/Author/Book2")
	insertTestIssue(t, database, string(IssueNoASIN), "/lib/Author/NoAsin")
	insertTestIssue(t, database, string(IssueMultiBook), "/lib/Author/MultiBook")

	issues, err := db.ListScanIssues(database)
	require.NoError(t, err)
	require.Len(t, issues, 5)

	result, err := CreatePlanFromIssues(database, issues)
	require.NoError(t, err)

	assert.Equal(t, 3, result.Created)
	assert.Equal(t, 2, result.Skipped)
	assert.Greater(t, result.PlanID, int64(0))
}

func TestCreatePlanFromIssues_AllSkipped(t *testing.T) {
	database := setupBridgeDB(t)

	insertTestIssue(t, database, string(IssueNoASIN), "/lib/Author/NoAsin")
	insertTestIssue(t, database, string(IssueCoverMissing), "/lib/Author/NoCover")

	issues, err := db.ListScanIssues(database)
	require.NoError(t, err)

	result, err := CreatePlanFromIssues(database, issues)
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no actionable issues found")
}

func TestCreatePlanFromIssues_EmptyInput(t *testing.T) {
	database := setupBridgeDB(t)

	result, err := CreatePlanFromIssues(database, []db.ScanIssue{})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no actionable issues found")
}

func TestCreatePlanFromIssues_OperationTypes(t *testing.T) {
	database := setupBridgeDB(t)

	insertTestIssue(t, database, string(IssueNestedAudio), "/lib/Author/NestedBook")
	insertTestIssue(t, database, string(IssueEmptyDir), "/lib/Author/EmptyBook")
	insertTestIssue(t, database, string(IssueOrphanFiles), "/lib/Author/OrphanBook")

	issues, err := db.ListScanIssues(database)
	require.NoError(t, err)

	result, err := CreatePlanFromIssues(database, issues)
	require.NoError(t, err)

	ops, err := db.ListOperations(database, result.PlanID)
	require.NoError(t, err)
	require.Len(t, ops, 3)

	// nested_audio -> flatten
	assert.Equal(t, "flatten", ops[0].OpType)
	assert.Equal(t, "/lib/Author/NestedBook", ops[0].SourcePath)
	assert.Equal(t, "", ops[0].DestPath)

	// empty_dir -> delete
	assert.Equal(t, "delete", ops[1].OpType)
	assert.Equal(t, "/lib/Author/EmptyBook", ops[1].SourcePath)

	// orphan_files -> delete
	assert.Equal(t, "delete", ops[2].OpType)
	assert.Equal(t, "/lib/Author/OrphanBook", ops[2].SourcePath)
}

func TestCreatePlanFromIssues_DraftStatus(t *testing.T) {
	database := setupBridgeDB(t)

	insertTestIssue(t, database, string(IssueEmptyDir), "/lib/Author/EmptyBook")

	issues, err := db.ListScanIssues(database)
	require.NoError(t, err)

	result, err := CreatePlanFromIssues(database, issues)
	require.NoError(t, err)

	plan, err := db.GetPlan(database, result.PlanID)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "draft", plan.Status)
}

func TestCreatePlanFromIssues_PlanNaming(t *testing.T) {
	database := setupBridgeDB(t)

	insertTestIssue(t, database, string(IssueNestedAudio), "/lib/Author/Book1")
	insertTestIssue(t, database, string(IssueEmptyDir), "/lib/Author/EmptyBook")

	issues, err := db.ListScanIssues(database)
	require.NoError(t, err)

	result, err := CreatePlanFromIssues(database, issues)
	require.NoError(t, err)

	plan, err := db.GetPlan(database, result.PlanID)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "scan-issues: auto-plan", plan.Name)
	assert.Contains(t, plan.Description, "2 scan issues")
}
