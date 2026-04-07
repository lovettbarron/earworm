package db

import (
	"database/sql"
	"fmt"
	"time"
)

// ScanIssue represents a problem detected during library scanning.
type ScanIssue struct {
	ID              int64
	Path            string
	IssueType       string
	Severity        string
	Message         string
	SuggestedAction string
	ScanRunID       string
	CreatedAt       time.Time
}

// scanIssueColumns is the shared column list for SELECT queries on scan_issues.
const scanIssueColumns = `id, path, issue_type, severity, message, suggested_action, scan_run_id, created_at`

// scanScanIssue scans a row into a ScanIssue struct.
func scanScanIssue(scanner interface{ Scan(dest ...any) error }) (*ScanIssue, error) {
	var issue ScanIssue
	err := scanner.Scan(
		&issue.ID,
		&issue.Path,
		&issue.IssueType,
		&issue.Severity,
		&issue.Message,
		&issue.SuggestedAction,
		&issue.ScanRunID,
		&issue.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &issue, nil
}

// InsertScanIssue inserts a new scan issue into the database.
// The path is normalized before storage to prevent duplicates from path variations.
func InsertScanIssue(db *sql.DB, issue ScanIssue) error {
	issue.Path = NormalizePath(issue.Path)

	_, err := db.Exec(
		`INSERT INTO scan_issues (path, issue_type, severity, message, suggested_action, scan_run_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		issue.Path, issue.IssueType, issue.Severity, issue.Message, issue.SuggestedAction, issue.ScanRunID,
	)
	if err != nil {
		return fmt.Errorf("insert scan issue: %w", err)
	}
	return nil
}

// ClearScanIssues removes all scan issues from the database.
func ClearScanIssues(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM scan_issues`)
	if err != nil {
		return fmt.Errorf("clear scan issues: %w", err)
	}
	return nil
}

// ListScanIssues returns all scan issues ordered by id ascending.
// Returns an empty slice (not nil) when no issues exist.
func ListScanIssues(db *sql.DB) ([]ScanIssue, error) {
	rows, err := db.Query(
		`SELECT ` + scanIssueColumns + ` FROM scan_issues ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list scan issues: %w", err)
	}
	defer rows.Close()

	var issues []ScanIssue
	for rows.Next() {
		issue, err := scanScanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("scan scan issue row: %w", err)
		}
		issues = append(issues, *issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scan issues: %w", err)
	}

	if issues == nil {
		issues = []ScanIssue{}
	}
	return issues, nil
}

// ListScanIssuesByPath returns all scan issues for a given path.
// The path is normalized before querying.
// Returns an empty slice (not nil) when no issues match.
func ListScanIssuesByPath(db *sql.DB, path string) ([]ScanIssue, error) {
	path = NormalizePath(path)

	rows, err := db.Query(
		`SELECT `+scanIssueColumns+` FROM scan_issues WHERE path = ? ORDER BY id ASC`,
		path,
	)
	if err != nil {
		return nil, fmt.Errorf("list scan issues by path %s: %w", path, err)
	}
	defer rows.Close()

	var issues []ScanIssue
	for rows.Next() {
		issue, err := scanScanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("scan scan issue row: %w", err)
		}
		issues = append(issues, *issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scan issues by path: %w", err)
	}

	if issues == nil {
		issues = []ScanIssue{}
	}
	return issues, nil
}

// ListScanIssuesByType returns all scan issues of a given type.
// Returns an empty slice (not nil) when no issues match.
func ListScanIssuesByType(db *sql.DB, issueType string) ([]ScanIssue, error) {
	rows, err := db.Query(
		`SELECT `+scanIssueColumns+` FROM scan_issues WHERE issue_type = ? ORDER BY id ASC`,
		issueType,
	)
	if err != nil {
		return nil, fmt.Errorf("list scan issues by type %s: %w", issueType, err)
	}
	defer rows.Close()

	var issues []ScanIssue
	for rows.Next() {
		issue, err := scanScanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("scan scan issue row: %w", err)
		}
		issues = append(issues, *issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scan issues by type: %w", err)
	}

	if issues == nil {
		issues = []ScanIssue{}
	}
	return issues, nil
}
