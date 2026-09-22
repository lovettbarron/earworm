# Phase 12: Plan Engine & CLI - Research

**Researched:** 2026-04-10
**Domain:** CLI plan review/apply engine with file operations, audit trail, and resume-on-failure
**Confidence:** HIGH

## Summary

Phase 12 wires together the plan infrastructure (Phase 9) and file operation primitives (Phase 11) into a user-facing CLI workflow. The core challenge is building a plan execution engine that executes operations sequentially with per-operation status tracking, SHA-256 hash recording, resume-from-failure capability, and full audit logging -- then exposing this through `earworm plan review` and `earworm plan apply` CLI commands.

All building blocks already exist: `db.Plan` and `db.PlanOperation` CRUD (plans.go), `db.AuditEntry` logging (audit.go), `fileops.HashFile`, `fileops.VerifiedMove`, `fileops.FlattenDir`, and `fileops.WriteMetadataSidecar`. The work is: (1) a plan engine package that dispatches operations by type and manages state transitions, (2) CLI commands for reviewing and applying plans, and (3) integration tests proving the resume and audit trail behavior.

**Primary recommendation:** Create an `internal/planengine` package containing an `Executor` struct that iterates plan operations, dispatches to the appropriate fileops function, records SHA-256 hashes and status per operation in the DB and audit log, and supports resume by skipping already-completed operations.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PLAN-02 | User can review a plan via CLI with human-readable diff showing what each action will do before applying | Existing `db.ListOperations` provides operation list; CLI renders source/dest/type table. No new DB functions needed. |
| PLAN-03 | User can apply a plan with SHA-256 verification, per-operation status tracking, resume on failure, and full audit trail | `fileops.HashFile` for SHA-256, `db.UpdateOperationStatus` for tracking, `db.LogAudit`/`db.LogAuditTx` for audit, resume by filtering operations with status != "completed" |
</phase_requirements>

## Standard Stack

### Core (already in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| spf13/cobra | v1.10.2 | CLI commands | Already used for all earworm commands |
| database/sql (stdlib) | Go 1.26 | DB operations | Already used throughout |
| modernc.org/sqlite | v1.48.1 | SQLite driver | Already in go.mod |
| testify/assert | v1.11.1 | Test assertions | Already used project-wide |

### No New Dependencies Required

This phase uses only existing project dependencies. The plan engine is pure Go code orchestrating existing DB functions and fileops primitives. No new libraries needed.

## Architecture Patterns

### Recommended Project Structure
```
internal/
  planengine/
    engine.go           # Executor struct, Apply(), dispatch logic
    engine_test.go      # Unit tests with in-memory DB and temp dirs
  cli/
    plan.go             # earworm plan {review,apply,list} commands
    plan_test.go        # CLI integration tests
```

### Pattern 1: Plan Executor with Operation Dispatch

**What:** A struct that takes a `*sql.DB`, iterates a plan's operations in sequence order, dispatches each to the correct fileops function based on `op_type`, and records results.

**When to use:** Plan application (the `apply` subcommand).

**Example:**
```go
// internal/planengine/engine.go
type Executor struct {
    DB *sql.DB
}

type OpResult struct {
    OperationID int64
    Success     bool
    SHA256      string
    Error       string
}

func (e *Executor) Apply(ctx context.Context, planID int64) ([]OpResult, error) {
    // 1. Validate plan status is "ready" (not draft/completed/running)
    // 2. Set plan status to "running"
    // 3. List operations, skip those with status "completed" (resume)
    // 4. For each pending operation:
    //    a. Set operation status to "running"
    //    b. Dispatch to handler based on op_type
    //    c. Record SHA-256 hash in audit after_state
    //    d. Set operation status to "completed" or "failed"
    //    e. Log audit entry with before/after state
    // 5. After all ops: set plan status to "completed" or "failed"
    // 6. Return results
}
```

### Pattern 2: Cobra Subcommand Group

**What:** `earworm plan` as a parent command with `review`, `apply`, and `list` subcommands.

**When to use:** CLI structure following existing project patterns.

**Example:**
```go
// internal/cli/plan.go
var planCmd = &cobra.Command{
    Use:   "plan",
    Short: "Manage library cleanup plans",
}

var planReviewCmd = &cobra.Command{
    Use:   "review [plan-id]",
    Short: "Review a plan's operations before applying",
    Args:  cobra.ExactArgs(1),
    RunE:  runPlanReview,
}

var planApplyCmd = &cobra.Command{
    Use:   "apply [plan-id]",
    Short: "Apply a plan's operations to the library",
    Args:  cobra.ExactArgs(1),
    RunE:  runPlanApply,
}

var planListCmd = &cobra.Command{
    Use:   "list",
    Short: "List all plans",
    RunE:  runPlanList,
}

func init() {
    planApplyCmd.Flags().BoolVar(&planConfirm, "confirm", false, "actually apply (default is dry-run)")
    planCmd.AddCommand(planReviewCmd, planApplyCmd, planListCmd)
    rootCmd.AddCommand(planCmd)
}
```

### Pattern 3: Resume-on-Failure via Operation Status

