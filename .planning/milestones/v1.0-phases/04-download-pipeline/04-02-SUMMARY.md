---
phase: 04-download-pipeline
plan: 02
subsystem: download
tags: [rate-limiting, backoff, progress, staging, m4a, tdd, dhowden-tag]

# Dependency graph
requires:
  - phase: 01-foundation-configuration
    provides: config defaults (download.rate_limit_seconds, download.max_retries, download.backoff_multiplier, staging_path)
provides:
  - RateLimiter with configurable delay and context cancellation
  - BackoffCalculator with exponential delays capped at 5 minutes
  - ProgressTracker with D-01 status lines, D-04 summaries, D-06 resume messages, quiet mode
  - Staging module with M4A verification (dhowden/tag), cross-filesystem file moves, orphan cleanup
affects: [04-download-pipeline]

# Tech tracking
tech-stack:
  added: [dhowden/tag (for M4A verification)]
  patterns: [TDD RED-GREEN for pure-logic components, select-based context cancellation, copy+delete cross-filesystem fallback, ASIN pattern matching for safe orphan cleanup]

key-files:
  created:
    - internal/download/ratelimiter.go
    - internal/download/ratelimiter_test.go
    - internal/download/backoff.go
    - internal/download/backoff_test.go
    - internal/download/progress.go
    - internal/download/progress_test.go
    - internal/download/staging.go
    - internal/download/staging_test.go
  modified: []

key-decisions:
  - "ASIN pattern regex (^B[A-Z0-9]{9}$) for safe orphan cleanup - only removes directories matching Audible ASIN format"
  - "Cross-filesystem move uses os.Rename with copy+delete fallback, not os.Link which has same cross-device limitation"
  - "Progress percentage uses -1 sentinel to indicate no percentage available (cleaner than pointer/bool)"

patterns-established:
  - "select-based context cancellation: Wait uses select on time.After and ctx.Done for clean shutdown"
  - "cross-filesystem file move: os.Rename first, fallback to io.Copy + os.Remove"
  - "ASIN-safe directory operations: regex guard before any directory deletion in staging"

requirements-completed: [DL-04, DL-05, DL-06, DL-09, DL-13, TEST-07]

# Metrics
duration: 3min
completed: 2026-04-04
---

# Phase 04 Plan 02: Download Pipeline Components Summary

**TDD-built rate limiter, exponential backoff, progress tracker, and M4A staging module for the download pipeline**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-04T12:15:00Z
- **Completed:** 2026-04-04T12:18:01Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- RateLimiter with configurable delay and context cancellation via select pattern
- BackoffCalculator producing exponential delays (base * multiplier^attempt) capped at 5 minutes
- ProgressTracker formatting D-01 compact status lines, D-04 summaries, D-06 resume messages with quiet mode support
- Staging module: VerifyM4A via dhowden/tag, MoveToLibrary with cross-filesystem fallback, CleanOrphans with ASIN-safe pattern matching

## Task Commits

Each task was committed atomically:

1. **Task 1: Rate limiter and backoff calculator (TDD)** - `3014217` (feat)
2. **Task 2: Progress tracker and staging module (TDD)** - `0e27603` (feat)

## Files Created/Modified
- `internal/download/ratelimiter.go` - Sleep-based rate limiter with context cancellation
- `internal/download/ratelimiter_test.go` - Table-driven tests for RateLimiter
- `internal/download/backoff.go` - Exponential backoff calculator with max cap
- `internal/download/backoff_test.go` - Table-driven tests for BackoffCalculator
- `internal/download/progress.go` - Per-book and batch progress formatting with quiet mode
- `internal/download/progress_test.go` - Tests for all FormatX and PrintX methods
- `internal/download/staging.go` - M4A verification, file move, orphan cleanup
- `internal/download/staging_test.go` - Tests for VerifyM4A, MoveToLibrary, CleanOrphans

## Decisions Made
- Used -1 sentinel for percentage (pct parameter) to indicate "no percentage available" rather than pointer or separate boolean
- ASIN pattern `^B[A-Z0-9]{9}$` ensures CleanOrphans only removes Audible ASIN directories, never arbitrary directories
- Cross-filesystem move tries os.Rename first then falls back to io.Copy + os.Remove (not os.Link which has same cross-device limitation)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all components are fully implemented with no placeholder data.

## Next Phase Readiness
- All four download pipeline components are ready for composition by Plan 03 (Pipeline orchestrator)
- RateLimiter and BackoffCalculator will be used by the retry state machine
- ProgressTracker will be wired to the download command output
- Staging module will be called after each successful audible-cli download

---
*Phase: 04-download-pipeline*
*Completed: 2026-04-04*
