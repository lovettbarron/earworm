# Roadmap: Earworm

## Overview

Earworm builds from the data layer up through a strict dependency chain: foundation and configuration first, then local library scanning to validate the data model against real files, then Audible integration to discover remote books, then the fault-tolerant download pipeline (the core differentiator), then file organization into Libation-compatible structure, and finally external integrations and polish. Each phase delivers a usable, verifiable capability.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Foundation & Configuration** - Go project skeleton, SQLite state, config system, CLI framework
- [ ] **Phase 2: Local Library Scanning** - Scan existing audiobook directories and display library status
- [ ] **Phase 3: Audible Integration** - Authenticate with Audible, sync remote library, detect new books
- [ ] **Phase 4: Download Pipeline** - Fault-tolerant batch downloads with rate limiting and crash recovery
- [ ] **Phase 5: File Organization** - Organize downloads into Libation-compatible folder structure
- [ ] **Phase 6: Integrations & Polish** - Audiobookshelf, Goodreads, daemon mode, documentation

## Phase Details

### Phase 1: Foundation & Configuration
**Goal**: Users can install Earworm, configure their library path and settings, and interact with a working CLI that persists state in SQLite
**Depends on**: Nothing (first phase)
**Requirements**: LIB-03, LIB-04, CLI-01, CLI-02, CLI-04, TEST-01, TEST-02
**Success Criteria** (what must be TRUE):
  1. User can install Earworm as a single Go binary and run `earworm --help` to see available commands
  2. User can set library root path and other settings via config file or CLI flags
  3. SQLite database is created automatically on first run in a local directory (never on NAS mount)
  4. README documents installation steps and audible-cli dependency setup
  5. Unit tests pass for database layer (schema, CRUD) and config system (parsing, defaults, validation) via `go test ./...`
**Plans**: 3 plans
Plans:
- [x] 01-01-PLAN.md — Go project init, SQLite database layer with embedded migrations and Book CRUD
- [x] 01-02-PLAN.md — Config system (Viper), CLI commands (Cobra: version, config show/set/init), tests
- [x] 01-03-PLAN.md — README documentation, GoReleaser config, CLAUDE.md version corrections

### Phase 2: Local Library Scanning
**Goal**: Users can index their existing audiobook library and see what they have
**Depends on**: Phase 1
**Requirements**: LIB-01, LIB-02, LIB-06, CLI-03, TEST-03, TEST-04
**Success Criteria** (what must be TRUE):
  1. User can run `earworm scan` on an existing Libation-style directory and see all discovered books indexed by ASIN
  2. User can run `earworm status` to see their library contents with book metadata and download status
  3. User can pass `--json` to get machine-readable output from status and list commands
  4. Error messages clearly tell the user what went wrong and suggest recovery steps
  5. Unit tests cover scanner logic (directory walking, ASIN extraction) and integration tests verify CLI commands (scan, status, --json) produce correct output
**Plans**: 2 plans
Plans:
- [x] 02-01-PLAN.md — DB schema extension, scanner package (ASIN extraction, directory walking), metadata extraction with fallback chain
- [x] 02-02-PLAN.md — CLI commands (earworm scan, earworm status), --json output, integration tests

### Phase 3: Audible Integration
**Goal**: Users can connect to their Audible account, see what books they own remotely, and identify what is new
**Depends on**: Phase 2
**Requirements**: AUD-01, AUD-02, AUD-03, AUD-04, LIB-05, TEST-05, TEST-06
**Success Criteria** (what must be TRUE):
  1. User can run `earworm auth` to authenticate with Audible via audible-cli
  2. User can run `earworm sync` to pull their full Audible library metadata into the local database
  3. User can see which Audible books are not yet downloaded locally (new book detection)
  4. User can preview what would be downloaded with a dry-run flag before committing to downloads
  5. Unit tests cover audible-cli wrapper (command building, output parsing, error mapping) using fake subprocess; integration tests verify sync and new-book detection flows
**Plans**: 3 plans
Plans:
- [x] 03-01-PLAN.md — DB migration 003 (Audible metadata columns), SyncRemoteBook upsert, ListNewBooks query
- [x] 03-02-PLAN.md — audible-cli subprocess wrapper package (interface, auth, library export, parsing, errors, tests)
- [x] 03-03-PLAN.md — CLI commands (earworm auth, sync, download --dry-run), integration tests

