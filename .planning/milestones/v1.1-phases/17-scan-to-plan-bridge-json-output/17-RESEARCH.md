# Phase 17: Scan-to-Plan Bridge & JSON Output - Research

**Researched:** 2026-04-12
**Domain:** CLI subcommand wiring, scan issue listing, plan creation from issues, JSON serialization
**Confidence:** HIGH

## Summary

Phase 17 bridges two existing subsystems that are already fully built but not connected: the deep scanner (Phase 10) which persists scan issues to the `scan_issues` table, and the plan engine (Phase 12) which creates and executes plans. The work is straightforward CLI wiring and a new "scan bridge" package that translates scan issues into plan operations.

All required primitives already exist. The `db.ListScanIssues()`, `db.ListScanIssuesByType()`, `db.CreatePlan()`, and `db.AddOperation()` functions are implemented and tested. The JSON output pattern is well-established across multiple commands (`status --json`, `plan list --json`, `split detect --json`). The `split.CreateSplitPlan()` function provides an exact template for the issue-to-plan creation logic.

**Primary recommendation:** Add `earworm scan issues` as a subcommand of `scan`, add `--create-plan` flag to create a plan from issues, add `--json` flag to `scan --deep`, and follow the established `split.CreateSplitPlan` pattern for plan creation logic.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCAN-01 | Deep scan all library folders and detect issues | Already implemented in Phase 10. This phase adds the `scan issues` listing CLI and `--json` output for deep scan. |
| SCAN-03 | Detected scan issues persisted in DB with severity, category, and suggested action | Already implemented in Phase 10. This phase adds the bridge from persisted issues to plan creation. |
| INTG-02 | Claude Code skill enables conversational plan creation | Already implemented in Phase 14. This phase adds `--json` output to `scan --deep` which the skill already references (`earworm scan --deep --json`). |
</phase_requirements>

## Standard Stack

No new dependencies required. This phase uses only existing packages:

