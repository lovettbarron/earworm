# Phase 16: Plan Lifecycle -- Draft Promotion - Research

**Researched:** 2026-04-12
**Domain:** CLI command addition, plan status lifecycle
**Confidence:** HIGH

## Summary

Phase 16 closes a critical integration gap identified in the v1.1 milestone audit: plans created by `earworm plan import` and `earworm split plan` are born with status "draft", but no CLI command exists to promote them to "ready" -- the only status that `plan apply` accepts. Tests currently bypass this by calling `db.UpdatePlanStatus()` directly, masking the gap from end users.

The fix is straightforward: add an `earworm plan approve <id>` subcommand that transitions a plan from "draft" to "ready" using the existing `db.UpdatePlanStatusAudited()` function. All DB infrastructure already exists -- `ValidPlanStatuses` includes both "draft" and "ready", `UpdatePlanStatusAudited` handles the transition atomically with audit logging, and the `Executor.Apply()` already gates on `plan.Status` being "ready", "running", or "failed".

**Primary recommendation:** Add a single `planApproveCmd` Cobra command in `internal/cli/plan.go` that validates the plan is in "draft" status before calling `db.UpdatePlanStatusAudited(database, planID, "ready")`. No DB changes, no engine changes, no migration needed.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PLAN-03 | User can apply a plan with SHA-256 verification, per-operation status tracking, resume on failure, and full audit trail | Apply itself works -- gap is that draft plans cannot reach "ready" status via CLI. Approve command closes the gap. |
| PLAN-04 | User can import plans from CSV spreadsheets to bridge manual analysis into the plan system | CSV import creates draft plans via `planengine.ImportCSV`. Approve command enables the import->approve->apply flow. |
| FOPS-04 | User can split multi-book folders into separate directories with content-based detection | `split.CreateSplitPlan` creates draft plans via `db.CreatePlan`. Approve command enables the split->approve->apply flow. |
</phase_requirements>

## Standard Stack

No new dependencies required. This phase uses only existing packages.

### Core (already in project)
| Library | Version | Purpose | Used For |
|---------|---------|---------|----------|
| spf13/cobra | v1.10.2 | CLI framework | New `plan approve` subcommand |
| database/sql | stdlib | DB access | Plan status queries |
| encoding/json | stdlib | JSON output | --json flag support |

## Architecture Patterns

### Existing Plan Command Structure
```
internal/cli/plan.go
  planCmd           (parent: "plan")
  planListCmd       ("plan list")
  planReviewCmd     ("plan review <id>")
  planApplyCmd      ("plan apply <id>")
  planImportCmd     ("plan import <file>")
  + planApproveCmd  ("plan approve <id>")  <-- NEW
```

### Pattern: Cobra Subcommand Addition
The project follows a consistent pattern for plan subcommands:

1. Declare a package-level `var planApproveCmd = &cobra.Command{...}` with `RunE`
2. Register in `init()` via `planCmd.AddCommand(planApproveCmd)`
3. Use `config.DBPath()` + `db.Open()` for database access
4. Support `--json` flag for machine-readable output
5. Use `cmd.OutOrStdout()` for all output (testability)

### Plan Status State Machine
```
draft --> ready --> running --> completed
                      |
                      v
                    failed --> running (resume)
```

Key constraints from existing code:
- `db.CreatePlan()` always creates plans with status "draft" (line 75 of plans.go)
- `Executor.Apply()` only accepts "ready", "running", or "failed" (engine.go line 46-49)
- `db.UpdatePlanStatusAudited()` validates status and logs audit entry atomically
- No validation exists in DB layer for valid transitions (any valid status can go to any valid status)

### Approve Command Logic
```go
func runPlanApprove(cmd *cobra.Command, args []string) error {
    planID := parseID(args[0])
    database := openDB()
    
    plan := db.GetPlan(database, planID)
    if plan == nil { return error("not found") }
    if plan.Status != "draft" { return error("can only approve draft plans, current status: %s") }
    
    db.UpdatePlanStatusAudited(database, planID, "ready")
    // output success message
}
```

### Anti-Patterns to Avoid
- **Skipping status validation:** Do not allow approving plans that are already "ready", "running", "completed", or "failed". Only "draft" -> "ready" is valid for approve.
- **Using UpdatePlanStatus instead of UpdatePlanStatusAudited:** The audited version creates a proper audit trail entry atomically.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Status transition with audit | Custom SQL + manual audit log | `db.UpdatePlanStatusAudited()` | Already handles transaction + audit atomically |
| Plan ID parsing | Custom parser | `strconv.ParseInt()` | Same pattern used by review/apply commands |
| DB open boilerplate | Custom setup | `config.DBPath()` + `db.Open()` | Consistent with all other plan commands |

## Common Pitfalls

