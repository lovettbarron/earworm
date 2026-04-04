---
phase: 05-file-organization
plan: 01
subsystem: organize
tags: [path-construction, file-mover, tdd, sanitization, cross-filesystem]
dependency_graph:
  requires: []
  provides: [SanitizeName, FirstAuthor, BuildBookPath, RenameM4AFile, MoveFile]
  affects: [internal/organize]
tech_stack:
  added: []
  patterns: [table-driven-tests, tdd-red-green-refactor, rune-safe-truncation]
key_files:
  created:
    - internal/organize/path.go
    - internal/organize/path_test.go
    - internal/organize/mover.go
    - internal/organize/mover_test.go
  modified: []
decisions:
  - "Illegal chars regex covers all 9 filesystem-unsafe characters per D-03"
  - "FirstAuthor splits on comma first, then semicolon, then ampersand for consistent multi-author handling"
  - "BuildBookPath validates both before and after sanitization to catch all-illegal-chars edge case"
  - "MoveFile creates parent directories automatically for destination path"
metrics:
  duration: 3min
  completed: "2026-04-04"
  tasks_completed: 2
  tasks_total: 2
  files_created: 4
  files_modified: 0
---

# Phase 05 Plan 01: Path Construction & File Mover Summary

TDD-driven path construction producing Libation-compatible Author/Title [ASIN] paths with rune-safe 255-byte truncation, plus cross-filesystem file mover with size verification before source deletion.

## What Was Built

### Feature 1: Path Construction (internal/organize/path.go)

- **SanitizeName**: Strips 9 illegal filesystem characters (: / \ * ? " < > |), trims whitespace, truncates to 255 bytes at valid UTF-8 rune boundary using `utf8.RuneStart()` check
- **FirstAuthor**: Extracts first author from comma/semicolon/ampersand-separated strings
- **BuildBookPath**: Produces `Author/Title [ASIN]` relative paths, validates non-empty before and after sanitization
- **RenameM4AFile**: Constructs `Title.m4a` filename with 255-byte total limit, falls back to "audiobook.m4a" for empty titles

### Feature 2: Cross-Filesystem File Mover (internal/organize/mover.go)

- **MoveFile**: Tries `os.Rename` first (same-filesystem fast path), falls back to copy+verify+delete on `syscall.EXDEV`
- **copyVerifyDelete**: Copies file, compares sizes, only deletes source after verified match (D-10)
- **copyFile**: Standard io.Copy with source file mode preservation
- Partial destination cleanup on copy failure (D-09)
- Auto-creates parent directories for destination

## Test Coverage

- **path_test.go**: 4 test functions with table-driven subtests covering SanitizeName (7 cases), FirstAuthor (6 cases), BuildBookPath (6 cases), RenameM4AFile (4 cases)
- **mover_test.go**: 7 test functions covering same-fs move, parent dir creation, copy fallback, size verification, cleanup on failure, basic copy, unreadable source

## Commits

| Commit | Type | Description |
|--------|------|-------------|
| c9d36ae | test+feat | Path construction with tests and implementation |
| 2648339 | feat | Cross-filesystem file mover with size verification |

## Deviations from Plan

None -- plan executed exactly as written.

## Known Stubs

None -- all functions are fully implemented with no placeholder logic.

## Verification

```
go test ./internal/organize/... -v  -- PASS (all 11 test functions)
go test ./... -count=1              -- PASS (no regressions across 9 packages)
```

## Self-Check: PASSED

- All 4 created files exist on disk
- Both commits (c9d36ae, 2648339) present in git log
- All required exports found: SanitizeName, FirstAuthor, BuildBookPath, RenameM4AFile, MoveFile
- syscall.EXDEV and utf8.RuneStart present in implementation
