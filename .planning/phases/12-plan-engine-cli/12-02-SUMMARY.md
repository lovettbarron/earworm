---
phase: 12-plan-engine-cli
plan: 02
subsystem: cli
tags: [cobra, planengine, cli, dry-run, plan-apply]

# Dependency graph
requires:
  - phase: 12-plan-engine-cli
    provides: Executor.Apply(), OpResult types from Plan 01
  - phase: 09-plan-infrastructure-db-schema
    provides: Plan/PlanOperation CRUD (ListPlans, GetPlan, ListOperations)
provides:
  - "earworm plan list" CLI command with --json and --status filters
  - "earworm plan review <id>" CLI command with operation table display
  - "earworm plan apply <id>" with dry-run default and --confirm for execution
affects: [csv-import, cleanup-command, claude-skill]

# Tech tracking
tech-stack:
  added: []
  patterns: [nested-subcommand-flag-reset, dry-run-default-with-confirm]

key-files:
  created:
    - internal/cli/plan.go
    - internal/cli/plan_test.go
  modified:
    - internal/cli/cli_test.go

key-decisions:
  - "Dry-run is the default for plan apply; --confirm flag required for mutation"
  - "Nested subcommand flags need separate reset loop in executeCommand test helper"

patterns-established:
  - "Nested subcommand flag reset: iterate planCmd.Commands() for flag Changed state reset"
  - "Dry-run-by-default pattern: show review output then print Add --confirm message"

requirements-completed: [PLAN-02, PLAN-03]

# Metrics
duration: 3min
completed: 2026-04-10
---

# Phase 12 Plan 02: Plan CLI Commands Summary

**Cobra CLI commands for plan list, review, and apply with dry-run default and planengine.Executor integration**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-10T19:03:10Z
- **Completed:** 2026-04-10T19:06:00Z
- **Tasks:** 1 (TDD: RED + GREEN)
- **Files modified:** 3

## Accomplishments
- Plan list command showing ID, name, status, created date with --json and --status filter support
- Plan review command displaying operation table with seq, type, status, source/destination paths
- Plan apply command defaulting to dry-run; --confirm flag triggers planengine.Executor.Apply with per-operation result output
- 8 CLI integration tests covering all commands and edge cases
- Flag reset block updated in cli_test.go to prevent cross-test contamination

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): Plan CLI tests** - `bff8269` (test)
2. **Task 1 (GREEN): Plan CLI implementation** - `a4193fb` (feat)

_TDD task with RED (failing tests) and GREEN (implementation) commits._

## Files Created/Modified
- `internal/cli/plan.go` - Cobra commands: plan, plan list, plan review, plan apply with flag vars and handlers
- `internal/cli/plan_test.go` - 8 integration tests for plan CLI commands
- `internal/cli/cli_test.go` - Added planConfirm/planJSON/planStatus flag resets and nested subcommand flag reset

## Decisions Made
- Dry-run is the default for plan apply; --confirm flag required for mutation (safe-by-default principle)
- Nested subcommand flags need a separate reset loop in the test helper because rootCmd.Commands() only iterates top-level commands, missing planListCmd/planReviewCmd/planApplyCmd

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None - all commands are fully wired to database and plan engine.

## Issues Encountered
- DB path mismatch in tests: setupPlanTestDB initially placed the DB at a custom path, but config.DBPath() resolves via ConfigDir (~/.config/earworm/). Fixed by placing test DB at the expected config directory path.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Plan list/review/apply commands are functional, completing the scan-to-plan-to-apply workflow
- Ready for CSV import (next plan) which will create plans consumable by these commands

---
*Phase: 12-plan-engine-cli*
*Completed: 2026-04-10*

## Self-Check: PASSED
