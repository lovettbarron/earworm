---
phase: 04-download-pipeline
plan: 04
subsystem: cli
tags: [cobra, signal-handling, download-pipeline, cli-wiring]

requires:
  - phase: 04-download-pipeline/03
    provides: "Download pipeline orchestrator (Pipeline.Run, PipelineConfig, Summary)"
  - phase: 04-download-pipeline/01
    provides: "Audible client with Download method, error classification"
  - phase: 04-download-pipeline/02
    provides: "Rate limiter, backoff, progress tracker, staging module"
provides:
  - "Fully wired earworm download command with pipeline execution"
  - "Two-stage SIGINT handling (graceful then force exit)"
  - "--limit and --asin CLI flags for batch control"
  - "library_path validation with actionable error message"
  - "Staging path defaulting to ~/.config/earworm/staging"
affects: [05-audiobookshelf, 06-polish]

tech-stack:
  added: []
  patterns: ["Two-stage signal handling via NotifyContext + goroutine listener", "CLI flag filtering applied both in dry-run and pipeline config"]

key-files:
  created: []
  modified:
    - internal/cli/download.go
    - internal/cli/download_test.go
    - internal/cli/cli_test.go
    - internal/download/pipeline.go

key-decisions:
  - "Reuse newAudibleClient var from sync.go for consistent testability pattern"
  - "Filter and limit applied in both dry-run (CLI-side) and real download (PipelineConfig fields)"

patterns-established:
  - "Two-stage SIGINT: signal.NotifyContext for graceful cancel, goroutine for force exit"
  - "Pipeline config extended with Limit/FilterASINs for CLI-driven batch control"

requirements-completed: [DL-06, DL-07, TEST-08]

duration: 2min
completed: 2026-04-04
---

# Phase 04 Plan 04: CLI Download Wiring Summary

**Fully wired earworm download command with two-stage signal handling, pipeline execution, --limit/--asin flags, and config-driven settings**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-04T12:30:10Z
- **Completed:** 2026-04-04T12:32:23Z
- **Tasks:** 1
- **Files modified:** 4

## Accomplishments
- Replaced "not yet implemented" error with full download pipeline execution via Pipeline.Run()
- Added two-stage Ctrl+C handling: first signal gracefully stops batch, second force exits (D-05)
- Added --limit and --asin flags for controlling batch size and filtering specific books
- Added library_path validation with actionable error message pointing to config set command
- Completion summary always prints, even in quiet mode (D-04)

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire download command with signal handling and flags** - `089ddfa` (feat)

## Files Created/Modified
- `internal/cli/download.go` - Full CLI wiring: signal handling, pipeline creation, config integration, summary printing
- `internal/cli/download_test.go` - Tests for no-library-path error, flag registration, limit and ASIN filtering in dry-run
- `internal/cli/cli_test.go` - Added limitN and filterASINs reset in executeCommand helper
- `internal/download/pipeline.go` - Added Limit and FilterASINs fields to PipelineConfig with filtering logic in Run()

## Decisions Made
- Reused the `newAudibleClient` package-level var (from sync.go) for consistent test injection pattern across CLI commands
- Applied ASIN filter and limit both in dry-run path (CLI-side filtering) and real download path (PipelineConfig fields processed in Pipeline.Run) for consistent behavior

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 04 download pipeline is now fully complete
- All download infrastructure wired: audible-cli wrapper, rate limiting, backoff, staging, pipeline orchestrator, and CLI command
- Ready for Phase 05 (Audiobookshelf integration) and Phase 06 (polish)

---
*Phase: 04-download-pipeline*
*Completed: 2026-04-04*
