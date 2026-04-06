# Milestones

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
