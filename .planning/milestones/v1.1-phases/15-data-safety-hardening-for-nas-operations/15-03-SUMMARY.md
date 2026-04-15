---
phase: 15-data-safety-hardening-for-nas-operations
plan: 03
subsystem: cli, planengine
tags: [safety, confirmation-prompt, audio-extensions, split-operation]
dependency_graph:
  requires: []
  provides: [permanent-aware-cleanup-prompt, expanded-audio-extension-list]
  affects: [cleanup-command, split-operation]
tech_stack:
  added: []
  patterns: [isAudioExt-helper-function, isPermanent-parameter]
key_files:
  created: []
  modified:
    - internal/cli/cleanup.go
    - internal/cli/cleanup_test.go
    - internal/planengine/engine.go
    - internal/planengine/engine_split_test.go
decisions:
  - "Switch-based isAudioExt helper over inline boolean chain for maintainability"
  - "Double confirmation text changes for both prompts (initial + follow-up) when permanent flag active"
metrics:
  duration: 7min
  completed: 2026-04-11
---

# Phase 15 Plan 03: Fix Permanent Prompt and Expand Split Audio Extensions Summary

Permanent-aware double-confirmation prompt for cleanup and expanded audio extension recognition in split operations to cover .mp3, .ogg, .flac, .wma, .aac, .opus beyond original .m4a/.m4b.

## What Was Done

### Task 1: Fix --permanent confirmation prompt and expand split audio extensions

**Part A - Cleanup prompt fix:**
- Changed `confirmCleanup` signature to accept `isPermanent bool` parameter
- First prompt now shows "PERMANENTLY DELETE N files? This cannot be undone." when --permanent is active
- Second prompt now shows "Files will be PERMANENTLY DELETED." when --permanent is active
- Non-permanent path preserves existing "Move N files to trash" behavior
- Updated call site in `runCleanup` to pass `cleanupPermanent` flag

**Part B - Expanded audio extensions:**
- Extracted inline `ext == ".m4a" || ext == ".m4b"` check into `isAudioExt()` helper function
- Added .mp3, .ogg, .flac, .wma, .aac, .opus to the recognized audio extensions
- These extensions now get VerifiedMove treatment (move with SHA-256 verification) instead of VerifiedCopy during split operations

**Tests added:**
- `TestConfirmCleanup_PermanentPrompt`: verifies PERMANENTLY DELETE text appears
- `TestConfirmCleanup_TrashPrompt`: verifies Move to trash text, no PERMANENTLY keyword
- `TestSplitOp_MP3UsesVerifiedMove`: verifies .mp3 source is moved (deleted from source)
- `TestSplitOp_OGGUsesVerifiedMove`: verifies .ogg source is moved
- `TestSplitOp_JPGUsesVerifiedCopy`: verifies .jpg source is copied (still exists)

## Commits

| # | Hash | Message |
|---|------|---------|
| 1 | 186d23b | test(15-03): add failing tests for permanent prompt and expanded audio extensions |
| 2 | 3f24f4f | feat(15-03): fix permanent prompt and expand split audio extensions |

## Verification

- `go test ./internal/cli/ -run "TestConfirmCleanup" -v -count=1` -- both permanent and trash prompts tested, PASS
- `go test ./internal/planengine/ -run "TestSplitOp" -v -count=1` -- MP3 move, OGG move, JPG copy tested, PASS
- `go test ./... -count=1` -- full suite green (15 packages), no regressions

## Deviations from Plan

None -- plan executed exactly as written.

## Known Stubs

None.
