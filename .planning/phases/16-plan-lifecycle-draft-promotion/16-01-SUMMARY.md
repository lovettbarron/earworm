---
phase: 16-plan-lifecycle-draft-promotion
plan: 01
subsystem: cli
tags: [cobra, plan-lifecycle, draft-promotion, sqlite]

# Dependency graph
requires:
  - phase: 09-plan-infrastructure-db-schema
    provides: Plan DB schema, CreatePlan, GetPlan, UpdatePlanStatusAudited
provides:
  - "earworm plan approve <id> command for draft-to-ready transition"
  - "import->approve->apply end-to-end plan lifecycle"
affects: [cleanup-command, csv-import, split-plans]

# Tech tracking
tech-stack:
  added: []
  patterns: [plan-lifecycle-state-machine]

key-files:
  created: []
  modified:
    - internal/cli/plan.go
    - internal/cli/plan_test.go

key-decisions:
  - "Reuse existing UpdatePlanStatusAudited for audit trail on approve"
  - "planApproveCmd uses same --json flag pattern as other plan subcommands"

patterns-established:
  - "Plan lifecycle: draft -> approve -> ready -> apply -> completed"

requirements-completed: [PLAN-03, PLAN-04, FOPS-04]

# Metrics
duration: 3min
completed: 2026-04-12
---

# Phase 16 Plan 01: Draft Promotion Summary

**`earworm plan approve <id>` command closing the draft-to-ready gap in plan lifecycle with full TDD coverage and import->approve->apply integration test**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-12T07:01:48Z
- **Completed:** 2026-04-12T07:04:18Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Added `plan approve` Cobra command that transitions draft plans to ready status
- Validates plan exists and is in draft status before approving, with clear error messages
- --json flag for machine-readable output matching other plan subcommand patterns
- End-to-end integration test proving import->approve->apply lifecycle works with real file operations

## Task Commits

Each task was committed atomically:

1. **Task 1: Add plan approve command with --json support** - `394b296` (test: RED), `27b6139` (feat: GREEN)
2. **Task 2: Integration test for import->approve->apply** - `987ccfc` (test)

## Files Created/Modified
- `internal/cli/plan.go` - Added planApproveCmd and runPlanApprove function
- `internal/cli/plan_test.go` - 6 unit tests + 1 integration test for approve command

## Decisions Made
- Reuse existing UpdatePlanStatusAudited (not UpdatePlanStatus) for audit trail consistency
- Same --json flag binding pattern as list/review/apply/import subcommands

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plan lifecycle is now complete: import/split creates drafts, approve promotes to ready, apply executes
- Ready for cleanup command work or Claude Code skill integration

## Self-Check: PASSED

- All files exist: internal/cli/plan.go, internal/cli/plan_test.go, SUMMARY.md
- All commits verified: 394b296, 27b6139, 987ccfc
- Full test suite passes (16 packages, 0 failures)

---
*Phase: 16-plan-lifecycle-draft-promotion*
*Completed: 2026-04-12*
