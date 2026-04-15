---
phase: 15-data-safety-hardening-for-nas-operations
plan: 04
subsystem: planengine
tags: [preflight, disk-space, validation, safety, nas]

requires:
  - phase: 12-plan-engine-cli
    provides: Plan executor with Apply() operation dispatch
  - phase: 15-01
    provides: SHA-256 verified moves and copies in fileops

provides:
  - Pre-flight validation in Apply() checking source existence before execution
  - Free space checking utility (FreeSpace, CheckFreeSpace) in fileops
  - Aggregated error reporting for missing sources
  - Disk space validation with 10% overhead buffer

affects: [planengine, fileops, plan-apply]

tech-stack:
  added: [syscall.Statfs]
  patterns: [preflight-validation, existingAncestor-walk]

key-files:
  created:
    - internal/fileops/diskspace.go
    - internal/fileops/diskspace_test.go
  modified:
    - internal/planengine/engine.go
    - internal/planengine/engine_test.go
    - internal/planengine/engine_split_test.go

key-decisions:
  - "Preflight allows idempotent resume: source missing + dest exists is not flagged as error"
  - "Delete ops skip preflight source check (handled per-op for idempotency)"
  - "existingAncestor walks up directory tree to find valid path for space checks on not-yet-created dirs"
  - "10% buffer added to space requirement for filesystem overhead"

patterns-established:
  - "preflight-validation: Validate all preconditions before executing any destructive operation"
  - "existingAncestor: Walk up directory tree to find existing parent for filesystem queries"

metrics:
  duration: 6min
  completed: 2026-04-11
  tasks: 2
  files: 5
---

# Phase 15 Plan 04: Pre-flight Validation for Plan Apply Summary

Pre-flight source existence and disk space validation in Apply() to prevent partial execution failures on NAS mounts.

## What Was Built

### Task 1: Free space checking utility (TDD)

Created `internal/fileops/diskspace.go` with two functions:
- `FreeSpace(path)` -- returns available bytes using `syscall.Statfs`
- `CheckFreeSpace(path, requiredBytes)` -- validates minimum space with clear error showing shortfall in GB

Tests cover valid paths, invalid paths, sufficient space, and insufficient space scenarios.

### Task 2: Pre-flight validation in Apply() (TDD)

Added `preflightCheck` method to Executor that runs before any operation executes:
- Validates all source files exist for pending move/split/flatten/write_metadata operations
- Skips completed operations (resume support)
- Allows idempotent resume: source missing but dest exists is not flagged (executeOp handles it)
- Checks destination free space for move/split operations with 10% overhead buffer
- Uses `existingAncestor` to walk up to an existing directory for space checks when dest dirs not yet created
- Aggregates all missing files into a single error (not fail-fast on first)

Updated existing tests that relied on per-op missing source handling to expect preflight errors instead.

## Commits

| Hash | Message |
|------|---------|
| e9a86f5 | test(15-04): add failing tests for disk space checking utility |
| adb6ae2 | feat(15-04): implement FreeSpace and CheckFreeSpace disk space utility |
| 6c94577 | test(15-04): add failing tests for Apply() pre-flight validation |
| ffeb3e9 | feat(15-04): add pre-flight validation to Apply() for source existence and disk space |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated existing tests for preflight-first error handling**
- **Found during:** Task 2
- **Issue:** Existing tests (FailedOpContinues, ResumeMissingBoth, SplitFailure) used nonexistent source paths expecting per-op failure. With preflight, these are now caught earlier.
- **Fix:** Updated TestApplyPlan_FailedOpContinues to use a read-only dest dir instead of missing source. Updated ResumeMissingBoth and SplitFailure to expect preflight errors.
- **Files modified:** engine_test.go, engine_split_test.go

**2. [Rule 2 - Critical] Added idempotent resume awareness to preflight**
- **Found during:** Task 2
- **Issue:** Naive preflight would reject valid resume cases where source was already moved to dest.
- **Fix:** For move/split ops, check if dest exists when source is missing -- if so, skip the error (executeOp's resume logic handles it).
- **Files modified:** engine.go

**3. [Rule 3 - Blocking] Added existingAncestor for space checks on uncreated dirs**
- **Found during:** Task 2
- **Issue:** CheckFreeSpace fails on paths that don't exist yet (dest directories created during execution).
- **Fix:** Walk up directory tree to find nearest existing ancestor for statfs call.
- **Files modified:** engine.go

## Known Stubs

None -- all functions are fully wired and tested.

## Verification

All tests pass:
- `go test ./internal/fileops/ -run "TestFreeSpace|TestCheckFreeSpace"` -- 5/5 pass
- `go test ./internal/planengine/ -run "TestApplyPlan_Preflight"` -- 4/4 pass
- `go test ./... -count=1` -- full suite green, no regressions

## Self-Check: PASSED
