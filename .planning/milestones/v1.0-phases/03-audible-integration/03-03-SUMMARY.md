---
phase: 03-audible-integration
plan: 03
subsystem: cli
tags: [cobra, audible-cli, sync, download, dry-run]

requires:
  - phase: 03-01
    provides: SyncRemoteBook, ListNewBooks, extended Book struct with Audible fields
  - phase: 03-02
    provides: AudibleClient interface, NewClient, WithProfilePath, typed errors
provides:
  - earworm auth command (wraps audible quickstart)
  - earworm sync command (pre-flight auth check, library export, upsert, summary)
  - earworm download --dry-run command (preview new books)
  - audible.profile_path config key
  - Integration tests with fake AudibleClient
affects: [phase-04-download-pipeline]

tech-stack:
  added: []
  patterns: [newAudibleClient injectable factory for CLI testing, per-command JSON flag variables]

key-files:
  created:
    - internal/cli/auth.go
    - internal/cli/sync.go
    - internal/cli/download.go
    - internal/cli/auth_test.go
    - internal/cli/sync_test.go
    - internal/cli/download_test.go
  modified:
    - internal/config/config.go
    - internal/cli/cli_test.go
---

## What was built

Three new CLI commands wiring the audible-cli wrapper (Plan 02) and extended DB layer (Plan 01) into user-facing functionality:

1. **`earworm auth`** — Wraps `audible quickstart` with interactive terminal pass-through. Uses `newAudibleClient` factory for testability.

2. **`earworm sync`** — Pre-flight auth check via `CheckAuth`, full library export via `LibraryExport`, upserts all books with `SyncRemoteBook` (preserving local fields), reports summary (total synced, new, already local). Supports `--json` output.

3. **`earworm download --dry-run`** — Lists books needing download in "Author - Title [ASIN] (runtime)" format. Supports `--json` for machine-readable output. Without `--dry-run`, returns "not yet implemented" (Phase 4). 

## Self-Check: PASSED

- [x] All tasks executed (2/2)
- [x] Each task committed individually
- [x] Tests pass: `go test ./internal/cli/ -count=1` exits 0
- [x] Full suite passes: `go test ./... -count=1` exits 0
- [x] Binary builds with new commands visible in `--help`

## Deviations

- Auth command refactored to use `newAudibleClient` factory (defined in sync.go) instead of inline `audible.NewClient` call, enabling test injection without TestHelperProcess overhead.
