---
phase: quick
plan: 260404-pw1
subsystem: venv-management
tags: [audible-cli, python, venv, dependency-management]
dependency_graph:
  requires: [internal/config]
  provides: [internal/venv]
  affects: [internal/cli/sync.go, internal/cli/auth.go]
tech_stack:
  added: []
  patterns: [test-seam-injection, idempotent-bootstrap]
key_files:
  created:
    - internal/venv/venv.go
    - internal/venv/venv_test.go
  modified:
    - internal/cli/sync.go
    - internal/cli/auth.go
decisions:
  - "venvDirFunc/lookPathFunc package vars for test seam injection (consistent with project cmdFactory pattern)"
  - "Fallback to bare 'audible' on venv bootstrap failure (graceful degradation)"
metrics:
  duration: 2min
  completed: 2026-04-04
---

# Quick Task 260404-pw1: Auto-manage audible-cli Python dependency Summary

Managed Python venv auto-bootstrap for audible-cli via internal/venv package with idempotent fast-path detection and graceful fallback

## What Was Done

### Task 1: Create internal/venv package (TDD)

Created `internal/venv/venv.go` with three exported functions:

- **VenvDir()** -- returns `~/.config/earworm/venv/` path
- **AudibleCLIPath()** -- returns expected `venv/bin/audible` binary path
- **EnsureAudibleCLI(ctx, writer)** -- idempotent bootstrap: checks if binary exists (fast path), otherwise creates venv via `python3 -m venv`, pip-installs audible-cli, verifies installation

Test seams via package-level vars (`venvDirFunc`, `lookPathFunc`, `execCommand`) for unit testing without real Python.

Tests cover: path resolution, fast-path reuse, python3-not-found error.

**Commits:**
- `3260ee4` test(quick-260404-pw1): add failing tests for venv package
- `bc5c2aa` feat(quick-260404-pw1): implement venv package for managed audible-cli

### Task 2: Wire venv bootstrap into CLI client factory

Updated `newAudibleClient` in `internal/cli/sync.go`:
- When `audible_cli_path` is default (`"audible"`), calls `venv.EnsureAudibleCLI` to get managed binary path
- On bootstrap failure, logs warning via slog and falls back to bare `"audible"`
- Custom `audible_cli_path` values bypass venv entirely

Updated `internal/cli/auth.go` error message from "pip install audible-cli" to "Ensure python3 is available on PATH" since earworm now self-manages the dependency.

**Commit:** `6fb6ac1` feat(quick-260404-pw1): wire venv bootstrap into CLI client factory

## Deviations from Plan

None -- plan executed exactly as written.

## Verification Results

- `go build ./cmd/earworm/` -- compiles cleanly
- `go test ./internal/venv/ -v` -- 4/4 tests pass
- `go test ./internal/cli/ -v` -- all CLI tests pass
- `go vet ./...` -- no issues

## Known Stubs

None -- all functions are fully implemented.

## Self-Check: PASSED

All 4 key files exist. All 3 commits verified.
