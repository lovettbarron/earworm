---
phase: 04-download-pipeline
plan: 03
subsystem: download
tags: [pipeline, retry, backoff, rate-limiting, error-handling, tdd]

# Dependency graph
requires:
  - phase: 04-01
    provides: "audible-cli Download method with error classification (AuthError, RateLimitError, CommandError)"
  - phase: 04-02
    provides: "RateLimiter, BackoffCalculator, ProgressTracker, staging functions (VerifyM4A, MoveToLibrary, CleanOrphans)"
provides:
  - "Pipeline orchestrator composing all download components into a sequential batch download loop"
  - "PipelineConfig for download settings (staging dir, library dir, rate limit, retries)"
  - "Summary struct with download results, error details, auth/interrupt flags"
  - "downloadWithRetry with exponential backoff and error categorization"
affects: [04-04, download-command]

# Tech tracking
tech-stack:
  added: []
  patterns: [pipeline-orchestrator, testable-func-override, fake-downloader-pattern]

key-files:
  created:
    - internal/download/pipeline.go
    - internal/download/pipeline_test.go
  modified: []

key-decisions:
  - "verifyFunc/sleepFunc function fields on Pipeline struct for test seam injection without interfaces"
  - "Auth errors propagate immediately (no retry) to abort batch fast"
  - "Rate limit errors double the normal backoff delay for longer waits"
  - "Orphan cleanup logs warnings but never aborts the pipeline"

patterns-established:
  - "Pipeline orchestrator pattern: compose components via struct fields with overridable function hooks for testing"
  - "fakeDownloader pattern: configurable per-ASIN errors and file creation for integration testing"

requirements-completed: [DL-07, DL-08, DL-10, TEST-08]

# Metrics
duration: 3min
completed: 2026-04-04
---

# Phase 04 Plan 03: Pipeline Orchestrator Summary

**Download pipeline orchestrator composing rate limiter, backoff, progress tracker, and staging into a sequential batch download loop with retry, auth abort, and DB state tracking**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-04T12:23:38Z
- **Completed:** 2026-04-04T12:27:13Z
- **Tasks:** 1 (TDD: test + implementation)
- **Files modified:** 2

## Accomplishments
- Pipeline.Run() orchestrates full download lifecycle: orphan cleanup, book listing, sequential download with rate limiting, verify, move, DB state tracking
- downloadWithRetry() handles exponential backoff with error categorization: auth abort, rate limit doubled backoff, network retry
- Resume detection reports previously completed books on restart (D-06)
- 12 integration tests covering all error paths with fakeDownloader and in-memory SQLite

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED):** Pipeline tests - `058d51b` (test)
2. **Task 1 (GREEN):** Pipeline implementation - `4541f93` (feat)

## Files Created/Modified
- `internal/download/pipeline.go` - Pipeline orchestrator with Run(), downloadWithRetry(), verifyAndMove(), cleanOrphans(), reportResumeState()
- `internal/download/pipeline_test.go` - 12 integration tests with fakeDownloader, rateLimitFakeDownloader, cancellingFakeDownloader

## Decisions Made
- Used function field overrides (verifyFunc, sleepFunc) on Pipeline struct for test seam injection rather than adding more interfaces
- Auth errors propagate immediately without retry to abort the batch fast
- Rate limit errors use doubled backoff delay (delay * 2) for longer waits
- Orphan cleanup logs warnings but never aborts the pipeline (non-critical path)
- Context cancellation checked between books and between retries for responsive shutdown

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed context cancellation test approach**
- **Found during:** Task 1 (GREEN phase)
- **Issue:** Original test tried to cancel via sleepFunc override during rate limiter wait, but rate limiter uses its own Wait method not sleepFunc
- **Fix:** Created cancellingFakeDownloader that cancels context after first successful Download call
- **Files modified:** internal/download/pipeline_test.go
- **Verification:** TestPipeline_ContextCancellation passes
- **Committed in:** 4541f93 (Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug in test)
**Impact on plan:** Test fix only. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all pipeline functionality is fully wired to real components.

## Next Phase Readiness
- Pipeline ready to be composed into CLI `download` command (Plan 04-04)
- All download components (rate limiter, backoff, progress, staging, pipeline) have comprehensive tests
- Pipeline exports: Pipeline, NewPipeline, PipelineConfig, Summary, BookError

---
*Phase: 04-download-pipeline*
*Completed: 2026-04-04*
