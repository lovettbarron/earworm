---
phase: 10-deep-library-scanner
plan: 02
subsystem: scanner
tags: [issue-detection, heuristics, library-health, pure-functions]

requires:
  - phase: 02-local-library-scanning
    provides: "ExtractASIN, FindAudioFiles, BookMetadata struct"
provides:
  - "8 issue type constants (IssueType) and severity levels"
  - "DetectedIssue struct for reporting library problems"
  - "DetectIssues aggregator function"
  - "8 individual pure-function detectors for library health analysis"
affects: [10-deep-library-scanner]

tech-stack:
  added: []
  patterns: [pure-function-detectors, title-extraction-heuristic]

key-files:
  created:
    - internal/scanner/issues.go
    - internal/scanner/issues_test.go
  modified: []

key-decisions:
  - "Conservative multi-book detection using title extraction before separator, not just stripping numeric prefixes"
  - "detectWrongStructure uses filepath.Rel depth counting rather than separator counting on raw path"
  - "detectNestedAudio is the only detector that touches filesystem beyond provided entries (via metadata.FindAudioFiles)"

patterns-established:
  - "Pure function detector pattern: each detector takes (dirPath, entries, optional meta/root) and returns []DetectedIssue"
  - "Title extraction heuristic: split on ' - ' separator, check if prefix is purely numeric for chapter grouping"

requirements-completed: [SCAN-01]

duration: 3min
completed: 2026-04-07
---

# Phase 10 Plan 02: Issue Detection Heuristics Summary

**8 pure-function issue detectors for library health analysis: no_asin, nested_audio, multi_book, missing_metadata, wrong_structure, orphan_files, empty_dir, cover_missing**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-07T07:04:47Z
- **Completed:** 2026-04-07T07:07:51Z
- **Tasks:** 1
- **Files modified:** 2

## Accomplishments
- Implemented all 8 issue detection heuristics as pure functions in internal/scanner/issues.go
- Conservative multi-book detection that correctly handles numbered chapters (disc1/disc2 pattern)
- DetectIssues aggregator that runs all 8 detectors in a single call
- 29 test cases covering each detector individually plus aggregation

## Task Commits

Each task was committed atomically:

1. **Task 1: Issue type definitions and all 8 detection heuristics** - `c2b3a8b` (test: RED), `dffc446` (feat: GREEN)

**Plan metadata:** pending

_Note: TDD task with RED/GREEN commits_

## Files Created/Modified
- `internal/scanner/issues.go` - IssueType/Severity constants, DetectedIssue struct, 8 detector functions, DetectIssues aggregator, hasAudioFiles helper, extractTitle heuristic
- `internal/scanner/issues_test.go` - 29 test cases: TestDetectEmptyDir, TestDetectNoASIN, TestDetectNestedAudio, TestDetectOrphanFiles, TestDetectCoverMissing, TestDetectMissingMetadata, TestDetectWrongStructure, TestDetectMultiBook, TestDetectIssues_Aggregation

## Decisions Made
- Conservative multi-book detection: files split on " - " separator, purely numeric prefixes group together (avoids false positives on "01 - Chapter One", "02 - Chapter Two" patterns)
- detectWrongStructure uses filepath.Rel for reliable depth computation instead of raw string separator counting
- Known extensions allowlist includes .m4a, .m4b, .jpg, .jpeg, .png, .json, .nfo, .cue, .txt, .log, .xml

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed multi-book title extraction for numbered chapters**
- **Found during:** Task 1 (GREEN phase)
- **Issue:** Initial implementation used numericPrefixPattern which requires separator after digits. Pure numbers like "01" from "01 - Chapter One" didn't match, causing false positives.
- **Fix:** Added purelyNumeric regex check before the prefix pattern to correctly handle bare numbers as track identifiers.
- **Files modified:** internal/scanner/issues.go
- **Verification:** TestDetectMultiBook/same_title_multi-disc_returns_nil now passes
- **Committed in:** dffc446

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Bug fix necessary for conservative multi-book detection. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Issue detection heuristics ready for integration with deep scan orchestrator (Plan 03)
- DetectIssues function provides clean interface for scanning each book directory

---
*Phase: 10-deep-library-scanner*
*Completed: 2026-04-07*
