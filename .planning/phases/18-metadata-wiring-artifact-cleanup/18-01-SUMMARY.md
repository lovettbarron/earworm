---
phase: 18-metadata-wiring-artifact-cleanup
plan: 01
subsystem: planengine, db
tags: [metadata, sidecar, write_metadata, resolution]
dependency_graph:
  requires: [db.Book, db.LibraryItem, metadata.ExtractMetadata, fileops.BuildABSMetadata, scanner.ExtractASIN]
  provides: [db.GetBookByLocalPath, planengine.resolveBookMetadata]
  affects: [planengine.executeOp write_metadata case]
tech_stack:
  added: []
  patterns: [layered-metadata-resolution, db-to-metadata-adapter]
key_files:
  created: []
  modified:
    - internal/db/books.go
    - internal/db/books_test.go
    - internal/planengine/engine.go
    - internal/planengine/engine_test.go
decisions:
  - "resolveBookMetadata uses 4-layer fallback: DB local_path -> library_items ASIN -> file extraction -> empty"
  - "bookToMetadata is a standalone function (not method) for reuse"
metrics:
  duration: 2min
  completed: 2026-04-12
  tasks: 2
  files: 4
---

# Phase 18 Plan 01: Metadata Wiring for Plan Engine Summary

Wire real book metadata into plan engine write_metadata operations via layered DB-first resolution with graceful fallback to empty.

## What Was Done

### Task 1: GetBookByLocalPath DB function
- Added `GetBookByLocalPath` to `internal/db/books.go` following the existing `GetBook` pattern
- Normalizes input with `filepath.Clean` before querying (strips trailing slashes)
- Returns `(nil, nil)` for not-found (consistent with `GetBook` contract)
- 3 tests: found, not-found, path normalization
- Commit: 1d1e01b

### Task 2: resolveBookMetadata and write_metadata wiring
- Added `bookToMetadata` helper to convert `db.Book` to `metadata.BookMetadata`
- Added `resolveBookMetadata` method on `*Executor` with 4-layer fallback chain:
  1. DB lookup by `local_path` via `GetBookByLocalPath`
  2. `library_items` -> `books` lookup by ASIN
  3. File-based metadata extraction via `metadata.ExtractMetadata`
  4. Empty `BookMetadata` with ASIN parsed from folder name
- Replaced the empty-skeleton `write_metadata` case with `resolveBookMetadata` + `BuildABSMetadata`
- Added `metadata` and `scanner` imports (no import cycle -- scanner only imports `regexp`)
- 4 tests: DB book produces real metadata.json, empty fallback produces valid structure, direct resolve with DB, ASIN extraction from folder name
- Commit: 906d7f6

## Deviations from Plan

None -- plan executed exactly as written.

## Verification

- `go test ./internal/db/ -run TestGetBookByLocalPath -v` -- PASS (3/3)
- `go test ./internal/planengine/ -run "TestWriteMetadata|TestResolveBookMetadata" -v` -- PASS (4/4)
- `go test ./...` -- all 16 packages PASS, no import cycles, no regressions

## Known Stubs

None -- all metadata fields are wired from real DB data or legitimate fallback chain.
