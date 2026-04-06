---
phase: 07-pipeline-integration-fix
plan: 01
subsystem: download
tags: [pipeline, staging, organize, audiobookshelf]

# Dependency graph
requires:
  - phase: 04-download-pipeline
    provides: download pipeline with verifyAndMove and MoveToLibrary
  - phase: 05-file-organization
    provides: organize command as the correct staging-to-library path
provides:
  - Download pipeline that decrypts, verifies, and leaves files in staging
  - Clean separation between download (staging) and organize (library) steps
  - Empty local_path in DB after download (organize fills it)
affects: [07-02-PLAN, organize, daemon]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Download pipeline ends at staging verification, not library move"
    - "DB local_path empty after download, populated by organize"

key-files:
  created: []
  modified:
    - internal/download/pipeline.go
    - internal/download/staging.go
    - internal/cli/download.go
    - internal/download/pipeline_test.go
    - internal/download/staging_test.go

key-decisions:
  - "MoveToLibrary fully removed from download package; organize is sole move path"
  - "ABS scan removed from download command; daemon cycle handles it after organize step"

patterns-established:
  - "Download leaves files in staging/ASIN/; organize moves to library"
  - "verifyStaged replaces verifyAndMove (decrypt + verify, no move)"

requirements-completed: [ORG-01, ORG-02]

# Metrics
duration: 4min
completed: 2026-04-06
---

# Phase 7 Plan 1: Download Pipeline Fix Summary

**Removed MoveToLibrary from download pipeline so files remain in staging after download, making organize the sole staging-to-library path**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-06T08:43:53Z
- **Completed:** 2026-04-06T08:48:09Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Removed double-move conflict where download moved files to library (flat, wrong structure) before organize could run
- Download pipeline now decrypts AAXC, verifies audio files, and leaves them in staging with status='downloaded' and empty local_path
- Removed MoveToLibrary, sanitizeFolderName, moveFile, copyAndDelete from staging.go (108 lines removed)
- Removed Audiobookshelf scan trigger from download command (daemon cycle handles it after organize)
- All 13 download package tests and all CLI tests updated and passing; full suite green

## Task Commits

Each task was committed atomically:

1. **Task 1: Remove MoveToLibrary from download pipeline and staging** - `250c8d3` (feat)
2. **Task 2: Remove ABS scan from download command, update all download tests** - `1470800` (feat)

## Files Created/Modified
- `internal/download/pipeline.go` - Renamed verifyAndMove to verifyStaged (no library move), UpdateDownloadComplete passes empty local_path
- `internal/download/staging.go` - Removed MoveToLibrary, sanitizeFolderName, moveFile, copyAndDelete; kept VerifyM4A and CleanOrphans
- `internal/cli/download.go` - Removed Audiobookshelf scan trigger block and audiobookshelf import
- `internal/download/pipeline_test.go` - Updated TestPipeline_DownloadCallsDBState (empty LocalPath), TestPipeline_AAXCDecryptIntegration (staging not library), TestPipeline_SequentialDownload (staging dir assertions)
- `internal/download/staging_test.go` - Removed TestMoveToLibrary (function no longer exists)

## Decisions Made
- MoveToLibrary fully removed from download package; organize command is the sole staging-to-library move path
- ABS scan removed from download command; the daemon cycle already triggers ABS scan after the organize step, so having it in download was redundant and premature (files haven't been organized yet)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Download pipeline now correctly leaves files in staging for the organize command
- Plan 07-02 can proceed to wire the full download-then-organize pipeline integration
- Daemon cycle already handles: download -> organize -> ABS scan

---
*Phase: 07-pipeline-integration-fix*
*Completed: 2026-04-06*

## Self-Check: PASSED
