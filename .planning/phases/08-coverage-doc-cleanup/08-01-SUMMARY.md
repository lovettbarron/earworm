---
phase: 08-coverage-doc-cleanup
plan: 01
subsystem: testing
tags: [go-test, coverage, test-seams, subprocess-mock, mp4-builder]

requires:
  - phase: 07-pipeline-integration-fix
    provides: stable codebase with all functional packages
provides:
  - "80%+ line coverage for 6 packages: metadata, venv, audible, config, db, download"
  - test seams in metadata/ffprobe.go (lookPathFn, execCommandCtx)
  - minimal MP4 builder for tag.go testing without real audio files
affects: [08-02-PLAN, 08-03-PLAN]

tech-stack:
  added: []
  patterns: [TestHelperProcess subprocess mock, minimal MP4 builder for tag tests, test seam injection via package-level vars]

key-files:
  created:
    - internal/audible/errors_test.go
  modified:
    - internal/metadata/ffprobe.go
    - internal/metadata/metadata_test.go
    - internal/venv/venv_test.go
    - internal/config/config_test.go
    - internal/db/db_test.go
    - internal/download/decrypt_test.go
    - internal/download/progress_test.go
    - internal/download/pipeline_test.go
    - internal/download/staging_test.go

key-decisions:
  - "Added lookPathFn and execCommandCtx test seams to ffprobe.go for subprocess mocking"
  - "Built minimal MP4 container in-test (ftyp+moov+udta+meta+ilst atoms) to test extractWithTag success path without real audio files"

patterns-established:
  - "TestHelperProcess pattern: reusable subprocess mock via GO_WANT_HELPER_PROCESS env var"
  - "Minimal MP4 builder: construct valid MP4 containers in tests for dhowden/tag parsing"

requirements-completed: [TEST-12]

duration: 8min
completed: 2026-04-06
---

# Phase 8 Plan 1: Test Coverage Summary

**Raised 6 below-threshold packages to 80%+ line coverage using subprocess mocks, minimal MP4 builders, and error path tests**

## Performance

- **Duration:** 8 min
- **Started:** 2026-04-06T15:53:12Z
- **Completed:** 2026-04-06T16:02:02Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- metadata package: 51.6% -> 94.6% (+43pp) via ffprobe subprocess mocks and minimal MP4 builder for tag.go
- venv package: 41.7% -> 94.4% (+52.7pp) via bootstrap success/failure/cleanup test paths
- audible package: 74.8% -> 85.4% (+10.6pp) via error type tests, classifyError, WithProgressFunc, SetProgressFunc
- config package: 79.4% -> 91.2% (+11.8pp) via validation error paths, WriteDefaultConfig bad path
- db package: 77.6% -> 81.4% (+3.8pp) via invalid status, closed DB, Open invalid path tests
- download package: 79.1% -> 81.2% (+2.1pp) via DefaultCmdFactory, defaultSleep, progress format tests

## Task Commits

Each task was committed atomically:

1. **Task 1: Add tests for metadata and venv packages** - `621d162` (test)
2. **Task 2: Add tests for audible, config, db, download packages** - `253616e` (test)

## Files Created/Modified
- `internal/metadata/ffprobe.go` - Added lookPathFn and execCommandCtx test seams
- `internal/metadata/metadata_test.go` - Added ffprobe mock tests, minimal MP4 builder, tag success/failure tests
- `internal/venv/venv_test.go` - Added bootstrap success/failure/cleanup tests with TestHelperProcess
- `internal/audible/errors_test.go` - New file: all 4 error types, classifyError, WithProgressFunc, SetProgressFunc, command fallback
- `internal/config/config_test.go` - Added validation error paths, WriteDefaultConfig bad path, InitConfig bad file
- `internal/db/db_test.go` - Added invalid status, closed DB, Open invalid path tests
- `internal/download/decrypt_test.go` - Added DefaultCmdFactory direct test
- `internal/download/progress_test.go` - Added FormatBookProgress, FormatSummary, FormatResume, PrintBookProgress, PrintSummary tests
- `internal/download/pipeline_test.go` - Added defaultSleep zero/negative/cancelled/short duration tests
- `internal/download/staging_test.go` - Added VerifyM4A valid M4B test, CleanOrphans non-existent dir test

## Decisions Made
- Added test seams (lookPathFn, execCommandCtx) to ffprobe.go rather than manipulating PATH env var -- more reliable and testable
- Built minimal MP4 containers in-test using raw byte construction (ftyp+moov+udta+meta+ilst atoms) to test extractWithTag success path without needing real audio file fixtures

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Viper does not error on syntactically invalid YAML content -- adjusted TestInitConfigBadFile to use a nonexistent file path instead
- books_test.go already existed with many db test functions -- removed duplicate test definitions to avoid redeclaration errors
- Function signatures for FormatBookProgress, FormatResume, UpdateDownloadComplete, UpdateDownloadError, UpdateOrganizeResult differed from plan assumptions -- corrected to match actual signatures

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All 6 target packages now exceed 80% coverage
- Plan 08-02 (CLI package coverage) can proceed independently
- Plan 08-03 (documentation cleanup and coverage verification gate) depends on 08-01 and 08-02

## Self-Check: PASSED

All 10 files verified present. Both task commits (621d162, 253616e) verified in git log.

---
*Phase: 08-coverage-doc-cleanup*
*Completed: 2026-04-06*
