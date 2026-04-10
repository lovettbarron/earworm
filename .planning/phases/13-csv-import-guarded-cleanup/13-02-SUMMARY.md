---
phase: 13-csv-import-guarded-cleanup
plan: 02
subsystem: cli
tags: [cleanup, trash-dir, audit, cobra, sqlite]

requires:
  - phase: 12-plan-engine-cli
    provides: Plan engine executor, plan/operation DB schema, audit logging
provides:
  - Guarded cleanup command with trash-dir move and double confirmation
  - ListDeleteOperations DB query for completed plan delete ops
  - MoveToTrash utility with EXDEV cross-filesystem fallback
  - CleanupExecutor with ListPending and Execute methods
affects: [14-claude-code-skill]

tech-stack:
  added: []
  patterns: [stdinReader injection for CLI confirmation testing, trash-dir move with timestamp-prefixed unique names]

key-files:
  created:
    - internal/planengine/cleanup.go
    - internal/cli/cleanup.go
    - internal/cli/cleanup_test.go
    - internal/planengine/cleanup_test.go
  modified:
    - internal/config/config.go
    - internal/db/plans.go
    - internal/cli/cli_test.go

key-decisions:
  - "stdinReader package var injection for double-confirmation testing instead of interface abstraction"
  - "Timestamp-prefixed trash names (UnixNano_basename) for uniqueness without collision"

patterns-established:
  - "stdinReader io.Reader package var for CLI confirmation test injection"
  - "createCompletedPlanWithOps test helper for cleanup/plan test setup"

requirements-completed: [FOPS-03]

duration: 5min
completed: 2026-04-10
---

# Phase 13 Plan 02: Guarded Cleanup Summary

**Trash-dir cleanup command with double confirmation, audit logging, and per-plan filtering for completed plan delete operations**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-10T21:00:17Z
- **Completed:** 2026-04-10T21:06:12Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Implemented MoveToTrash with EXDEV cross-filesystem fallback and timestamp-prefixed unique naming
- Added ListDeleteOperations DB query joining plans and operations with status/type/plan-id filtering
- Built earworm cleanup CLI command with double confirmation, --plan-id, --permanent, --json flags
- Full audit trail logging for each trash move operation
- 17 new tests covering cleanup logic, DB queries, and CLI integration

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement cleanup logic with trash-dir move, DB query, and audit logging** - `6ee91aa` (test) + `944f11c` (feat) -- TDD RED/GREEN
2. **Task 2: Wire cleanup CLI command with double confirmation** - `9f5964a` (feat)

## Files Created/Modified
- `internal/planengine/cleanup.go` - MoveToTrash, CleanupExecutor, CleanupResult
- `internal/planengine/cleanup_test.go` - 11 unit tests for cleanup logic
- `internal/cli/cleanup.go` - earworm cleanup command with double confirmation
- `internal/cli/cleanup_test.go` - 6 CLI integration tests
- `internal/config/config.go` - Added cleanup.trash_dir default
- `internal/db/plans.go` - Added ListDeleteOperations query
- `internal/cli/cli_test.go` - Added cleanup flag resets in executeCommand helper

## Decisions Made
- Used stdinReader package var injection for confirmation testing (consistent with cmdFactory pattern used elsewhere)
- Timestamp-prefixed trash names using UnixNano for guaranteed uniqueness without needing random strings
- Cleanup only processes "pending" status ops from "completed" status plans (double safety gate)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- JSON test initially failed because confirmation text preceded JSON output; fixed by extracting JSON portion from output string

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Cleanup command ready for use with existing plan system
- CSV import (13-01) provides plan creation; cleanup (13-02) provides safe deletion
- Phase 14 Claude Code skill can orchestrate both commands

---
*Phase: 13-csv-import-guarded-cleanup*
*Completed: 2026-04-10*
