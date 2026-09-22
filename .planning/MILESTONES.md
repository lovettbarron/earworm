# Milestones

## v1.1 Library Cleanup (Shipped: 2026-04-14)

**Phases completed:** 11 phases, 24 plans, 34 tasks
**Timeline:** 2026-04-07 → 2026-04-12 (6 days, 172 commits)

**Key accomplishments:**

- Plan infrastructure with DB-backed plans, operations, audit trail, and library_items tracking for non-ASIN content
- Deep library scanner detecting 8 issue types (no_asin, nested_audio, multi_book, missing_metadata, wrong_structure, orphan_files, empty_dir, cover_missing) with severity and suggested actions
- File operation primitives: flatten nested audio dirs, SHA-256 verified moves/copies, Audiobookshelf metadata.json sidecars
- Plan engine with review→approve→apply lifecycle, dry-run default, resume-on-failure, and full audit trail
- CSV import bridge with flexible column aliases, multi-book folder splitting with content detection, and guarded cleanup with trash-dir safety
- Data safety hardening: fsync in all copy paths, SHA-256 cross-filesystem verification, idempotent resume, pre-flight checks
- Scan-to-plan bridge connecting deep scan issues to actionable plans, with JSON output for machine consumption
- Claude Code skill for conversational library management with mandatory human approval gate

### Known Gaps

- Cleanup command unreachable in normal plan apply flow (plan apply executes deletes directly)
- SKILL.md missing `scan issues`, `--create-plan`, `plan approve` commands
- Human verification pending: NAS fsync on NFS/SMB, split on real multi-book folder, scan→plan on real library

---

## v1.0 MVP (Shipped: 2026-04-06)

**Phases completed:** 8 phases, 22 plans, 31 tasks

**Key accomplishments:**

- Pure Go SQLite database layer with embedded migrations, Book CRUD operations, and 13 passing unit tests using modernc.org/sqlite
- CLI framework with Cobra commands and Viper-managed YAML configuration, plus 20 passing tests
- User-facing README, cross-platform build config, and corrected project documentation
- Scan and status CLI commands with goroutine-based spinner progress, --json output, author/status filters, and integration tests
- audible-cli Download method with --aaxc/--cover/--chapter flags and SQLite download tracking via migration 004
- TDD-built rate limiter, exponential backoff, progress tracker, and M4A staging module for the download pipeline
- Download pipeline orchestrator composing rate limiter, backoff, progress tracker, and staging into a sequential batch download loop with retry, auth abort, and DB state tracking
- Fully wired earworm download command with two-stage signal handling, pipeline execution, --limit/--asin flags, and config-driven settings
- OrganizeBook moves M4A/cover/chapters from staging to Author/Title [ASIN] library structure with earworm organize CLI command
- Audiobookshelf scan client with Bearer auth and Goodreads CSV exporter with exact column format, date conversion, and shelf assignment — 11 tests passing
- Three new CLI commands (notify, goodreads, daemon) wired to ABS/Goodreads packages, with daemon polling loop and ABS auto-scan on download completion
- Comprehensive v1 README with 369 lines covering all 12+ commands, quickstart guide, full config reference, Audiobookshelf integration, daemon/systemd setup, and Goodreads export
- Removed MoveToLibrary from download pipeline so files remain in staging after download, making organize the sole staging-to-library path
- Raised 6 below-threshold packages to 80%+ line coverage using subprocess mocks, minimal MP4 builders, and error path tests
- Raised internal/cli package test coverage from 58.8% to 81.0% with tests for skip, daemon, notify, spinner, status, goodreads, and download commands
- ROADMAP.md checkboxes and progress table corrected for Phases 1-7; overall coverage verified at 83.2% with all 43 v1 requirements complete

---
