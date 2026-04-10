# Roadmap: Earworm

## Overview

Earworm builds from the data layer up through a strict dependency chain: foundation and configuration first, then local library scanning to validate the data model against real files, then Audible integration to discover remote books, then the fault-tolerant download pipeline (the core differentiator), then file organization into Libation-compatible structure, and finally external integrations and polish. Each phase delivers a usable, verifiable capability.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation & Configuration** - Go project skeleton, SQLite state, config system, CLI framework
- [x] **Phase 2: Local Library Scanning** - Scan existing audiobook directories and display library status
- [x] **Phase 3: Audible Integration** - Authenticate with Audible, sync remote library, detect new books
- [x] **Phase 4: Download Pipeline** - Fault-tolerant batch downloads with rate limiting and crash recovery
- [x] **Phase 5: File Organization** - Organize downloads into Libation-compatible folder structure
- [x] **Phase 6: Integrations & Polish** - Audiobookshelf, Goodreads, daemon mode, documentation
- [x] **Phase 9: Plan Infrastructure & DB Schema** - DB tables, plan CRUD, audit logger, and library_items tracking for non-ASIN content
- [ ] **Phase 10: Deep Library Scanner** - Deep scan all folders, detect structural issues, persist scan results
- [x] **Phase 11: Structural Operations & Metadata** - Flatten nested audio, write metadata.json sidecars, SHA-256 verification (completed 2026-04-07)
- [ ] **Phase 12: Plan Engine & CLI** - Wire scan results into reviewable, executable plans with per-operation tracking
- [ ] **Phase 13: CSV Import & Guarded Cleanup** - CSV-to-plan bridge and separated deletion command with safety guards
- [ ] **Phase 14: Multi-Book Split & Claude Skill** - Content-based folder splitting and conversational plan creation

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
- [x] 06-02-PLAN.md — CLI commands (notify, goodreads, daemon), daemon package, ABS hook in download pipeline
- [x] 06-03-PLAN.md — README rewrite with full v1 documentation

### Phase 7: Fix Download→Organize Pipeline Integration
**Goal**: Fix the broken download→organize pipeline so books are correctly organized into Libation-compatible structure after download
**Depends on**: Phase 4, Phase 5
**Requirements**: ORG-01, ORG-02
**Gap Closure**: Closes gaps from v1.0 milestone audit
**Success Criteria** (what must be TRUE):
  1. After `earworm download` completes, files remain in staging (not moved to library by download pipeline)
  2. `earworm organize` successfully moves files from staging to library in Author/Title [ASIN]/ structure with correct file naming
  3. Full pipeline flow (download → organize → notify) completes end-to-end without errors
  4. Daemon cycle (sync → download → organize → ABS scan) succeeds with books reaching 'OK' status
  5. Integration tests verify the download→organize handoff with real staging directory state
**Plans**: 2 plans
Plans:
- [x] 07-01-PLAN.md — Remove MoveToLibrary from download pipeline, update verifyAndMove to verifyStaged, remove ABS scan from download command
- [x] 07-02-PLAN.md — Download-to-organize handoff integration tests, ABS scan trigger in organize command

### Phase 8: Test Coverage & Documentation Cleanup
**Goal**: Establish >80% test coverage measurement and fix stale documentation artifacts for clean milestone close
**Depends on**: Phase 7
**Requirements**: TEST-12
**Gap Closure**: Closes gaps from v1.0 milestone audit
**Success Criteria** (what must be TRUE):
  1. `go test ./... -coverprofile` runs and reports line coverage per package
  2. Overall line coverage exceeds 80% (or gaps are identified and addressed)
  3. REQUIREMENTS.md traceability table accurately reflects completion status for all 43 requirements
  4. ROADMAP.md progress table and phase checkboxes reflect actual completion state
**Plans**: 3 plans
Plans:
- [x] 08-01-PLAN.md — Test coverage for metadata, venv, audible, config, db, download packages
- [x] 08-02-PLAN.md — Test coverage for cli package
- [x] 08-03-PLAN.md — Documentation cleanup and coverage verification gate

### Phase 9: Plan Infrastructure & DB Schema
**Goal**: Create the database schema for library items and plan infrastructure that deep scanner and plan engine will build on
**Depends on**: Phase 8
**Requirements**: SCAN-02, PLAN-01, INTG-01
**Success Criteria** (what must be TRUE):
  1. Migration 005 creates library_items, plans, plan_operations, and audit_log tables
  2. LibraryItem CRUD functions work with path-based primary key and NormalizePath deduplication
  3. All existing tests continue to pass
**Plans**: 2 plans
Plans:
- [x] 09-01-PLAN.md — Migration 005, LibraryItem CRUD, tests
- [x] 09-02-PLAN.md — Plan and PlanOperation CRUD, audit log functions

### Phase 10: Deep Library Scanner
**Goal**: Users can deep-scan their entire library to discover non-ASIN content and detect structural issues with severity, category, and suggested actions
**Depends on**: Phase 9
**Requirements**: SCAN-01, SCAN-03
**Success Criteria** (what must be TRUE):
  1. User can run `earworm scan --deep` to traverse all library directories, not just ASIN-bearing ones
  2. Non-ASIN directories are tracked in the library_items table
  3. Eight issue types are detected: no_asin, nested_audio, multi_book, missing_metadata, wrong_structure, orphan_files, empty_dir, cover_missing
  4. Detected issues are persisted in scan_issues table with severity, category, and suggested action
  5. Running `--deep` again clears old issues and inserts fresh results (no accumulation)
  6. Existing `earworm scan` (without --deep) works exactly as before
  7. Unit tests cover all 8 issue detectors, deep scan traversal, DB persistence, and CLI integration
