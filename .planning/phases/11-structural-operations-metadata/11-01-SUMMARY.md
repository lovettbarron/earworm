---
phase: 11-structural-operations-metadata
plan: 01
subsystem: fileops
tags: [sha256, hash, verified-move, flatten, file-operations]
dependency_graph:
  requires: [internal/organize/mover.go]
  provides: [internal/fileops/hash.go, internal/fileops/flatten.go]
  affects: [future plan engine file operations]
tech_stack:
  added: [crypto/sha256, encoding/hex]
  patterns: [TDD red-green, per-file error isolation, bottom-up directory cleanup]
key_files:
  created:
    - internal/fileops/hash.go
    - internal/fileops/hash_test.go
    - internal/fileops/flatten.go
    - internal/fileops/flatten_test.go
  modified: []
decisions:
  - "VerifiedMove delegates to organize.MoveFile for actual file moves (reuses cross-FS EXDEV fallback)"
  - "FlattenDir uses filepath.WalkDir for recursive traversal with case-insensitive .m4a/.m4b matching"
  - "Name collisions resolved with _N suffix (up to 999)"
  - "removeEmptyDirs sorts by depth descending for correct bottom-up cleanup"
metrics:
  duration: 2min
  completed: 2026-04-07
---

# Phase 11 Plan 01: SHA-256 Hash, Verified Move, and FlattenDir Summary

SHA-256 file hashing utility, integrity-verified file moves via organize.MoveFile, and recursive directory flattener for nested audiobook directories with collision handling and empty dir cleanup.

## What Was Built

### Task 1: SHA-256 Hash Utility and Verified Move (126c13d)
- `HashFile(path) (string, error)` -- streams file through sha256.New via io.Copy, returns lowercase hex string
- `VerifiedMove(src, dst) error` -- hashes source, moves via organize.MoveFile, hashes destination, errors on mismatch
- Tests: correct hash, not-found, same-FS move, parent dir creation, source-not-found

### Task 2: FlattenDir with Collision Handling (2132ae2)
- `FlattenDir(bookDir) (*FlattenResult, error)` -- walks nested subdirectories, moves .m4a/.m4b to root with SHA-256 verification
- `FlattenResult` and `FileMoveResult` structs for detailed operation reporting
- `uniquePath` handles name collisions with _1, _2, ... suffixes
- `removeEmptyDirs` cleans up empty subdirectories bottom-up (deepest first)
- Per-file error isolation: one failed move does not abort remaining files
- Tests: nested moves, root skip, collisions, deep nesting (3 levels), non-audio ignore, empty dir, deeply nested m4b

## Verification

- `go test ./internal/fileops/ -v -count=1` -- 12 tests pass (5 hash/move + 7 flatten)
- `go vet ./internal/fileops/` -- clean
- `go test ./... -count=1` -- full suite green, 14 packages pass

## Deviations from Plan

None -- plan executed exactly as written.

## Decisions Made

1. VerifiedMove delegates to organize.MoveFile for the actual move (reuses existing cross-FS EXDEV fallback with size verification)
2. FlattenDir uses filepath.WalkDir for recursive discovery with case-insensitive extension matching (consistent with metadata.FindAudioFiles)
3. Name collision suffix pattern is `name_N.ext` (not `name (N).ext`) for filesystem safety
4. removeEmptyDirs sorts by separator count descending to ensure deepest directories are removed first

## Known Stubs

None -- all functions are fully implemented and tested.
