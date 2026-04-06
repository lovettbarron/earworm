---
phase: 07-pipeline-integration-fix
plan: 02
subsystem: cli
tags: [integration-test, organize, audiobookshelf, pipeline]

# Dependency graph
requires:
  - phase: 07-pipeline-integration-fix
    provides: download pipeline decoupled from organize (Plan 01)
  - phase: 05-file-organization
    provides: OrganizeAll, OrganizeBook, file renaming logic
provides:
  - integration test proving download-to-organize handoff
  - ABS scan trigger in organize command
affects:
  - internal/cli/organize.go
  - internal/cli/pipeline_integration_test.go

# Tech stack
added: []
patterns:
  - external test package (cli_test) for integration tests avoiding circular imports
  - in-memory SQLite + temp dirs for pipeline handoff simulation

# Key files
created:
  - internal/cli/pipeline_integration_test.go
modified:
  - internal/cli/organize.go

# Decisions
key-decisions:
  - ABS scan in organize command mirrors daemon cycle pattern for standalone usage
  - Integration tests use cli_test package to access both db and organize packages

# Metrics
duration: 3min
completed: "2026-04-06"
tasks_completed: 2
tasks_total: 2
files_changed: 2
---

# Phase 07 Plan 02: Integration Test and ABS Scan Summary

Integration test verifying download-to-organize handoff with ABS scan trigger moved to organize command.

**One-liner:** Three integration tests prove M4B/M4A download-to-organize handoff with DB status transitions, plus ABS scan fires after organize instead of download.

## What Was Done

### Task 1: Download-to-Organize Handoff Integration Tests
Created `internal/cli/pipeline_integration_test.go` with three tests:

1. **TestDownloadOrganizeHandoff** -- Full M4B pipeline: simulates post-download state (files in staging/ASIN/, DB status='downloaded'), runs OrganizeAll, verifies Libation-compatible Author/Title [ASIN]/ structure with correct file renaming (Title.m4b, cover.jpg, chapters.json), DB status='organized', and staging cleanup.

2. **TestDownloadOrganizeHandoff_M4A** -- Same pipeline with M4A format to cover both audio file types.

3. **TestDownloadOrganizeHandoff_MissingStagingDir** -- Verifies graceful error handling when staging directory doesn't exist (pre-fix scenario where downloads moved files immediately). Confirms DB marked as 'error'.

**Commit:** 380ddbb

### Task 2: ABS Scan Trigger in Organize Command
Added Audiobookshelf library scan trigger to `internal/cli/organize.go` after successful organization:
- Only fires when successCount > 0 and ABS URL is configured
- Silent skip when ABS not configured (no error, no warning)
- Warning on scan failure without aborting the command
- Mirrors the existing daemon cycle pattern for consistency

**Commit:** bf33f7d

## Deviations from Plan

None -- plan executed exactly as written.

## Verification Results

- `go test ./internal/cli/ -run TestDownloadOrganizeHandoff -count=1 -v` -- all 3 tests PASS
- `go build ./...` -- compiles clean
- `go test ./... -count=1` -- full suite green (13 packages)
- `grep ScanLibrary internal/cli/organize.go` -- present (1 match)
- `grep ScanLibrary internal/cli/download.go` -- not present (0 matches, correct)

## Known Stubs

None -- all functionality is fully wired.

## Self-Check: PASSED