**What:** When `Apply()` is called on a plan that was previously interrupted (status "running" or "failed"), it skips operations with status "completed" and resumes from the first "pending" or "failed" operation.

**When to use:** Every apply invocation -- the engine always checks for prior progress.

**Implementation detail:** On resume, reset any "running" operations back to "pending" first (they were mid-flight when interrupted), then process all non-"completed" operations.

### Pattern 4: Dry-Run by Default

**What:** `earworm plan apply` without `--confirm` shows what would happen but makes no changes. Only `--confirm` triggers actual mutations.

**When to use:** Safety-first approach matching the download command's `--dry-run` pattern, but inverted -- dry-run is the default, explicit opt-in for mutation.

### Anti-Patterns to Avoid
- **Processing operations in parallel:** Operations may have dependencies (e.g., flatten before move). Always sequential by `seq` order.
- **Modifying plan_operations schema:** The existing schema has everything needed (status, error_message, completed_at). Don't add hash columns -- store hashes in audit_log after_state JSON.
- **Swallowing operation errors:** Each failed operation must be recorded in both `plan_operations.error_message` AND `audit_log`. A failed operation does NOT abort the plan -- continue with remaining operations (matching the per-book error isolation pattern from Phase 5).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SHA-256 hashing | Custom hasher | `fileops.HashFile()` | Already implemented, tested |
| Verified file moves | Custom move+verify | `fileops.VerifiedMove()` | Already handles cross-filesystem + hash comparison |
| Directory flattening | Custom flatten logic | `fileops.FlattenDir()` | Already handles collisions, empty dir cleanup, per-file SHA-256 |
| Metadata sidecar writing | Custom JSON writer | `fileops.WriteMetadataSidecar()` | Already handles ABS-compatible format |
| Audit logging | Custom log system | `db.LogAudit()` / `db.LogAuditTx()` | Already implemented with entity type/ID tracking |
| Plan status management | Custom state machine | `db.UpdatePlanStatusAudited()` | Already handles status + audit atomically |

**Key insight:** Phase 12 is almost entirely orchestration code -- connecting existing primitives via a dispatch table. The primitives are all tested and working.

## Common Pitfalls

### Pitfall 1: Not Handling "running" Status on Resume
**What goes wrong:** If the process crashes during `Apply()`, the plan status is "running" and some operations are "running". On restart, the engine must detect this and reset "running" ops to "pending".
**Why it happens:** The plan was interrupted mid-execution.
**How to avoid:** At the start of `Apply()`, if plan status is "running", reset any "running" operations to "pending" before proceeding. Allow applying plans with status "ready", "running", or "failed".
**Warning signs:** Tests that only test clean starts, never resume scenarios.

### Pitfall 2: Audit Trail Missing SHA-256 Hashes
**What goes wrong:** The success criteria explicitly require "Applied plans record SHA-256 hashes" in the audit trail. If you only record success/failure, the audit is incomplete.
**Why it happens:** Forgetting that the audit after_state JSON should include the hash.
**How to avoid:** For move/flatten operations, compute hash of destination file and include it in the `AuditEntry.AfterState` JSON. For operations that don't produce files (delete), hash is not applicable.
**Warning signs:** Audit entries with empty after_state.

### Pitfall 3: Review Output Not Being Human-Readable
**What goes wrong:** Just printing raw DB rows is not a "human-readable diff." The success criteria says "source, destination, and operation type."
**Why it happens:** Treating review as a data dump rather than a formatted display.
**How to avoid:** Format review output as a table or aligned list with clear columns: `#`, `Type`, `Source`, `Destination`, `Status`. Use lipgloss if available, but plain text alignment is fine for v1.
**Warning signs:** Review output that's hard to scan visually.

### Pitfall 4: Not Defaulting to Dry-Run
**What goes wrong:** If `earworm plan apply 1` actually applies without `--confirm`, users could accidentally mutate their library.
**Why it happens:** Forgetting the safety-first requirement.
**How to avoid:** Make the apply command check for `--confirm` flag. Without it, print what would happen and exit with a message like "Add --confirm to apply this plan."
**Warning signs:** Apply command that lacks the confirmation check.

### Pitfall 5: Context Cancellation Not Handled
**What goes wrong:** If user presses Ctrl+C during apply, the current operation may leave files in inconsistent state.
**Why it happens:** Not checking `ctx.Done()` between operations.
**How to avoid:** Check context cancellation between operations (not mid-operation -- fileops are atomic). On cancellation, mark remaining operations as "pending" and plan as "failed" with a clear message.
**Warning signs:** Apply function that doesn't accept context.Context.

### Pitfall 6: Package-Level Cobra Flag Variables
**What goes wrong:** Tests contaminate each other because Cobra flag variables persist between tests.
**Why it happens:** Cobra binds flags to package-level variables.
**How to avoid:** Add new flag variables (planConfirm, planJSON, etc.) to the reset block in `executeCommand()` test helper (cli_test.go).
**Warning signs:** Tests passing individually but failing in suite.

## Code Examples