### Phase 4: Download Pipeline
**Goal**: Users can reliably download their Audible library with fault tolerance -- the core differentiator over Libation
**Depends on**: Phase 3
**Requirements**: DL-01, DL-02, DL-03, DL-04, DL-05, DL-06, DL-07, DL-08, DL-09, TEST-07, TEST-08
**Success Criteria** (what must be TRUE):
  1. User can run `earworm download` to batch-download new audiobooks in M4A format with cover art and chapter metadata
  2. Downloads are rate-limited with visible delays, and the user sees per-book and overall progress
  3. If the process is interrupted (Ctrl+C, crash, network failure), restarting picks up from the last incomplete book without re-downloading successful ones
  4. Failed downloads are tracked separately and can be retried with a single command
  5. Downloads land in a local staging directory before being moved to the library location
  6. Unit tests cover rate limiter, backoff calculator, retry state machine, and progress tracker; integration tests verify interrupt recovery and failure tracking end-to-end
**Plans**: 4 plans
Plans:
- [x] 04-01-PLAN.md — DB migration 004 (download tracking columns), download DB functions, audible-cli Download implementation
- [x] 04-02-PLAN.md — Download pipeline components: rate limiter, backoff calculator, progress tracker, staging module (TDD)
- [x] 04-03-PLAN.md — Pipeline orchestrator: batch download loop with retry, error categorization, DB state tracking
- [x] 04-04-PLAN.md — CLI wiring: earworm download command with signal handling, --limit, --asin flags

### Phase 5: File Organization
**Goal**: Downloaded books are automatically organized into Audiobookshelf-compatible folder structure on the library path (including NAS mounts)
**Depends on**: Phase 4
**Requirements**: ORG-01, ORG-02, ORG-03, TEST-09, TEST-10
**Success Criteria** (what must be TRUE):
  1. After download completes, books appear in Author/Title [ASIN]/ folder structure at the configured library path
  2. Each book folder contains the M4A audio file, cover art, and chapter metadata in their correct locations
  3. File moves from staging to library work correctly across filesystem boundaries (local to NAS mount)
  4. Unit tests cover path construction and naming logic; integration tests verify staging-to-library moves including cross-filesystem boundary handling
**Plans**: 2 plans
Plans:
- [x] 05-01-PLAN.md — Path construction (sanitization, first-author, BuildBookPath) and cross-filesystem file mover with size verification (TDD)
- [x] 05-02-PLAN.md — Organizer orchestrator, DB functions (ListOrganizable), earworm organize CLI command, integration tests

### Phase 6: Integrations & Polish
**Goal**: Users have a complete audiobook workflow with Audiobookshelf scan triggers, Goodreads sync, and unattended operation
**Depends on**: Phase 5
**Requirements**: INT-01, INT-02, INT-03, INT-04, CLI-05, TEST-11
**Success Criteria** (what must be TRUE):
  1. After downloads complete, Audiobookshelf automatically triggers a library scan (user configures API URL, token, and library ID)
  2. User can sync their Audible library to Goodreads via `earworm goodreads`
  3. User can run Earworm in daemon/polling mode that periodically checks for and downloads new books
  4. README reflects all current capabilities and commands
  5. Integration tests cover Audiobookshelf API calls (using HTTP mock), Goodreads sync trigger, and daemon mode start/stop lifecycle
**Plans**: 3 plans
Plans:
- [x] 06-01-PLAN.md — Audiobookshelf API client and Goodreads CSV export packages with tests
- [ ] 06-02-PLAN.md — CLI commands (notify, goodreads, daemon), daemon package, ABS hook in download pipeline
- [x] 06-03-PLAN.md — README rewrite with full v1 documentation

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation & Configuration | 0/3 | Planning complete | - |
| 2. Local Library Scanning | 0/2 | Planning complete | - |
| 3. Audible Integration | 1/3 | Executing | - |
| 4. Download Pipeline | 0/4 | Planning complete | - |
| 5. File Organization | 0/2 | Planning complete | - |
| 6. Integrations & Polish | 1/3 | In Progress|  |