**Plans**: 3 plans
Plans:
- [ ] 10-01-PLAN.md — Migration 006 (scan_issues table), ScanIssue CRUD functions and tests
- [ ] 10-02-PLAN.md — Issue detection heuristics (8 detectors as pure functions) with tests
- [ ] 10-03-PLAN.md — DeepScanLibrary orchestrator, CLI --deep flag wiring, integration tests

### Phase 11: Structural Operations & Metadata
**Goal**: The file operation primitives exist for plan execution — flatten nested directories, write metadata sidecars, and verify file integrity via SHA-256
**Depends on**: Phase 9
**Requirements**: FOPS-01, FOPS-02
**Success Criteria** (what must be TRUE):
  1. User can flatten a nested audio directory, moving all audio files up to the book folder level with SHA-256 verification
  2. User can write an Audiobookshelf-compatible metadata.json sidecar for a book folder without modifying any audio files
  3. All file moves are verified via SHA-256 hash comparison (source before, destination after) before source deletion
  4. Failed operations leave source files intact (no data loss on verification failure)
**Plans**: 2 plans
Plans:
- [ ] 11-01-PLAN.md — SHA-256 hash utility, verified move, FlattenDir with collision handling
- [ ] 11-02-PLAN.md — Audiobookshelf metadata.json sidecar writer (ABSMetadata, BuildABSMetadata, WriteMetadataSidecar)

### Phase 12: Plan Engine & CLI
**Goal**: Users can go from scan results to reviewed, executed plans through CLI commands — the full scan-to-plan-to-apply workflow works end to end
**Depends on**: Phase 10, Phase 11
**Requirements**: PLAN-02, PLAN-03
**Success Criteria** (what must be TRUE):
  1. User can review a plan via CLI and see a human-readable diff showing source, destination, and operation type for every action
  2. User can apply a plan with `--confirm` and see per-operation progress with pass/fail status
  3. Plan application resumes from the last successful operation after interruption or failure
  4. Applied plans record SHA-256 hashes and per-operation status in the audit trail
  5. Plans default to dry-run (no mutation without explicit confirmation)
**Plans**: 2 plans
Plans:
- [ ] 12-01-PLAN.md — Plan executor engine with operation dispatch, resume-on-failure, SHA-256 audit trail
- [ ] 12-02-PLAN.md — CLI commands (earworm plan list/review/apply), dry-run default, --confirm flag

### Phase 13: CSV Import & Guarded Cleanup
**Goal**: Users can bridge manual spreadsheet analysis into the plan system and safely delete unwanted files through a separated, guarded command
**Depends on**: Phase 12
**Requirements**: PLAN-04, FOPS-03
**Success Criteria** (what must be TRUE):
  1. User can run `earworm plan import FILE.csv` and get a named plan created from CSV rows with validation feedback
  2. CSV import handles BOM, CRLF normalization, and reports row-level validation errors with line numbers
  3. User can run `earworm cleanup` with trash-dir default (not permanent deletion), double confirmation prompt, and full audit logging
  4. Cleanup command only processes delete operations from completed plans — it cannot delete arbitrary files
**Plans**: TBD

### Phase 14: Multi-Book Split & Claude Skill
**Goal**: Users can handle the hardest structural issue (multi-book folders) and optionally use Claude Code for conversational plan creation
**Depends on**: Phase 12
**Requirements**: FOPS-04, INTG-02
**Success Criteria** (what must be TRUE):
  1. User can split a multi-book folder into separate directories based on content detection (audio metadata comparison)
  2. Split operations use SHA-256 verification and produce audit trail entries like all other file operations
  3. Claude Code skill can orchestrate scan and plan creation through conversation, producing plans the user reviews before applying
  4. Claude Code skill never executes plans — only creates them for human review and application
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation & Configuration | 3/3 | Complete | Yes |
| 2. Local Library Scanning | 2/2 | Complete | Yes |
| 3. Audible Integration | 3/3 | Complete | Yes |
| 4. Download Pipeline | 4/4 | Complete | Yes |
| 5. File Organization | 2/2 | Complete | Yes |
| 6. Integrations & Polish | 3/3 | Complete | Yes |
| 7. Fix Download→Organize Pipeline | 2/2 | Complete | Yes |
| 8. Test Coverage & Doc Cleanup | 3/3 | Complete | Yes |
| 9. Plan Infrastructure & DB Schema | 2/2 | Complete | 2026-04-07 |
| 10. Deep Library Scanner | 0/3 | Planned | - |
| 11. Structural Operations & Metadata | 1/2 | Complete    | 2026-04-07 |
| 12. Plan Engine & CLI | 0/2 | Planned | - |
| 13. CSV Import & Guarded Cleanup | 0/0 | Not started | - |
| 14. Multi-Book Split & Claude Skill | 0/0 | Not started | - |
