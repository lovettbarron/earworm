---
phase: 02-local-library-scanning
plan: 02
subsystem: cli
tags: [cobra, cli, scanner, sqlite, spinner, json]

requires:
  - phase: 02-local-library-scanning/01
    provides: scanner.ScanLibrary, scanner.IncrementalSync, metadata.ExtractMetadata, db.UpsertBook, db.ListBooks
  - phase: 01-foundation-configuration
    provides: Cobra CLI framework, Viper config, SQLite DB layer
provides:
  - "earworm scan command with spinner progress and incremental sync"
  - "earworm status command with --json output and author/status filters"
  - "Goroutine-based spinner component for CLI progress feedback"
affects: [03-audible-integration, download-commands, audiobookshelf-sync]

tech-stack:
  added: []
  patterns: [goroutine-spinner-stderr, metadata-adapter-pattern, flag-reset-in-tests]

key-files:
  created:
    - internal/cli/scan.go
    - internal/cli/spinner.go
    - internal/cli/status.go
    - internal/cli/scan_test.go
    - internal/cli/status_test.go
  modified:
    - internal/config/config.go
    - internal/cli/cli_test.go

key-decisions:
  - "Metadata adapter function bridges scanner.BookMetadata and metadata.BookMetadata types"
  - "Package-level flag vars reset in executeCommand test helper to prevent cross-test contamination"
  - "Spinner writes to stderr so stdout piping remains clean for --json output"

patterns-established:
  - "Goroutine spinner: NewSpinner(stderr, msg) -> Start() -> Stop() pattern for long I/O ops"
  - "Error messages with recovery hints: 'what went wrong\\n\\nHow to fix it'"
  - "Flag reset in test helper: reset all package-level flag vars before each executeCommand call"

requirements-completed: [LIB-02, LIB-06, CLI-03, TEST-04]

duration: 4min
completed: 2026-04-04
---

# Phase 02 Plan 02: CLI Commands Summary

**Scan and status CLI commands with goroutine-based spinner progress, --json output, author/status filters, and integration tests**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-04T08:04:26Z
- **Completed:** 2026-04-04T08:08:02Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- `earworm scan` command walks library, extracts metadata via fallback chain, performs incremental sync to SQLite
- `earworm status` displays books in one-line-per-book format with --json, --author, --status flags
- Goroutine-based spinner with live counter on stderr for NAS mount scanning feedback
- 11 integration tests covering scan (error paths, valid scan, recursive, rescan) and status (empty, populated, JSON, fields, filter)

## Task Commits

Each task was committed atomically:

1. **Task 1: Create scan, status, spinner CLI commands** - `e2d6ba7` (feat)
2. **Task 2: Integration tests for scan and status** - `0640f8c` (test)

## Files Created/Modified
- `internal/cli/scan.go` - earworm scan command with spinner, metadata extraction, incremental sync
- `internal/cli/spinner.go` - Goroutine-based spinner with atomic counter for stderr progress
- `internal/cli/status.go` - earworm status command with --json, --author, --status flags
- `internal/cli/scan_test.go` - Integration tests for scan command (6 tests)
- `internal/cli/status_test.go` - Integration tests for status command (5 tests)
- `internal/cli/cli_test.go` - Updated executeCommand helper with flag reset
- `internal/config/config.go` - Added scan.recursive config default

## Decisions Made
- Used adapter function to bridge scanner.BookMetadata and metadata.BookMetadata types (different packages define identical structs)
- Reset package-level Cobra flag variables in executeCommand test helper to prevent state leaking between tests
- Spinner writes to stderr to keep stdout clean for piping/JSON output

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed cross-test flag contamination in test helper**
- **Found during:** Task 2 (integration tests)
- **Issue:** Package-level vars (jsonOutput, filterAuthor, etc.) persisted between tests, causing TestStatusFilterAuthor to get JSON output instead of text
- **Fix:** Added flag var resets (jsonOutput, filterAuthor, filterStatus, scanRecursive) to executeCommand helper
- **Files modified:** internal/cli/cli_test.go
- **Verification:** All 11 tests pass with -race flag
- **Committed in:** 0640f8c (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential for test correctness. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all commands are fully wired to scanner, metadata, and database packages.

## Next Phase Readiness
- Scan and status commands ready for use
- Phase 02 complete: library scanning, metadata extraction, CLI commands all functional
- Ready for Phase 03 (audible-cli integration) which will add download/sync commands

---
*Phase: 02-local-library-scanning*
*Completed: 2026-04-04*
