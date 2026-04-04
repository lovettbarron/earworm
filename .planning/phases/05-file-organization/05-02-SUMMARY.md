---
phase: 05-file-organization
plan: 02
subsystem: organize
tags: [file-organization, cli, sqlite, cobra, audiobookshelf-compatible]

requires:
  - phase: 05-01
    provides: "BuildBookPath, RenameM4AFile, MoveFile path construction and file mover"
  - phase: 04-download-pipeline
    provides: "Download pipeline that sets books to 'downloaded' status"
provides:
  - "OrganizeBook function for staging-to-library file moves with correct naming"
  - "OrganizeAll batch orchestrator with per-book error handling"
  - "ListOrganizable and UpdateOrganizeResult DB functions"
  - "earworm organize CLI command with --json and --quiet flags"
affects: [06-integration, audiobookshelf-scan]

tech-stack:
  added: []
  patterns: ["organizer orchestrator with continue-on-error per book", "atomic DB status update with local_path"]

key-files:
  created:
    - internal/organize/organizer.go
    - internal/organize/organizer_test.go
    - internal/cli/organize.go
    - internal/cli/organize_test.go
  modified:
    - internal/db/books.go
    - internal/db/books_test.go
    - internal/cli/cli_test.go

key-decisions:
  - "OrganizeAll continues processing remaining books when one fails (per-book error isolation)"
  - "Cover images (.jpg/.jpeg/.png) all renamed to cover.jpg; chapter JSON to chapters.json"
  - "Staging ASIN dir removed after successful organize (os.Remove ignores non-empty)"

patterns-established:
  - "Organizer pattern: query DB for eligible books, process each, update DB status atomically"
  - "destinationFilename helper for file type routing by extension"

requirements-completed: [ORG-02, TEST-10]

duration: 5min
completed: 2026-04-04
---

# Phase 05 Plan 02: Organizer Orchestrator and CLI Summary

**OrganizeBook moves M4A/cover/chapters from staging to Author/Title [ASIN] library structure with earworm organize CLI command**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-04T17:29:01Z
- **Completed:** 2026-04-04T17:34:01Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- OrganizeBook moves files from staging/ASIN to library/Author/Title [ASIN] with correct file naming (Title.m4a, cover.jpg, chapters.json)
- OrganizeAll batch orchestrator processes all downloaded books, continues on per-book errors, updates DB status to organized or error
- ListOrganizable and UpdateOrganizeResult DB functions for querying and updating book organize state
- earworm organize CLI command with --json output, --quiet support, and missing config validation

## Task Commits

Each task was committed atomically:

1. **Task 1: DB functions and organizer orchestrator** - `baa8834` (feat)
2. **Task 2: earworm organize CLI command with integration tests** - `f8add3a` (feat)

## Files Created/Modified
- `internal/organize/organizer.go` - OrganizeBook, OrganizeAll, OrganizeResult, destinationFilename
- `internal/organize/organizer_test.go` - 9 test functions covering success, failure, partial failure, overwrite, cover rename
- `internal/cli/organize.go` - Cobra organize command with --json flag, text/JSON output
- `internal/cli/organize_test.go` - 6 test functions covering no-config, JSON, empty, registration, text output
- `internal/db/books.go` - ListOrganizable (downloaded status query), UpdateOrganizeResult (atomic status+path update)
- `internal/db/books_test.go` - 6 new test functions for ListOrganizable and UpdateOrganizeResult
- `internal/cli/cli_test.go` - Added organizeJSON flag reset to executeCommand helper

## Decisions Made
- OrganizeAll continues processing remaining books when one fails -- individual book errors do not abort the batch
- Cover images with any image extension (.jpg/.jpeg/.png) are all renamed to cover.jpg per D-05
- Chapter metadata JSON files are renamed to chapters.json per D-06
- Staging ASIN directory is removed after successful organize; failure is silently ignored (may have remaining files)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- File organization feature complete: scan -> sync -> download -> organize pipeline is end-to-end functional
- Ready for Audiobookshelf API scan trigger integration (notify after organize completes)
- Full test suite passes with no regressions

---
*Phase: 05-file-organization*
*Completed: 2026-04-04*
