---
phase: 04-download-pipeline
plan: 01
subsystem: database, subprocess
tags: [sqlite, audible-cli, exec, download, migration]

# Dependency graph
requires:
  - phase: 03-audible-integration
    provides: audible-cli subprocess wrapper (client, cmdFactory, classifyError, buildArgs)
  - phase: 01-project-setup
    provides: SQLite database with migrations framework
provides:
  - Download tracking columns on books table (retry_count, last_error, download_started_at, download_completed_at)
  - UpdateDownloadStart/Complete/Error DB functions for download state management
  - ListDownloadable query for eligible download candidates
  - audible.Download() method invoking audible-cli with correct flags
affects: [04-download-pipeline plans 02-04, file organization]

# Tech tracking
tech-stack:
  added: []
  patterns: [sql.NullTime for nullable datetime columns, goroutine stderr capture with pipe draining before Wait]

key-files:
  created:
    - internal/db/migrations/004_add_download_tracking.sql
    - internal/db/books_test.go
    - internal/audible/download_test.go
  modified:
    - internal/db/books.go
    - internal/audible/download.go
    - internal/audible/audible.go
    - internal/audible/audible_test.go

key-decisions:
  - "sql.NullTime used for nullable download_started_at/download_completed_at with conversion to *time.Time in Book struct"
  - "ListDownloadable uses OR condition: audible_status set AND not downloaded/organized, OR status=error"
  - "Stdout drained line-by-line via bufio.Scanner for future progress parsing extensibility"
  - "Stderr captured in goroutine to prevent pipe deadlock per Go subprocess best practices"

patterns-established:
  - "Nullable datetime pattern: sql.NullTime scan -> *time.Time struct field"
  - "Download pipe management: drain all pipes before cmd.Wait() to prevent deadlock"

requirements-completed: [DL-01, DL-02, DL-03, DL-08, DL-09]

# Metrics
duration: 5min
completed: 2026-04-04
---

# Phase 04 Plan 01: Download Foundation Summary

**audible-cli Download method with --aaxc/--cover/--chapter flags and SQLite download tracking via migration 004**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-04T12:15:15Z
- **Completed:** 2026-04-04T12:20:35Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Migration 004 adds retry_count, last_error, download_started_at, download_completed_at to books table
- Four new DB functions (UpdateDownloadStart/Complete/Error, ListDownloadable) for download state machine
- Real audible-cli Download method replacing stub, with proper subprocess pipe management and error classification
- Full TDD coverage: 9 new DB tests, 7 new download tests, all existing tests preserved

## Task Commits

Each task was committed atomically:

1. **Task 1: DB migration 004 and download tracking functions** - `f0ee704` (feat)
2. **Task 2: Implement audible-cli Download method** - `caf6d8f` (feat)

_Both tasks followed TDD: tests written first (RED), implementation (GREEN)._

## Files Created/Modified
- `internal/db/migrations/004_add_download_tracking.sql` - Migration adding 4 download tracking columns
- `internal/db/books.go` - Book struct extended, 4 new functions, updated scanBook/allColumns
- `internal/db/books_test.go` - 9 tests for download tracking functions and migration
- `internal/audible/download.go` - Real audible-cli download invocation replacing stub
- `internal/audible/download_test.go` - 7 tests for download command construction, error classification, context cancellation
- `internal/audible/audible.go` - Removed stub comment from Download interface method
- `internal/audible/audible_test.go` - Updated fakeCommand to use CommandContext, removed obsolete NotImplemented test, added slow scenario

## Decisions Made
- Used `sql.NullTime` for nullable datetime columns, converting to `*time.Time` in the Book struct for clean Go API
- ListDownloadable includes error-status books (for retry) alongside new/undownloaded books
- Stdout is drained line-by-line via bufio.Scanner to support future progress bar integration
- Stderr is captured in a goroutine with channel synchronization, ensuring all pipes are fully drained before cmd.Wait() (per Go subprocess best practices to prevent deadlock)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed fakeCommand test helper to use CommandContext**
- **Found during:** Task 2 (Download context cancellation test)
- **Issue:** fakeCommand used exec.Command (not CommandContext), so context cancellation was not propagated to test subprocess
- **Fix:** Changed fakeCommand to use exec.CommandContext(ctx, ...) and added "slow" test helper scenario
- **Files modified:** internal/audible/audible_test.go
- **Verification:** TestDownload_ContextCancellation passes in 50ms
- **Committed in:** caf6d8f (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug fix)
**Impact on plan:** Essential for context cancellation test correctness. No scope creep.

## Issues Encountered
None

## Known Stubs
None - all stubs from Phase 3 (ErrNotImplemented in Download) have been replaced with real implementations.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Download method and DB tracking ready for Plan 02 (download pipeline orchestrator)
- ListDownloadable provides the query needed to find books eligible for download
- UpdateDownloadStart/Complete/Error provide the state transitions the orchestrator will call
- Error classification (AuthError, RateLimitError, CommandError) ready for retry logic in Plan 02

---
*Phase: 04-download-pipeline*
*Completed: 2026-04-04*
