---
phase: 06-integrations-polish
plan: 02
subsystem: cli
tags: [cobra, daemon, audiobookshelf, goodreads, csv, polling]

# Dependency graph
requires:
  - phase: 06-01
    provides: audiobookshelf client and goodreads export packages
  - phase: 04-download-pipeline
    provides: download pipeline with Summary struct
  - phase: 05-file-organization
    provides: organize command and RunE function
provides:
  - earworm notify CLI command for Audiobookshelf scan
  - earworm goodreads CLI command for CSV export
  - earworm daemon CLI command for polling mode
  - ABS auto-scan hook in download pipeline
  - daemon polling package with context cancellation
affects: [testing, documentation]

# Tech tracking
tech-stack:
  added: []
  patterns: [daemon polling with context cancellation, signal.NotifyContext two-stage shutdown, CLI command reuse in daemon cycle]

key-files:
  created:
    - internal/daemon/daemon.go
    - internal/daemon/daemon_test.go
    - internal/cli/notify.go
    - internal/cli/goodreads.go
    - internal/cli/daemon.go
    - internal/audiobookshelf/client.go
    - internal/goodreads/export.go
  modified:
    - internal/config/config.go
    - internal/cli/download.go
    - internal/cli/cli_test.go

key-decisions:
  - "Daemon cycle reuses existing RunE functions (runSync, runDownload, runOrganize) for zero code duplication"
  - "Cycle errors are logged but do not stop the daemon polling loop"
  - "ABS scan hook only triggers when summary.Succeeded > 0 (no scan on zero downloads)"

patterns-established:
  - "Daemon polling: immediate first cycle, then ticker-based interval with context cancellation"
  - "Two-stage signal handling pattern reused from download command for daemon"
  - "Silent skip pattern: ABS operations return nil when URL unconfigured"

requirements-completed: [INT-04, TEST-11]

# Metrics
duration: 3min
completed: 2026-04-05
---

# Phase 6 Plan 02: CLI Commands and Daemon Summary

**Three new CLI commands (notify, goodreads, daemon) wired to ABS/Goodreads packages, with daemon polling loop and ABS auto-scan on download completion**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-05T12:09:30Z
- **Completed:** 2026-04-05T12:12:30Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- Created daemon polling package with context cancellation and cycle error resilience
- Wired three new CLI commands: notify (ABS scan), goodreads (CSV export), daemon (polling mode)
- Added ABS auto-scan hook to download command after successful batch
- All existing tests continue to pass with no regressions

## Task Commits

Each task was committed atomically:

1. **Task 1: Daemon package and config update** - `e99fbe2` (feat)
2. **Task 2: CLI commands and ABS hook** - `7b8c031` (feat)

## Files Created/Modified
- `internal/daemon/daemon.go` - Polling loop with context cancellation and ticker
- `internal/daemon/daemon_test.go` - 4 test cases covering lifecycle and error handling
- `internal/audiobookshelf/client.go` - ABS HTTP client with ScanLibrary (dependency from Plan 01)
- `internal/goodreads/export.go` - CSV export with Goodreads column format (dependency from Plan 01)
- `internal/config/config.go` - Added daemon.polling_interval default (6h)
- `internal/cli/notify.go` - earworm notify command (ABS scan trigger)
- `internal/cli/goodreads.go` - earworm goodreads command (CSV export to stdout/file)
- `internal/cli/daemon.go` - earworm daemon command (sync->download->organize->notify cycle)
- `internal/cli/download.go` - Added ABS scan hook after successful batch
- `internal/cli/cli_test.go` - Flag resets and tests for new commands

## Decisions Made
- Daemon cycle reuses existing RunE functions (runSync, runDownload, runOrganize) for zero code duplication
- Cycle errors are logged but do not stop the daemon polling loop (per D-12)
- ABS scan hook only triggers when summary.Succeeded > 0 (no unnecessary scans)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Created audiobookshelf and goodreads packages inline**
- **Found during:** Task 1 (pre-execution dependency check)
- **Issue:** Plan 01 packages (internal/audiobookshelf, internal/goodreads) did not exist yet as Plan 01 runs in parallel
- **Fix:** Created minimal implementations matching the interface contracts specified in Plan 01/02
- **Files modified:** internal/audiobookshelf/client.go, internal/goodreads/export.go
- **Verification:** All CLI commands compile and tests pass
- **Committed in:** e99fbe2 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Necessary to unblock parallel execution. Implementations match Plan 01 contracts exactly.

## Issues Encountered
- Goodreads CLI test initially failed because temp HOME directory lacked .config/earworm/ subdirectory for SQLite DB creation. Fixed by adding os.MkdirAll in test setup.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all commands are fully wired to their respective packages.

## Next Phase Readiness
- All integration commands functional
- Daemon mode ready for unattended operation
- Ready for documentation and final polish

---
*Phase: 06-integrations-polish*
*Completed: 2026-04-05*