### Core (Already in Project)
| Package | Purpose | Already Used In |
|---------|---------|-----------------|
| `internal/db` | ScanIssue CRUD, Plan CRUD, AddOperation | scanner, planengine, cli |
| `internal/scanner` | IssueType constants, DetectedIssue struct | cli/scan.go |
| `encoding/json` | JSON output serialization | cli/status.go, cli/plan.go, cli/split.go |
| `github.com/spf13/cobra` | CLI subcommand structure | all cli/*.go |

### No New Dependencies
This phase requires zero new Go modules. All work is internal wiring.

## Architecture Patterns

### Recommended Project Structure

No new packages needed. Changes go into existing files plus one new bridge module:

```
internal/
  cli/
    scan.go          # MODIFY: add `scan issues` subcommand, --json flag, --create-plan
    scan_test.go     # MODIFY: add tests for new subcommands
    cli_test.go      # MODIFY: reset new flag vars in executeCommand helper
  scanner/
    bridge.go        # NEW: CreatePlanFromIssues() - translates scan issues to plan ops
    bridge_test.go   # NEW: unit tests for bridge logic
```

### Pattern 1: Subcommand Registration (Established)

The `split` command demonstrates the exact pattern for nested subcommands under an existing parent:

```go
// scan.go - add subcommand to existing scanCmd
var scanIssuesCmd = &cobra.Command{
    Use:   "issues",
    Short: "List detected issues from the last deep scan",
    RunE:  runScanIssues,
}

func init() {
    scanIssuesCmd.Flags().BoolVar(&scanIssuesJSON, "json", false, "output in JSON format")
    scanIssuesCmd.Flags().BoolVar(&scanCreatePlan, "create-plan", false, "create a plan from detected issues")
    scanIssuesCmd.Flags().StringVar(&scanFilterType, "type", "", "filter issues by type")
    scanCmd.AddCommand(scanIssuesCmd)
}
```

**Important:** When `scanCmd` gets subcommands added, it should keep its existing `RunE: runScan` so bare `earworm scan` still works. Cobra handles this correctly -- a command can have both `RunE` and subcommands.

### Pattern 2: JSON Output (Established)

Every CLI command follows the same JSON pattern. The `--json` flag outputs via `json.NewEncoder` with `SetIndent("", "  ")`:

```go
if scanIssuesJSON {
    enc := json.NewEncoder(cmd.OutOrStdout())
    enc.SetIndent("", "  ")
    return enc.Encode(issues)
}
```

For `scan --deep --json`, the deep scan result and issues should be combined into a single JSON object.

### Pattern 3: Plan Creation from External Source (Established)

`split.CreateSplitPlan()` is the exact template:
1. Create plan with `db.CreatePlan(database, name, description)`
2. Add operations with `db.AddOperation(database, op)` in sequence
3. Plan is created in "draft" status
4. Return plan ID

### Issue-to-Operation Mapping

This is the core logic of the bridge. Each scan issue type maps to a plan operation type:

| Issue Type | Operation Type | Source | Destination | Notes |
|------------|---------------|--------|-------------|-------|
| `nested_audio` | `flatten` | audio files in subdirs | book directory level | FlattenDir already implemented |
| `multi_book` | (skip) | - | - | Requires `split detect` for per-book grouping; auto-plan is too risky |
| `no_asin` | (skip/manual) | - | - | Cannot auto-determine ASIN; flag for manual review |
| `missing_metadata` | `write_metadata` | book directory | metadata.json sidecar | Phase 18 wires this fully |
| `wrong_structure` | `move` | current path | correct Author/Title path | Needs metadata to build correct path |
| `orphan_files` | `delete` | orphan file path | - | Low severity, optional cleanup |
| `empty_dir` | `delete` | empty directory | - | Safe to auto-plan |
| `cover_missing` | (skip) | - | - | Cannot auto-generate cover |

**Key design decision:** Not all issue types can be auto-planned. The bridge should:
1. Only create operations for issues with clear automated remediation (`nested_audio`, `empty_dir`, `orphan_files`)
2. Include `wrong_structure` and `missing_metadata` only when sufficient metadata exists to determine the correct destination
3. Skip `multi_book`, `no_asin`, and `cover_missing` (these need human intervention or separate workflows)
4. Report skipped issues in the output so users know what still needs manual attention

### Pattern 4: Flag Variable Reset in Tests (Critical)

The `executeCommand` helper in `cli_test.go` must reset ALL package-level flag variables. Any new flags added (e.g., `scanIssuesJSON`, `scanCreatePlan`, `scanFilterType`) MUST be added to the reset list, and new subcommand flag loops must be added for `scanCmd.Commands()`.

### Anti-Patterns to Avoid

- **Adding scan issues as a top-level command:** Keep it under `scan` for discoverability. `earworm scan issues` is more intuitive than `earworm issues`.
- **Auto-planning multi_book issues:** The split workflow already exists with its mandatory detect-then-approve gate. Don't bypass it.
- **Mixing human-readable and JSON output:** When `--json` is set, ALL output must go through the JSON encoder. No `fmt.Fprintf` to stdout.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Issue listing from DB | Custom SQL queries | `db.ListScanIssues()`, `db.ListScanIssuesByType()` | Already implemented and tested |
| Plan creation | Manual INSERT statements | `db.CreatePlan()` + `db.AddOperation()` | Handles audit logging automatically |
| JSON serialization | Custom string building | `encoding/json` with `json.NewEncoder` | Standard, consistent with rest of codebase |
| Issue type constants | String literals | `scanner.IssueType` constants | Already defined as typed constants |

## Common Pitfalls

### Pitfall 1: Cobra Parent Command with RunE and Subcommands
**What goes wrong:** Adding subcommands to `scanCmd` might break the bare `earworm scan` behavior.
**Why it happens:** Some Cobra patterns require removing `RunE` when adding subcommands.
**How to avoid:** Cobra supports both simultaneously. Keep `RunE: runScan` on `scanCmd`. When subcommands are registered, `earworm scan` still calls `runScan`, and `earworm scan issues` calls the subcommand. Test both paths.
**Warning signs:** `earworm scan` stops working or shows help instead of scanning.

### Pitfall 2: Flag Variable Contamination in Tests
**What goes wrong:** New flag variables are not reset in `executeCommand()`, causing test A's flags to leak into test B.
**Why it happens:** Package-level flag variables persist across test runs in the same process.
**How to avoid:** Add every new flag variable to the reset block in `cli_test.go`'s `executeCommand()`. Also add `scanCmd.Commands()` flag reset loop.
**Warning signs:** Tests pass individually but fail when run together.

### Pitfall 3: Creating Plans with Zero Operations
**What goes wrong:** If no issues are actionable, `--create-plan` creates an empty plan.
**Why it happens:** All detected issues might be types that can't be auto-planned (no_asin, multi_book, cover_missing).
**How to avoid:** Check that at least one operation would be created before calling `db.CreatePlan()`. Return a clear message: "No actionable issues found for automatic plan creation."
**Warning signs:** Empty plans in `plan list` output.

### Pitfall 4: JSON Output for Deep Scan Missing Issues Detail
**What goes wrong:** `scan --deep --json` returns only summary counts, not the actual issues.
**Why it happens:** The current `DeepScanResult` struct only has `IssueCounts` (a map), not the full issue list.
**How to avoid:** For JSON output, query `db.ListScanIssues()` after the deep scan completes and include full issues in the JSON response. The deep scan already persists all issues to the DB.
**Warning signs:** Claude Code skill can't parse individual issues from scan output.

### Pitfall 5: Incorrect Operation Source Paths
**What goes wrong:** Plan operations reference the directory path from the scan issue, not the actual file paths inside the directory.
**Why it happens:** Scan issues store the directory path, but operations need file-level paths for moves/flattens.
**How to avoid:** The bridge must read the directory contents to build file-level operations. For `nested_audio`, it needs to find the actual audio files in subdirectories. For `empty_dir`, the directory path itself is the source.
**Warning signs:** Plan apply fails with "source not found" errors.

## Code Examples

### Scan Issues Subcommand (CLI Pattern)

```go
// Source: established pattern from cli/split.go and cli/plan.go
func runScanIssues(cmd *cobra.Command, args []string) error {
    dbPath, err := config.DBPath()
    if err != nil {
        return fmt.Errorf("failed to determine database path: %w", err)
    }
    database, err := db.Open(dbPath)
    if err != nil {
        return fmt.Errorf("failed to open database: %w", err)
    }
    defer database.Close()

    var issues []db.ScanIssue
    if scanFilterType != "" {
        issues, err = db.ListScanIssuesByType(database, scanFilterType)
    } else {
        issues, err = db.ListScanIssues(database)
    }
    if err != nil {
        return fmt.Errorf("failed to list scan issues: %w", err)
    }

    if scanCreatePlan {
        return createPlanFromIssues(cmd, database, issues)
    }

    // JSON or human-readable output...
}
```

### Bridge Function (Plan Creation Pattern)

```go
// Source: established pattern from split/planner.go
// scanner/bridge.go
func CreatePlanFromIssues(database *sql.DB, issues []db.ScanIssue) (planID int64, skipped int, err error) {
    // Filter to actionable issues
    var actionable []db.ScanIssue
    for _, issue := range issues {
        switch issue.IssueType {
        case string(IssueNestedAudio), string(IssueEmptyDir), string(IssueOrphanFiles):
            actionable = append(actionable, issue)
        // wrong_structure and missing_metadata need metadata lookup
        }
    }
    
    if len(actionable) == 0 {
        return 0, len(issues), fmt.Errorf("no actionable issues found")
    }
    
    planID, err = db.CreatePlan(database, "scan-issues: auto-plan", 
        fmt.Sprintf("Auto-generated from %d scan issues", len(actionable)))
    if err != nil {
        return 0, 0, err
    }
    
    seq := 1
    for _, issue := range actionable {
        // Build operations based on issue type...
        seq++
    }
    
    return planID, len(issues) - len(actionable), nil
}
```

### Deep Scan JSON Output Structure

```go
// JSON output for `earworm scan --deep --json`
type DeepScanJSON struct {
    TotalDirs   int              `json:"total_dirs"`
    WithASIN    int              `json:"with_asin"`
    WithoutASIN int              `json:"without_asin"`
    IssuesFound int              `json:"issues_found"`
    IssueCounts map[string]int   `json:"issue_counts"`
    Issues      []ScanIssueJSON  `json:"issues"`
}

type ScanIssueJSON struct {
    ID              int64  `json:"id"`
    Path            string `json:"path"`
    IssueType       string `json:"issue_type"`
    Severity        string `json:"severity"`
    Message         string `json:"message"`
    SuggestedAction string `json:"suggested_action"`
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.11.1 |
| Config file | None (Go convention) |
| Quick run command | `go test ./internal/cli/ ./internal/scanner/ -run TestScan -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCAN-01 | `scan issues` lists persisted issues | integration | `go test ./internal/cli/ -run TestScanIssues -count=1` | Wave 0 |
| SCAN-01 | `scan issues --type` filters by type | integration | `go test ./internal/cli/ -run TestScanIssuesFilter -count=1` | Wave 0 |
| SCAN-03 | `scan issues --create-plan` creates plan from issues | integration | `go test ./internal/cli/ -run TestScanIssuesCreatePlan -count=1` | Wave 0 |
| SCAN-03 | Bridge skips non-actionable issues | unit | `go test ./internal/scanner/ -run TestCreatePlanFromIssues -count=1` | Wave 0 |
| SCAN-03 | Created plan is in draft status | unit | `go test ./internal/scanner/ -run TestBridgePlanDraft -count=1` | Wave 0 |
| INTG-02 | `scan --deep --json` produces valid JSON | integration | `go test ./internal/cli/ -run TestScanDeepJSON -count=1` | Wave 0 |
| INTG-02 | JSON includes full issue details | integration | `go test ./internal/cli/ -run TestScanDeepJSONIssues -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/cli/ ./internal/scanner/ -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/scanner/bridge.go` -- new file, plan creation from issues
- [ ] `internal/scanner/bridge_test.go` -- unit tests for bridge logic
- [ ] Tests in `internal/cli/scan_test.go` -- integration tests for new subcommands
- [ ] Flag reset additions in `internal/cli/cli_test.go` -- new flag variables

## Sources

### Primary (HIGH confidence)
- Existing codebase: `internal/db/scan_issues.go` -- full ScanIssue CRUD already implemented
- Existing codebase: `internal/db/plans.go` -- full Plan/PlanOperation CRUD already implemented
- Existing codebase: `internal/cli/scan.go` -- deep scan CLI with spinner and summary output
- Existing codebase: `internal/cli/plan.go` -- JSON output patterns for all plan subcommands
- Existing codebase: `internal/cli/split.go` -- subcommand registration and plan creation pattern
- Existing codebase: `internal/split/planner.go` -- `CreateSplitPlan` as template for bridge
- Existing codebase: `.claude/skills/earworm/SKILL.md` -- Claude Code skill already references `earworm scan --deep --json`

### Secondary (MEDIUM confidence)
- Cobra documentation: parent commands with both `RunE` and subcommands work correctly

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all primitives exist
- Architecture: HIGH -- follows established patterns exactly (split, plan CLI)
- Pitfalls: HIGH -- all pitfalls are from patterns observed in this codebase (flag contamination, empty plans)

**Research date:** 2026-04-12
**Valid until:** 2026-05-12 (stable -- internal wiring only)
