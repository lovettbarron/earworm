---
phase: 10-deep-library-scanner
plan: 01
subsystem: db
tags: [migration, crud, scan-issues, sqlite]
dependency_graph:
  requires: [migration-005]
  provides: [scan-issues-table, scan-issue-crud]
  affects: [internal/db]
tech_stack:
  added: []
  patterns: [scanScanIssue-row-scanner, NormalizePath-on-insert-and-query]
key_files:
  created:
    - internal/db/migrations/006_scan_issues.sql
    - internal/db/scan_issues.go
    - internal/db/scan_issues_test.go
  modified: []
decisions:
  - No FK from scan_issues to library_items -- issues may reference paths not yet tracked
  - INTEGER PRIMARY KEY AUTOINCREMENT for stable ordering in ListScanIssues
metrics:
  duration: 1min
  completed: "2026-04-07T07:06:38Z"
---

# Phase 10 Plan 01: Scan Issues DB Schema Summary

Migration 006 creates scan_issues table with path/type/severity indexes; CRUD functions normalize paths and follow established scanRow/columns pattern from library_items.go.

## What Was Done

### Task 1: Migration 006 and ScanIssue CRUD (TDD)

**RED:** Created 7 failing tests covering migration existence, insert round-trip, list all/by-path/by-type, clear, and path normalization.

**GREEN:** Implemented migration 006 SQL and scan_issues.go with:
- `ScanIssue` struct: ID, Path, IssueType, Severity, Message, SuggestedAction, ScanRunID, CreatedAt
- `InsertScanIssue` -- normalizes path via `NormalizePath` before INSERT
- `ClearScanIssues` -- DELETE FROM scan_issues
- `ListScanIssues` -- SELECT all, ordered by id ASC
- `ListScanIssuesByPath` -- SELECT WHERE path = NormalizePath(path)
- `ListScanIssuesByType` -- SELECT WHERE issue_type = ?
- `scanScanIssue` -- row scanner helper matching established pattern
- `scanIssueColumns` -- shared column constant

**Verification:** All 7 new tests pass. Full `go test ./...` passes with zero regressions across 13 packages.

## Commits

| Hash | Message |
|------|---------|
| 9e0e76a | test(10-01): add failing tests for scan issue CRUD and migration 006 |
| e309769 | feat(10-01): implement ScanIssue CRUD with path normalization |

## Deviations from Plan

None -- plan executed exactly as written.

## Decisions Made

1. **No foreign key to library_items** -- scan issues may reference paths not yet in the library_items table (per research guidance)
2. **AUTOINCREMENT on id** -- provides stable insertion ordering for ListScanIssues ORDER BY id ASC

## Known Stubs

None -- all functions are fully implemented and tested.

## Self-Check: PASSED
