---
phase: 15-data-safety-hardening-for-nas-operations
plan: 01
subsystem: fileops, organize, planengine
tags: [data-safety, fsync, sha256, nas, cross-filesystem]
dependency_graph:
  requires: []
  provides: [fsync-copy-paths, sha256-cross-fs-verification, flatten-error-guard]
  affects: [organize/mover, fileops/copy, fileops/flatten, planengine/cleanup]
tech_stack:
  added: []
  patterns: [fsync-before-close, sha256-verify-before-delete, error-guard-cleanup]
key_files:
  created: [internal/fileops/copy_test.go]
  modified: [internal/organize/mover.go, internal/organize/mover_test.go, internal/fileops/copy.go, internal/fileops/flatten.go, internal/fileops/flatten_test.go, internal/planengine/cleanup.go, internal/planengine/cleanup_test.go]
decisions:
  - Inline hashFileSHA256 in organize and planengine to avoid import cycle with fileops/hash.go
  - fsync added before Close() in all copy paths for NAS write cache safety
metrics:
  duration: 5min
  completed: 2026-04-11
---

# Phase 15 Plan 01: Data Safety Hardening for NAS Operations Summary

Fsync all copy paths and SHA-256 cross-filesystem verification to prevent silent data loss on NAS mounts

## What Was Done

### Task 1: Add fsync to all copy paths and upgrade cross-fs moves to SHA-256

Added `dstFile.Sync()` before `Close()` in three copy functions to flush OS write caches before reporting success -- critical for NFS/SMB mounts where buffered writes can silently fail. Replaced size-only verification with SHA-256 hash comparison in both `copyVerifyDelete` implementations (organize and planengine packages) to detect bit-flip corruption, not just truncation.

**Files modified:**
- `internal/organize/mover.go` -- fsync in copyFile, SHA-256 in copyVerifyDelete, inline hashFileSHA256 helper
- `internal/organize/mover_test.go` -- TestCopyFile_Fsync, TestCopyVerifyDelete_SHA256
- `internal/fileops/copy.go` -- fsync in VerifiedCopy before close and hash verification
- `internal/fileops/copy_test.go` -- TestVerifiedCopy_Fsync (new file)
- `internal/planengine/cleanup.go` -- fsync + SHA-256 in copyVerifyDelete, inline hashFileSHA256 helper
- `internal/planengine/cleanup_test.go` -- TestCleanup_CopyVerifyDelete_SHA256

**Commits:** d8f2df5, 1a47086

### Task 2: Guard FlattenDir against directory cleanup on errors

Added guard to skip `removeEmptyDirs` when any file move error occurred during flattening. Previously, cleanup was unconditional -- if a move failed and the source file remained in a subdirectory, the cleanup could attempt to remove that directory (though `os.Remove` would fail on non-empty dirs, the intent was wrong and edge cases exist with hash-mismatch failures where files could be at destination without source).

**Files modified:**
- `internal/fileops/flatten.go` -- `if len(result.Errors) == 0` guard before removeEmptyDirs
- `internal/fileops/flatten_test.go` -- TestFlattenDir_SkipsCleanupOnError, TestFlattenDir_CleansUpOnSuccess

**Commits:** 5ab2230, b95f1a9

## Deviations from Plan

None -- plan executed exactly as written.

## Decisions Made

1. **Inline hashFileSHA256 in organize and planengine packages** -- fileops/hash.go imports organize (for VerifiedMove), so organize/mover.go cannot import fileops without creating a circular dependency. Same applies to planengine. Each package gets its own unexported `hashFileSHA256` helper.

2. **Fsync placement** -- Sync() is called after io.Copy and before Close() in all paths. The defer Close() remains as a safety net but the explicit Close() after Sync() is the primary path.

## Verification Results

All tests pass across all three packages:
- `go test ./internal/organize/` -- PASS
- `go test ./internal/fileops/` -- PASS
- `go test ./internal/planengine/` -- PASS

Pattern verification:
- `dstFile.Sync()` present in organize/mover.go, fileops/copy.go, planengine/cleanup.go
- `hashFileSHA256` present in organize/mover.go, planengine/cleanup.go
- `len(result.Errors) == 0` guard in fileops/flatten.go
- Size-only checks (`srcInfo.Size() != dstInfo.Size()`) removed from both copyVerifyDelete implementations

## Known Stubs

None -- all implementations are complete and wired.

## Self-Check: PASSED

All 8 key files verified present. All 4 commit hashes verified in git log.