### Pitfall 1: Forgetting to add flag variables to executeCommand reset
**What goes wrong:** New package-level flag variables (if any) cause cross-test contamination
**Why it happens:** Cobra flags bind to package vars that persist across test runs
**How to avoid:** Any new flag vars must be reset in `executeCommand()` in `cli_test.go`
**Warning signs:** Tests pass individually but fail when run together

### Pitfall 2: Not resetting nested subcommand flags
**What goes wrong:** The `plan approve` subcommand flags don't get reset between tests
**Why it happens:** `cli_test.go` has explicit loops for `planCmd.Commands()` -- new subcommands are automatically covered
**How to avoid:** Verify the existing loop at line 67-74 of `cli_test.go` covers newly added subcommands (it does -- it iterates all `planCmd.Commands()`)

### Pitfall 3: Missing --json support
**What goes wrong:** Machine consumers (Claude Code skill) cannot parse approve output
**Why it happens:** Forgetting the --json flag that all other plan subcommands support
**How to avoid:** Add `--json` flag and JSON output path matching existing commands

### Pitfall 4: Not testing error cases for non-draft plans
**What goes wrong:** Users get confusing errors when trying to approve already-ready or completed plans
**Why it happens:** Only testing the happy path
**How to avoid:** Test approve on "ready", "running", "completed", "failed" plans -- all should return clear errors

## Code Examples

### Existing Pattern: Plan Subcommand (from plan.go)
```go
// Source: internal/cli/plan.go lines 38-48
var planApplyCmd = &cobra.Command{
    Use:   "apply [plan-id]",
    Short: "Apply a plan's operations",
    Args:  cobra.ExactArgs(1),
    RunE:  runPlanApply,
}
```

### Existing Pattern: DB Open + Status Check (from plan.go runPlanApply)
```go
// Source: internal/cli/plan.go lines 182-206
planID, err := strconv.ParseInt(args[0], 10, 64)
dbPath, err := config.DBPath()
database, err := db.Open(dbPath)
defer database.Close()
plan, err := db.GetPlan(database, planID)
if plan == nil { return fmt.Errorf("plan %d not found", planID) }
```

### Existing Pattern: Audited Status Transition (from db/plans.go)
```go
// Source: internal/db/plans.go lines 190-238
// UpdatePlanStatusAudited handles: validate status, capture before state,
// begin TX, update, audit log, commit -- all atomically
err := db.UpdatePlanStatusAudited(database, planID, "ready")
```

### Existing Pattern: Test Setup (from plan_test.go)
```go
// Source: internal/cli/plan_test.go lines 17-40
database := setupPlanTestDB(t)
planID, err := db.CreatePlan(database, "Test plan", "desc")
// Plans start as "draft" -- can test approve directly
out, err := executeCommand(t, "plan", "approve", "1")
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None (stdlib) |
| Quick run command | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PLAN-03 | Approve enables import->approve->apply flow | integration | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` | No -- Wave 0 |
| PLAN-04 | Imported plans can be approved then applied | integration | `go test ./internal/cli/ -run TestPlanImport_Approve_Apply -count=1 -v` | No -- Wave 0 |
| FOPS-04 | Split plans can be approved then applied | integration | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/cli/ -run TestPlan -count=1 -v`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before verify

### Wave 0 Gaps
- [ ] Tests for `plan approve` happy path (draft -> ready)
- [ ] Tests for `plan approve` error cases (non-draft, not found, invalid ID)
- [ ] Tests for `plan approve --json`
- [ ] Integration test: import -> approve -> apply end-to-end

None of these require new test infrastructure -- existing `setupPlanTestDB` and `executeCommand` helpers cover everything needed.

## Sources

### Primary (HIGH confidence)
- `internal/cli/plan.go` -- existing plan commands, all subcommand patterns
- `internal/db/plans.go` -- plan status lifecycle, `UpdatePlanStatusAudited`, `ValidPlanStatuses`
- `internal/planengine/engine.go` -- `Executor.Apply()` status gate (line 46-49: only "ready", "running", "failed")
- `internal/planengine/csvimport.go` -- `ImportCSV` creates draft plans via `db.CreatePlan`
- `internal/split/planner.go` -- `CreateSplitPlan` creates draft plans via `db.CreatePlan`
- `internal/cli/cli_test.go` -- `executeCommand` helper, flag reset patterns
- `internal/cli/plan_test.go` -- existing plan test patterns, `setupPlanTestDB`
- `.planning/v1.1-MILESTONE-AUDIT.md` -- gap identification (PLAN-03, PLAN-04, FOPS-04 all "partial" due to draft-to-ready gap)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all existing code
- Architecture: HIGH -- exact pattern exists in 4 sibling commands
- Pitfalls: HIGH -- well-understood from prior phase work

**Research date:** 2026-04-12
**Valid until:** 2026-05-12 (stable -- internal codebase patterns)