### Operation Dispatch Table
```go
// Dispatch operations by type to the appropriate handler
func (e *Executor) executeOp(ctx context.Context, op db.PlanOperation) OpResult {
    result := OpResult{OperationID: op.ID}
    
    var err error
    var hash string
    
    switch op.OpType {
    case "move":
        err = fileops.VerifiedMove(op.SourcePath, op.DestPath)
        if err == nil {
            hash, _ = fileops.HashFile(op.DestPath)
        }
    case "flatten":
        var fr *fileops.FlattenResult
        fr, err = fileops.FlattenDir(op.SourcePath)
        if err == nil && len(fr.Errors) > 0 {
            err = fr.Errors[0] // report first error
        }
    case "write_metadata":
        // Source is the book dir, metadata built from DB/scan data
        err = e.writeMetadata(op.SourcePath)
    case "delete":
        err = os.Remove(op.SourcePath)
    default:
        err = fmt.Errorf("unknown operation type: %s", op.OpType)
    }
    
    if err != nil {
        result.Error = err.Error()
    } else {
        result.Success = true
        result.SHA256 = hash
    }
    return result
}
```

### Review Output Format
```go
// Format plan review for human readability
func formatReview(plan *db.Plan, ops []db.PlanOperation) string {
    var b strings.Builder
    fmt.Fprintf(&b, "Plan #%d: %s\n", plan.ID, plan.Name)
    fmt.Fprintf(&b, "Status: %s\n", plan.Status)
    fmt.Fprintf(&b, "Operations: %d\n\n", len(ops))
    
    fmt.Fprintf(&b, "%-4s %-16s %-6s %s\n", "#", "Type", "Status", "Details")
    fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 80))
    
    for _, op := range ops {
        detail := op.SourcePath
        if op.DestPath != "" {
            detail += " -> " + op.DestPath
        }
        fmt.Fprintf(&b, "%-4d %-16s %-6s %s\n", op.Seq, op.OpType, op.Status, detail)
    }
    return b.String()
}
```

### Resume Logic
```go
// Reset any "running" operations back to "pending" for resume
func (e *Executor) prepareResume(planID int64) error {
    _, err := e.DB.Exec(
        `UPDATE plan_operations SET status = 'pending', updated_at = CURRENT_TIMESTAMP 
         WHERE plan_id = ? AND status = 'running'`,
        planID,
    )
    return err
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed (stdlib) |
| Quick run command | `go test ./internal/planengine/ ./internal/cli/ -run TestPlan -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PLAN-02 | Review shows human-readable diff with source, dest, op type | unit + integration | `go test ./internal/cli/ -run TestPlanReview -count=1` | No - Wave 0 |
| PLAN-03a | Apply with --confirm executes operations with progress | integration | `go test ./internal/planengine/ -run TestApply -count=1` | No - Wave 0 |
| PLAN-03b | Resume from last successful operation after failure | unit | `go test ./internal/planengine/ -run TestResume -count=1` | No - Wave 0 |
| PLAN-03c | SHA-256 hashes recorded in audit trail | unit | `go test ./internal/planengine/ -run TestAuditHash -count=1` | No - Wave 0 |
| PLAN-03d | Dry-run default (no mutation without --confirm) | integration | `go test ./internal/cli/ -run TestPlanApplyDryRun -count=1` | No - Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/planengine/ ./internal/cli/ -run TestPlan -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/planengine/engine_test.go` -- covers PLAN-03a, PLAN-03b, PLAN-03c
- [ ] `internal/cli/plan_test.go` -- covers PLAN-02, PLAN-03d (CLI integration)

## Project Constraints (from CLAUDE.md)

- **Language:** Go, single binary distribution
- **Database:** modernc.org/sqlite with driver name "sqlite", WAL mode
- **CLI:** Cobra commands in internal/cli/, one file per command, RunE pattern
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests
- **Error handling:** Wrap errors with fmt.Errorf("context: %w", err)
- **Conventions:** Package-level Cobra flag vars must be reset in test helper
- **GSD workflow:** All changes through GSD commands

## Sources

### Primary (HIGH confidence)
- Project codebase: `internal/db/plans.go` -- Plan/PlanOperation CRUD, status validation
- Project codebase: `internal/db/audit.go` -- AuditEntry, LogAudit, LogAuditTx
- Project codebase: `internal/fileops/hash.go` -- HashFile, VerifiedMove
- Project codebase: `internal/fileops/flatten.go` -- FlattenDir, FlattenResult with per-file SHA-256
- Project codebase: `internal/fileops/sidecar.go` -- WriteMetadataSidecar
- Project codebase: `internal/db/migrations/005_plan_infrastructure.sql` -- Schema definition
- Project codebase: `internal/cli/download.go` -- Reference for --dry-run / --confirm pattern
- Project codebase: `internal/cli/scan.go` -- Reference for CLI command structure

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all libraries already in project, no new deps
- Architecture: HIGH - clear dispatch pattern, existing primitives are well-understood
- Pitfalls: HIGH - derived from analysis of existing codebase patterns and success criteria

**Research date:** 2026-04-10
**Valid until:** 2026-05-10 (stable -- all dependencies are project-internal)
