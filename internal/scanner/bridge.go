package scanner

import (
	"database/sql"
	"fmt"

	"github.com/lovettbarron/earworm/internal/db"
)

// BridgeResult holds the outcome of creating a plan from scan issues.
type BridgeResult struct {
	PlanID  int64
	Created int // number of operations created
	Skipped int // number of issues skipped (non-actionable)
}

// actionableTypes maps issue types that can be auto-planned to their operation type.
var actionableTypes = map[IssueType]string{
	IssueNestedAudio: "flatten",
	IssueEmptyDir:    "delete",
	IssueOrphanFiles: "delete",
}

// CreatePlanFromIssues translates scan issues into a plan with operations.
// Only actionable issue types get operations (nested_audio -> flatten, empty_dir -> delete,
// orphan_files -> delete). Non-actionable types (no_asin, multi_book, cover_missing,
// missing_metadata, wrong_structure) are skipped because they require human judgment
// or separate workflows.
// Returns BridgeResult with plan ID and counts, or error if no actionable issues found.
func CreatePlanFromIssues(database *sql.DB, issues []db.ScanIssue) (*BridgeResult, error) {
	// Filter to actionable issues
	var actionable []db.ScanIssue
	for _, issue := range issues {
		if _, ok := actionableTypes[IssueType(issue.IssueType)]; ok {
			actionable = append(actionable, issue)
		}
	}

	skipped := len(issues) - len(actionable)

	if len(actionable) == 0 {
		return nil, fmt.Errorf("no actionable issues found for automatic plan creation (%d issues require manual review)", len(issues))
	}

	planID, err := db.CreatePlan(database,
		"scan-issues: auto-plan",
		fmt.Sprintf("Auto-generated from %d scan issues (%d skipped)", len(actionable), skipped))
	if err != nil {
		return nil, fmt.Errorf("create plan from issues: %w", err)
	}

	seq := 1
	for _, issue := range actionable {
		opType := actionableTypes[IssueType(issue.IssueType)]
		_, err := db.AddOperation(database, db.PlanOperation{
			PlanID:     planID,
			Seq:        seq,
			OpType:     opType,
			SourcePath: issue.Path,
			DestPath:   "", // flatten uses source dir; delete has no dest
		})
		if err != nil {
			return nil, fmt.Errorf("add operation for issue %d: %w", issue.ID, err)
		}
		seq++
	}

	return &BridgeResult{
		PlanID:  planID,
		Created: len(actionable),
		Skipped: skipped,
	}, nil
}
