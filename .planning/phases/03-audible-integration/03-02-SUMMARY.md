---
phase: 03-audible-integration
plan: 02
subsystem: audible-cli-wrapper
tags: [subprocess, audible-cli, interface, testing, os-exec]
dependency_graph:
  requires: [internal/config (audible_cli_path default)]
  provides: [AudibleClient interface, LibraryItem struct, ParseLibraryExport, typed errors]
  affects: [internal/cli (Plan 03 will use AudibleClient), internal/db (Plan 03 will store LibraryItems)]
tech_stack:
  added: []
  patterns: [TestHelperProcess subprocess faking, interface-based client with cmdFactory injection, typed error classification]
key_files:
  created:
    - internal/audible/audible.go
    - internal/audible/auth.go
    - internal/audible/library.go
    - internal/audible/download.go
    - internal/audible/errors.go
    - internal/audible/parse.go
    - internal/audible/audible_test.go
  modified: []
decisions:
  - "Used pointer types (*int) for nullable JSON fields (runtime_length_min, num_ratings) per Research Pitfall 6"
  - "cmdFactory injection pattern for testing instead of interface-based exec abstraction -- simpler, TestHelperProcess compatible"
  - "buildArgs helper centralizes --profile-dir flag insertion across all commands"
  - "LibraryExport uses temp file strategy per Research Pitfall 1 (audible-cli stdout may contain non-JSON)"
metrics:
  duration: 4min
  completed: "2026-04-04"
---

# Phase 03 Plan 02: audible-cli Subprocess Wrapper Summary

Interface-based audible-cli wrapper with TestHelperProcess subprocess fakes, typed error classification, and JSON library export parsing with null-safe field handling.

## Tasks Completed

| # | Task | Commit | Key Files |
|---|------|--------|-----------|
| 1 | Create audible package with interface, errors, parsing, and wrappers | 2b50da0 | audible.go, errors.go, parse.go, auth.go, library.go, download.go |
| 2 | TestHelperProcess subprocess tests | 87c200f | audible_test.go |

## What Was Built

### AudibleClient Interface (audible.go)
- `Quickstart(ctx)` -- interactive auth via terminal passthrough
- `CheckAuth(ctx)` -- preflight token validation via `library list --bunch-size 1`
- `LibraryExport(ctx)` -- full library export to JSON via temp file
- `Download(ctx, asin, outputDir)` -- Phase 4 stub returning ErrNotImplemented
- `WithProfilePath` and `WithCmdFactory` client options
- `buildArgs` helper for consistent --profile-dir injection

### Typed Error System (errors.go)
- `AuthError` -- unauthorized, expired, auth keyword detection
- `RateLimitError` -- rate limit, too many requests detection
- `CommandError` -- generic subprocess failure with exit code and stderr
- `classifyError()` -- stderr-based error classification function

### Library Export Parsing (parse.go)
- `LibraryItem` struct with 16 JSON-tagged fields
- Pointer types for nullable fields (`RuntimeLengthMin *int`, `NumRatings *int`)
- `RuntimeMinutes()` -- null-safe accessor defaulting to 0
- `AudibleStatus()` -- derives finished/in_progress/new from fields
- `ParseLibraryExport(data)` -- JSON unmarshaling with wrapped error

### Subprocess Implementations
- `auth.go` -- Quickstart (stdin/stdout passthrough), CheckAuth (stderr capture)
- `library.go` -- temp file creation, audible-cli invocation, file read-back, JSON parse
- `download.go` -- ErrNotImplemented stub

### Test Suite (audible_test.go)
- TestHelperProcess pattern: routes on GO_HELPER_SCENARIO env var
- fakeCommand factory creates subprocess fakes without real audible-cli
- 14 test cases covering auth, library export, download, error classification, JSON parsing, and status derivation

## Verification

- `go build ./internal/audible/` -- exits 0
- `go test ./internal/audible/ -count=1` -- 14/14 tests pass
- `go test ./... -count=1` -- all packages pass, no regressions

## Deviations from Plan

None -- plan executed exactly as written.

## Known Stubs

| File | Line | Stub | Resolution |
|------|------|------|------------|
| internal/audible/download.go | 7 | `return ErrNotImplemented` | Intentional -- Phase 4 will implement actual download logic |

## Self-Check: PASSED

All 7 files verified on disk. Both commit hashes (2b50da0, 87c200f) found in git log.
