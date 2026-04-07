package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration006(t *testing.T) {
	db := setupTestDB(t)

	// Verify migration 006 was recorded
	var version int
	err := db.QueryRow("SELECT version FROM schema_versions WHERE version = 6").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 6, version)

	// Verify scan_issues table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='scan_issues'").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "scan_issues", name)
}

func TestInsertScanIssue(t *testing.T) {
	db := setupTestDB(t)

	issue := ScanIssue{
		Path:            "/library/Author/Book",
		IssueType:       "missing_cover",
		Severity:        "warning",
		Message:         "No cover image found",
		SuggestedAction: "Add a cover.jpg file to the book folder",
		ScanRunID:       "run-001",
	}
	err := InsertScanIssue(db, issue)
	require.NoError(t, err)

	// Read it back
	issues, err := ListScanIssues(db)
	require.NoError(t, err)
	require.Len(t, issues, 1)

	got := issues[0]
	assert.Equal(t, "/library/Author/Book", got.Path)
	assert.Equal(t, "missing_cover", got.IssueType)
	assert.Equal(t, "warning", got.Severity)
	assert.Equal(t, "No cover image found", got.Message)
	assert.Equal(t, "Add a cover.jpg file to the book folder", got.SuggestedAction)
	assert.Equal(t, "run-001", got.ScanRunID)
	assert.False(t, got.CreatedAt.IsZero())
	assert.NotZero(t, got.ID)
}

func TestListScanIssues(t *testing.T) {
	db := setupTestDB(t)

	issues := []ScanIssue{
		{Path: "/path/a", IssueType: "missing_cover", Severity: "warning", ScanRunID: "run-1"},
		{Path: "/path/b", IssueType: "no_metadata", Severity: "error", ScanRunID: "run-1"},
		{Path: "/path/c", IssueType: "duplicate", Severity: "info", ScanRunID: "run-1"},
	}
	for _, issue := range issues {
		require.NoError(t, InsertScanIssue(db, issue))
	}

	result, err := ListScanIssues(db)
	require.NoError(t, err)
	assert.Len(t, result, 3)
}

func TestListScanIssuesByPath(t *testing.T) {
	db := setupTestDB(t)

	issues := []ScanIssue{
		{Path: "/library/Author1/Book1", IssueType: "missing_cover", Severity: "warning", ScanRunID: "run-1"},
		{Path: "/library/Author1/Book1", IssueType: "no_metadata", Severity: "error", ScanRunID: "run-1"},
		{Path: "/library/Author2/Book2", IssueType: "duplicate", Severity: "info", ScanRunID: "run-1"},
	}
	for _, issue := range issues {
		require.NoError(t, InsertScanIssue(db, issue))
	}

	result, err := ListScanIssuesByPath(db, "/library/Author1/Book1")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	for _, r := range result {
		assert.Equal(t, "/library/Author1/Book1", r.Path)
	}
}

func TestListScanIssuesByType(t *testing.T) {
	db := setupTestDB(t)

	issues := []ScanIssue{
		{Path: "/path/a", IssueType: "missing_cover", Severity: "warning", ScanRunID: "run-1"},
		{Path: "/path/b", IssueType: "no_metadata", Severity: "error", ScanRunID: "run-1"},
		{Path: "/path/c", IssueType: "missing_cover", Severity: "warning", ScanRunID: "run-1"},
	}
	for _, issue := range issues {
		require.NoError(t, InsertScanIssue(db, issue))
	}

	result, err := ListScanIssuesByType(db, "missing_cover")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	for _, r := range result {
		assert.Equal(t, "missing_cover", r.IssueType)
	}
}

func TestClearScanIssues(t *testing.T) {
	db := setupTestDB(t)

	issues := []ScanIssue{
		{Path: "/path/a", IssueType: "missing_cover", Severity: "warning", ScanRunID: "run-1"},
		{Path: "/path/b", IssueType: "no_metadata", Severity: "error", ScanRunID: "run-1"},
	}
	for _, issue := range issues {
		require.NoError(t, InsertScanIssue(db, issue))
	}

	// Verify they exist
	before, err := ListScanIssues(db)
	require.NoError(t, err)
	assert.Len(t, before, 2)

	// Clear
	err = ClearScanIssues(db)
	require.NoError(t, err)

	// Verify empty
	after, err := ListScanIssues(db)
	require.NoError(t, err)
	assert.Empty(t, after)
}

func TestInsertScanIssue_NormalizesPath(t *testing.T) {
	db := setupTestDB(t)

	// Insert with trailing slash
	issue := ScanIssue{
		Path:      "/library/Author/Book/",
		IssueType: "missing_cover",
		Severity:  "warning",
		ScanRunID: "run-1",
	}
	err := InsertScanIssue(db, issue)
	require.NoError(t, err)

	// Read back - path should be normalized (no trailing slash)
	issues, err := ListScanIssues(db)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "/library/Author/Book", issues[0].Path)

	// Query by normalized path should find it
	byPath, err := ListScanIssuesByPath(db, "/library/Author/Book/")
	require.NoError(t, err)
	assert.Len(t, byPath, 1)
}
