package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestConfig creates a temp config file with the given library_path and returns
// the config file path. It also sets HOME to a temp dir so DBPath resolves there.
func writeTestConfig(t *testing.T, libPath string) string {
	t.Helper()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Create config dir
	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := "library_path: " + libPath + "\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))

	return cfgPath
}

// createTestLibrary creates a Libation-compatible library structure in a temp dir.
// Returns the library root path.
func createTestLibrary(t *testing.T) string {
	t.Helper()
	tmpLib := t.TempDir()

	// Author Name/Book Title [B08C6YJ1LS]/book.m4a
	bookDir := filepath.Join(tmpLib, "Test Author", "Great Book [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), []byte("fake"), 0644))

	// Second book
	bookDir2 := filepath.Join(tmpLib, "Another Writer", "Second Book [B09ABCDEF1]")
	require.NoError(t, os.MkdirAll(bookDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir2, "part1.m4a"), []byte("fake"), 0644))

	return tmpLib
}

func TestScanNoLibraryPath(t *testing.T) {
	// Set HOME to temp so no existing config interferes
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	_, err := executeCommand(t, "scan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library path not configured")
}

func TestScanNonexistentPath(t *testing.T) {
	cfgPath := writeTestConfig(t, "/nonexistent/path/that/does/not/exist")

	_, err := executeCommand(t, "--config", cfgPath, "scan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestScanValidLibrary(t *testing.T) {
	tmpLib := createTestLibrary(t)
	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan")
	require.NoError(t, err)
	assert.Contains(t, out, "Scan complete")
	assert.Contains(t, out, "Added:")
	// Should have found 2 books
	assert.Contains(t, out, "Added:   2")
}

func TestScanRecursive(t *testing.T) {
	tmpLib := t.TempDir()

	// Create a deeply nested structure
	deepDir := filepath.Join(tmpLib, "Level1", "Level2", "Deep Title [B09ABCDEF1]")
	require.NoError(t, os.MkdirAll(deepDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(deepDir, "book.m4a"), []byte("fake"), 0644))

	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan", "--recursive")
	require.NoError(t, err)
	assert.Contains(t, out, "Scan complete")
	assert.Contains(t, out, "Added:")
	// The recursive scan should find the deep book
	assert.True(t, strings.Contains(out, "Added:   1"), "expected 1 book added, got: %s", out)
}

func TestScanRescan(t *testing.T) {
	tmpLib := createTestLibrary(t)
	cfgPath := writeTestConfig(t, tmpLib)

	// First scan
	out1, err := executeCommand(t, "--config", cfgPath, "scan")
	require.NoError(t, err)
	assert.Contains(t, out1, "Added:   2")

	// Second scan -- same books should be updated, not added again
	out2, err := executeCommand(t, "--config", cfgPath, "scan")
	require.NoError(t, err)
	assert.Contains(t, out2, "Scan complete")
	assert.Contains(t, out2, "Updated:")
	// No new additions on rescan
	assert.Contains(t, out2, "Added:   0")
	assert.Contains(t, out2, "Updated: 2")
}

func TestScanCommand(t *testing.T) {
	// Verify scan command is registered with expected flags
	out, err := executeCommand(t, "scan", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--recursive")
	assert.Contains(t, out, "-r")
	assert.Contains(t, out, "--deep")
}

func TestScanDeep(t *testing.T) {
	tmpLib := t.TempDir()

	// Create dirs with ASIN and without
	bookDir := filepath.Join(tmpLib, "Author", "Book [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), []byte("fake"), 0644))

	noASINDir := filepath.Join(tmpLib, "Random Folder")
	require.NoError(t, os.MkdirAll(noASINDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(noASINDir, "track.m4a"), []byte("fake"), 0644))

	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan", "--deep")
	require.NoError(t, err)
	assert.Contains(t, out, "Deep scan complete")
	assert.Contains(t, out, "Directories:")
	assert.Contains(t, out, "With ASIN:")
	assert.Contains(t, out, "Without ASIN:")
	assert.Contains(t, out, "Issues found:")
}

func TestScanDeepShowsIssues(t *testing.T) {
	tmpLib := t.TempDir()

	// Create empty subdir to trigger empty_dir issue
	emptyDir := filepath.Join(tmpLib, "EmptyBook")
	require.NoError(t, os.MkdirAll(emptyDir, 0755))

	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan", "--deep")
	require.NoError(t, err)
	assert.Contains(t, out, "Deep scan complete")
	// Should have at least 1 issue (empty_dir)
	assert.NotContains(t, out, "Issues found: 0")
}

func TestScanWithoutDeep_Unchanged(t *testing.T) {
	tmpLib := createTestLibrary(t)
	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan")
	require.NoError(t, err)
	// Existing behavior unchanged: shows "Scan complete:" not "Deep scan complete:"
	assert.Contains(t, out, "Scan complete:")
	assert.NotContains(t, out, "Deep scan complete")
}

// setupTestDB opens the DB at the config-derived path and inserts test issues.
// Must be called AFTER writeTestConfig (which sets HOME).
func setupTestDB(t *testing.T, issues []db.ScanIssue) {
	t.Helper()
	dbPath, err := config.DBPath()
	require.NoError(t, err)
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	defer database.Close()
	for _, issue := range issues {
		require.NoError(t, db.InsertScanIssue(database, issue))
	}
}

func TestScanIssues_ListAll(t *testing.T) {
	tmpLib := t.TempDir()
	cfgPath := writeTestConfig(t, tmpLib)

	setupTestDB(t, []db.ScanIssue{
		{Path: "/lib/Author/Book1", IssueType: "nested_audio", Severity: "warning", Message: "Nested audio files found", SuggestedAction: "flatten", ScanRunID: "run1"},
		{Path: "/lib/Author/Empty", IssueType: "empty_dir", Severity: "info", Message: "Empty directory", SuggestedAction: "delete", ScanRunID: "run1"},
		{Path: "/lib/Unknown", IssueType: "no_asin", Severity: "warning", Message: "No ASIN found", SuggestedAction: "manual review", ScanRunID: "run1"},
	})

	out, err := executeCommand(t, "--config", cfgPath, "scan", "issues")
	require.NoError(t, err)
	assert.Contains(t, out, "nested_audio")
	assert.Contains(t, out, "empty_dir")
	assert.Contains(t, out, "no_asin")
	assert.Contains(t, out, "3)")
}

func TestScanIssues_FilterByType(t *testing.T) {
	tmpLib := t.TempDir()
	cfgPath := writeTestConfig(t, tmpLib)

	setupTestDB(t, []db.ScanIssue{
		{Path: "/lib/Author/Book1", IssueType: "nested_audio", Severity: "warning", Message: "Nested audio", SuggestedAction: "flatten", ScanRunID: "run1"},
		{Path: "/lib/Author/Empty", IssueType: "empty_dir", Severity: "info", Message: "Empty directory", SuggestedAction: "delete", ScanRunID: "run1"},
		{Path: "/lib/Unknown", IssueType: "no_asin", Severity: "warning", Message: "No ASIN", SuggestedAction: "manual review", ScanRunID: "run1"},
	})

	out, err := executeCommand(t, "--config", cfgPath, "scan", "issues", "--type", "nested_audio")
	require.NoError(t, err)
	assert.Contains(t, out, "nested_audio")
	assert.NotContains(t, out, "empty_dir")
	assert.NotContains(t, out, "no_asin")
}

func TestScanIssues_JSONOutput(t *testing.T) {
	tmpLib := t.TempDir()
	cfgPath := writeTestConfig(t, tmpLib)

	setupTestDB(t, []db.ScanIssue{
		{Path: "/lib/Author/Book1", IssueType: "nested_audio", Severity: "warning", Message: "Nested audio", SuggestedAction: "flatten", ScanRunID: "run1"},
		{Path: "/lib/Author/Empty", IssueType: "empty_dir", Severity: "info", Message: "Empty", SuggestedAction: "delete", ScanRunID: "run1"},
	})

	out, err := executeCommand(t, "--config", cfgPath, "scan", "issues", "--json")
	require.NoError(t, err)

	var issues []db.ScanIssue
	require.NoError(t, json.Unmarshal([]byte(out), &issues))
	assert.Len(t, issues, 2)
	assert.Equal(t, "nested_audio", issues[0].IssueType)
	assert.Equal(t, "empty_dir", issues[1].IssueType)
}

func TestScanIssues_CreatePlan(t *testing.T) {
	tmpLib := t.TempDir()
	cfgPath := writeTestConfig(t, tmpLib)

	setupTestDB(t, []db.ScanIssue{
		{Path: "/lib/Author/Book1", IssueType: "nested_audio", Severity: "warning", Message: "Nested audio", SuggestedAction: "flatten", ScanRunID: "run1"},
		{Path: "/lib/Author/Empty", IssueType: "empty_dir", Severity: "info", Message: "Empty", SuggestedAction: "delete", ScanRunID: "run1"},
		{Path: "/lib/Unknown", IssueType: "no_asin", Severity: "warning", Message: "No ASIN", SuggestedAction: "manual", ScanRunID: "run1"},
	})

	out, err := executeCommand(t, "--config", cfgPath, "scan", "issues", "--create-plan")
	require.NoError(t, err)
	assert.Contains(t, out, "Plan created")
	assert.Contains(t, out, "2 operations")
	assert.Contains(t, out, "1 issues skipped")

	// Verify plan exists in DB
	dbPath, err := config.DBPath()
	require.NoError(t, err)
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	defer database.Close()

	plan, err := db.GetPlan(database, 1)
	require.NoError(t, err)
	assert.Equal(t, "draft", plan.Status)
}

func TestScanIssues_CreatePlan_NoActionable(t *testing.T) {
	tmpLib := t.TempDir()
	cfgPath := writeTestConfig(t, tmpLib)

	setupTestDB(t, []db.ScanIssue{
		{Path: "/lib/Unknown", IssueType: "no_asin", Severity: "warning", Message: "No ASIN", SuggestedAction: "manual", ScanRunID: "run1"},
		{Path: "/lib/NoCover", IssueType: "cover_missing", Severity: "info", Message: "No cover", SuggestedAction: "manual", ScanRunID: "run1"},
	})

	_, err := executeCommand(t, "--config", cfgPath, "scan", "issues", "--create-plan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no actionable issues")
}

func TestScanIssues_EmptyList(t *testing.T) {
	tmpLib := t.TempDir()
	cfgPath := writeTestConfig(t, tmpLib)

	// Create DB but no issues
	setupTestDB(t, nil)

	out, err := executeCommand(t, "--config", cfgPath, "scan", "issues")
	require.NoError(t, err)
	assert.Contains(t, out, "No issues found")
}

func TestScanDeep_JSONOutput(t *testing.T) {
	tmpLib := t.TempDir()

	// Create a dir with ASIN
	bookDir := filepath.Join(tmpLib, "Author", "Book [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), []byte("fake"), 0644))

	// Create an empty dir to trigger empty_dir issue
	emptyDir := filepath.Join(tmpLib, "EmptyBook")
	require.NoError(t, os.MkdirAll(emptyDir, 0755))

	cfgPath := writeTestConfig(t, tmpLib)

	out, err := executeCommand(t, "--config", cfgPath, "scan", "--deep", "--json")
	require.NoError(t, err)

	var result deepScanJSON
	require.NoError(t, json.Unmarshal([]byte(out), &result), "output should be valid JSON: %s", out)
	assert.Greater(t, result.TotalDirs, 0)
	assert.NotNil(t, result.Issues)
	assert.Greater(t, result.IssuesFound, 0)
}
