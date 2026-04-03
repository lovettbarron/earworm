---
phase: 02-local-library-scanning
plan: 01
subsystem: scanner-infrastructure
tags: [database, scanner, metadata, asin, sqlite, m4a]
dependency_graph:
  requires: [01-01, 01-02]
  provides: [db-extended-schema, scanner-package, metadata-package]
  affects: [02-02]
tech_stack:
  added: [dhowden/tag]
  patterns: [upsert-on-conflict, fallback-chain, two-level-scan, incremental-sync]
key_files:
  created:
    - internal/db/migrations/002_add_metadata_fields.sql
    - internal/scanner/asin.go
    - internal/scanner/scanner.go
    - internal/scanner/asin_test.go
    - internal/scanner/scanner_test.go
    - internal/metadata/metadata.go
    - internal/metadata/tag.go
    - internal/metadata/ffprobe.go
    - internal/metadata/folder.go
    - internal/metadata/metadata_test.go
  modified:
    - internal/db/books.go
    - internal/db/db_test.go
    - go.mod
    - go.sum
decisions:
  - Used INSERT ON CONFLICT for UpsertBook to handle incremental scans without UNIQUE constraint errors
  - Defined BookMetadata struct in scanner package (duplicated from metadata) to avoid circular import; metadata package has its own canonical type
  - dhowden/tag does not provide duration or chapter count for M4A; ffprobe fills that gap
  - Folder name is always the final fallback for metadata extraction
metrics:
  duration: 6m
  completed: 2026-04-03
  tasks: 3
  files: 14
---

# Phase 02 Plan 01: Scanner Infrastructure Summary

Extended database schema with 9 metadata columns, built ASIN extraction with regex matching, two-level and recursive directory scanning, and metadata fallback chain (dhowden/tag -> ffprobe -> folder name parsing) with incremental sync support.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Extend database schema and Book struct | 98ef732 | 002_add_metadata_fields.sql, books.go, db_test.go |
| 2 | Scanner package with ASIN extraction | 1eb477c | asin.go, scanner.go, asin_test.go, scanner_test.go |
| 3 | Metadata extraction with fallback chain | 082a1dd | metadata.go, tag.go, ffprobe.go, folder.go, metadata_test.go, go.mod, go.sum |

## What Was Built

### Database Schema Extension (Task 1)
- Migration 002 adds 9 columns: narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count
- Extended Book struct with all new fields including HasCover bool (stored as INTEGER 0/1 in SQLite)
- UpsertBook function using `INSERT ON CONFLICT(asin) DO UPDATE` for safe incremental scanning
- Added "removed" to ValidStatuses for marking books no longer found on disk
- Refactored scan logic into shared scanBook helper to reduce code duplication

### Scanner Package (Task 2)
- ExtractASIN: regex `B[0-9A-Z]{9}` matches ASINs in brackets, parentheses, and standalone
- ScanLibrary dispatches to scanTwoLevel (default, Author/Title) or scanRecursive (full tree walk)
- scanTwoLevel: reads two directory levels, extracts ASIN from title folders, lists M4A files
- scanRecursive: uses filepath.WalkDir with fs.DirEntry for efficient recursive discovery
- Both modes handle permission errors gracefully (skip + warn, continue scanning)
- IncrementalSync: compares discovered books against DB, inserts new, updates existing, marks missing as "removed"

### Metadata Package (Task 3)
- ExtractMetadata orchestrates the fallback chain: dhowden/tag -> ffprobe -> folder name
- extractWithTag: reads M4A tags for title, author, narrator, genre, year, cover, series
- extractWithFFprobe: subprocess with 30s timeout, JSON parsing, chapter counting, duration
- extractFromFolderName: strips ASIN from folder name, uses parent dir as author
- FindM4AFiles: case-insensitive .m4a extension matching, sorted results

## Test Results

- **DB tests:** 20 tests passing (including UpsertBook insert/update/preserves-created-at, migration 002, removed status)
- **Scanner tests:** 7 tests passing (ASIN extraction table-driven, two-level scan, recursive, permission errors, incremental sync)
- **Metadata tests:** 9 tests passing (M4A finding, folder parsing, invalid file fallback, ffprobe unavailability)
- **Full suite:** 5 packages, all passing, no regressions from Phase 1

## Decisions Made

1. **UpsertBook uses ON CONFLICT** -- avoids UNIQUE constraint errors during incremental scans, preserves created_at on updates
2. **BookMetadata defined in both scanner and metadata packages** -- scanner has its own type for the metadataFn callback; metadata package has the canonical ExtractMetadata return type. Plan 02 will wire them together.
3. **dhowden/tag limitations acknowledged** -- no duration or chapter_count from tag library; ffprobe fills this gap when available
4. **Folder fallback is always available** -- even without M4A files, metadata is extracted from directory naming conventions

## Deviations from Plan

None -- plan executed exactly as written.

## Known Stubs

None -- all functionality is fully implemented and tested.

## Self-Check: PASSED

All 10 created files verified on disk. All 3 task commit hashes (98ef732, 1eb477c, 082a1dd) verified in git log.
