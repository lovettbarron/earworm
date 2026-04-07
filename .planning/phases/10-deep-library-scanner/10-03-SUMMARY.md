---
phase: 10-deep-library-scanner
plan: 03
subsystem: scanner
tags: [walkdir, deep-scan, library-items, scan-issues, cobra-flag]

requires:
  - phase: 10-deep-library-scanner (plan 01)
    provides: ScanIssue CRUD, scan_issues table, ClearScanIssues
  - phase: 10-deep-library-scanner (plan 02)
    provides: DetectIssues aggregator with 8 heuristic detectors
  - phase: 09-plan-infrastructure-db-schema
    provides: library_items table, UpsertLibraryItem, ListLibraryItems
provides:
  - DeepScanLibrary orchestrator that walks all directories and wires detection to persistence
  - --deep flag on earworm scan command
  - DeepScanResult struct with directory and issue counts
affects: [cli, scanner, future-repair-operations]

tech-stack:
  added: []
  patterns: [filepath.WalkDir for full tree traversal, metadataFn callback injection for testability]

key-files:
  created:
    - internal/scanner/deep.go
    - internal/scanner/deep_test.go
  modified:
    - internal/cli/scan.go
    - internal/cli/scan_test.go
    - internal/cli/cli_test.go

key-decisions:
  - "metadataFn callback pattern for dependency injection in DeepScanLibrary (same pattern as IncrementalSync)"
  - "itemType defaults to 'unknown' for non-audio dirs, 'audiobook' when audio files present"
  - "Deep scan clears all old issues before each run to prevent accumulation"

patterns-established:
  - "DeepScanLibrary follows same callback injection pattern as IncrementalSync for metadata extraction"
  - "scanDeep flag reset in executeCommand test helper to prevent cross-test contamination"

requirements-completed: [SCAN-01, SCAN-03]

duration: 3min
completed: 2026-04-07
---

# Phase 10 Plan 03: Deep Scan Orchestrator and CLI Flag Summary

**DeepScanLibrary orchestrator wiring WalkDir traversal to library_items persistence and issue detection with --deep CLI flag**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-07T18:20:25Z
- **Completed:** 2026-04-07T18:23:42Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- DeepScanLibrary walks all directories (not just ASIN-bearing), populates library_items, runs issue detection, and persists results
- --deep flag added to earworm scan command with summary output showing directory counts and issue breakdown
- Existing scan behavior completely unchanged (regression-safe)

## Task Commits

Each task was committed atomically:

1. **Task 1: DeepScanLibrary orchestrator** - `d7275d9` (test: failing tests) + `54ec5f7` (feat: implementation)
2. **Task 2: CLI --deep flag and output display** - `0310e91` (feat)

## Files Created/Modified
- `internal/scanner/deep.go` - DeepScanLibrary orchestrator, DeepScanResult struct, findAudioFilesInEntries helper
- `internal/scanner/deep_test.go` - 7 tests for deep scan traversal, persistence, error handling
- `internal/cli/scan.go` - --deep flag, runDeepScan function with metadata adapter and summary output
- `internal/cli/scan_test.go` - 3 new tests: TestScanDeep, TestScanDeepShowsIssues, TestScanWithoutDeep_Unchanged
- `internal/cli/cli_test.go` - scanDeep reset in executeCommand helper

## Decisions Made
- metadataFn callback pattern for dependency injection (consistent with IncrementalSync)
- Deep scan clears all old issues before each run (prevents accumulation)
- itemType defaults to "unknown" for non-audio dirs, "audiobook" when audio files present

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Deep scan orchestrator complete, scan issues wired end-to-end
- Ready for future repair/fix operations that consume scan issues
- library_items table now populated with all directories for structural operations

---
*Phase: 10-deep-library-scanner*
*Completed: 2026-04-07*
