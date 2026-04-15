---
phase: 13-csv-import-guarded-cleanup
plan: 01
subsystem: cli
tags: [csv, import, planengine, cobra, bom]

requires:
  - phase: 12-plan-engine-cli
    provides: Plan/PlanOperation DB schema, CreatePlan, AddOperation, plan CLI subcommand tree
provides:
  - CSV-to-plan import function (planengine.ImportCSV)
  - BOM stripping for Excel/Google Sheets compatibility (planengine.StripBOM)
  - CSVRowError and CSVImportResult types for structured validation
  - plan import CLI subcommand (earworm plan import FILE.csv)
  - Exported db.IsValidOpType for cross-package validation
affects: [13-csv-import-guarded-cleanup]

tech-stack:
  added: [encoding/csv, bufio]
  patterns: [BOM stripping via bufio.Reader.Peek, case-insensitive header mapping, two-pass CSV validation]

key-files:
  created:
    - internal/planengine/csvimport.go
    - internal/planengine/csvimport_test.go
  modified:
    - internal/db/plans.go
    - internal/cli/plan.go
    - internal/cli/plan_test.go
    - internal/cli/cli_test.go

key-decisions:
  - "Export IsValidOpType for cross-package reuse rather than duplicating validation"
  - "Two-pass validation: collect all errors before deciding whether to create plan"
  - "BOM stripping via bufio.Reader.Peek avoids consuming non-BOM content"

patterns-established:
  - "CSV import pattern: StripBOM -> csv.Reader -> header index map -> validate all rows -> create plan atomically"
  - "Case-insensitive CSV headers via strings.ToLower column index map"

requirements-completed: [PLAN-04]

duration: 3min
completed: 2026-04-10
---

# Phase 13 Plan 01: CSV Import Summary

**CSV-to-plan import with BOM stripping, case-insensitive headers, line-numbered validation errors, and CLI subcommand**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-10T21:00:10Z
- **Completed:** 2026-04-10T21:03:29Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- CSV import function that parses valid CSV into draft plans with one operation per row
- BOM stripping for Excel/Google Sheets compatibility (UTF-8 BOM 0xEF 0xBB 0xBF)
- Row validation with line-numbered errors; no plan created on any validation failure
- CLI subcommand `earworm plan import FILE.csv` with --name and --json flags
- 12 unit tests for CSV import covering valid, BOM, CRLF, missing columns, invalid op types, empty source, move-no-dest, delete-no-dest, extra columns, case-insensitive headers, empty file
- 5 CLI integration tests covering valid import, named import, invalid CSV, missing file, no args

## Task Commits

Each task was committed atomically:

1. **Task 1: Export IsValidOpType and implement CSV import package** (TDD)
   - `32ccedd` (test: failing tests for CSV import)
   - `cdef894` (feat: implement CSV import with BOM stripping and row validation)
2. **Task 2: Wire plan import CLI subcommand** - `ac05bf0` (feat)

## Files Created/Modified
- `internal/planengine/csvimport.go` - ImportCSV, StripBOM, CSVRowError, CSVImportResult types
- `internal/planengine/csvimport_test.go` - 12 unit tests for CSV import logic
- `internal/db/plans.go` - Exported IsValidOpType (was isValidOpType)
- `internal/cli/plan.go` - planImportCmd subcommand with runPlanImport handler
- `internal/cli/plan_test.go` - 5 CLI integration tests for plan import
- `internal/cli/cli_test.go` - planImportName flag reset in executeCommand helper

## Decisions Made
- Exported IsValidOpType for cross-package reuse rather than duplicating validation logic in csvimport.go
- Two-pass validation approach: all rows validated first, plan only created if zero errors (no partial plans)
- BOM stripping uses bufio.Reader.Peek to avoid consuming non-BOM bytes

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all functionality is fully wired.

## Next Phase Readiness
- CSV import is complete; plan 13-02 (guarded cleanup command) can proceed
- IsValidOpType export is available for any future cross-package validation needs

---
*Phase: 13-csv-import-guarded-cleanup*
*Completed: 2026-04-10*
