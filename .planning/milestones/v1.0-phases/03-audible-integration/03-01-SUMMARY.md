---
phase: 03-audible-integration
plan: 01
subsystem: database
tags: [migration, audible-metadata, sync, upsert]
dependency_graph:
  requires: [internal/db/db.go, internal/db/books.go, internal/db/migrations/002_add_metadata_fields.sql]
  provides: [SyncRemoteBook, ListNewBooks, migration-003]
  affects: [internal/db/books.go, internal/db/db_test.go]
tech_stack:
  added: []
  patterns: [selective-upsert, remote-vs-local-field-preservation]
key_files:
  created:
    - internal/db/migrations/003_add_audible_fields.sql
  modified:
    - internal/db/books.go
    - internal/db/db_test.go
decisions:
  - SyncRemoteBook ON CONFLICT preserves local-only fields (status, local_path, metadata_source, file_count, has_cover, duration, chapter_count) while updating remote metadata
  - ListNewBooks uses audible_status != '' AND (local_path = '' OR status NOT IN ('downloaded', 'organized')) to identify books needing download
metrics:
  duration: 4min
  completed: 2026-04-04
---

# Phase 03 Plan 01: Audible Remote Metadata Schema and Sync Functions Summary

Migration 003 adds 6 Audible metadata columns; SyncRemoteBook upserts remote data preserving local-only fields; ListNewBooks identifies books available remotely but not yet downloaded.

## Tasks Completed

| # | Task | Commit | Key Changes |
|---|------|--------|-------------|
| 1 | Migration 003 + Book struct extension | 384927e, 08d3825 | Migration SQL with 6 ALTER TABLE statements, Book struct extended, allColumns/scanBook/InsertBook/UpsertBook updated |
| 2 | SyncRemoteBook + ListNewBooks | bf8e16a, fab2dec | SyncRemoteBook with selective ON CONFLICT, ListNewBooks with status-aware filtering, comprehensive table-driven tests |

## Implementation Details

### Migration 003
Adds 6 columns to books table: `audible_status`, `purchase_date`, `runtime_minutes`, `narrators`, `series_name`, `series_position`. All with NOT NULL DEFAULT constraints matching the pattern from migration 002.

### SyncRemoteBook
INSERT with hardcoded defaults for local-only fields (`status='unknown'`, `local_path=''`, `metadata_source=''`, `file_count=0`). ON CONFLICT updates only remote metadata fields (title, author, narrator, genre, year, series, audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position) plus updated_at. Local-only fields (status, local_path, metadata_source, file_count, has_cover, duration, chapter_count) are preserved.

### ListNewBooks
Returns books where `audible_status != ''` AND `(local_path = '' OR status NOT IN ('downloaded', 'organized'))`. Books with status `scanned` are included (scanned means the local file was found but the book was not obtained via download pipeline). Returns empty slice not nil.

## Deviations from Plan

None - plan executed exactly as written.

## Decisions Made

1. **Selective upsert pattern**: SyncRemoteBook preserves 7 local-only fields on conflict while updating 12 remote fields. This ensures local scan data is never clobbered by remote sync.
2. **ListNewBooks inclusion of scanned books**: Books with status `scanned` and an `audible_status` set are returned by ListNewBooks because `scanned` indicates local file presence via directory scan, not a completed download.

## Test Coverage

- TestMigration003Applied: verifies schema_versions and new column existence
- TestInsertBookWithAudibleFields: round-trip of all 6 new fields
- TestSyncRemoteBook_NewBook: remote-only book created with correct defaults
- TestSyncRemoteBook_PreservesLocalFields: all 7 local-only fields preserved after sync
- TestListNewBooks: 6-case table-driven test covering inclusion/exclusion logic
- TestListNewBooks_Empty: empty slice not nil guarantee

All existing tests continue to pass (22 total DB tests, full suite `go test ./...` green).

## Known Stubs

None.

## Verification

```
go test ./internal/db/ -count=1    # 22 tests PASS
go test ./... -count=1             # full suite PASS (6 packages)
```
