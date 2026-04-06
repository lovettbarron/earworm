# Phase 4: Download Pipeline - Context

**Gathered:** 2026-04-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Fault-tolerant batch downloading of Audible audiobooks via audible-cli subprocess, with rate limiting, exponential backoff, crash recovery, progress reporting, and local staging. Downloads land in a staging directory, get verified, then move to the library location. This phase makes `earworm download` fully functional (Phase 3 stubbed the command with only --dry-run). No file organization into Libation folder structure -- that's Phase 5.

</domain>

<decisions>
## Implementation Decisions

### Progress Reporting
- **D-01:** Compact status line per book: `[3/12] Downloading: Author - Title [ASIN]... 45%` -- single updating line, keeps terminal clean. Consistent with scan spinner pattern from Phase 2.
- **D-02:** Include download speed and ETA estimates when audible-cli output provides enough data to calculate them.
- **D-03:** `--quiet` mode is fully silent until complete -- no progress output at all. Only the completion summary prints.
- **D-04:** Completion summary always prints, even in `--quiet` mode: "Downloaded 10/12 books (2 failed, 47m 23s elapsed)".

### Interrupt & Recovery
- **D-05:** Graceful two-stage Ctrl+C: first SIGINT finishes the current book then stops. Second SIGINT kills immediately, marking current book as incomplete. Prevents partial files on graceful shutdown.
- **D-06:** Auto-detect and report on restart: `earworm download` detects incomplete state and prints "Resuming: 8 of 12 remaining (4 completed previously)". No special --resume flag needed.
- **D-07:** Clean up orphaned staging files on startup: check staging dir for files with no matching 'downloaded' status in DB. Delete them and re-download from scratch. Simple, avoids corrupt files.

### Retry & Failure Tracking
- **D-08:** Auto-retry within batch: each book gets up to max_retries (default 3, configurable) attempts with exponential backoff (backoff_multiplier default 2.0) before being marked 'error'. Batch continues to next book after exhausting retries.
- **D-09:** Previously failed books are included automatically in the next `earworm download` alongside new books. Retry count resets on each new invocation. Just re-run the command.
- **D-10:** Categorize errors by parsing audible-cli output: network errors (retry with backoff), auth failures (abort entire batch + print "Run `earworm auth` to re-authenticate"), rate limits (longer backoff delay). Different error types get different handling.

### Staging Workflow
- **D-11:** Default staging directory: `~/.config/earworm/staging/`. Always local filesystem, fast writes, avoids NAS latency. `staging_path` config key (already defined in Phase 1) allows override.
- **D-12:** Move each book to library immediately after download + verification completes. Frees staging space and makes progress visible in library sooner.
- **D-13:** Basic verification before moving: check file exists, non-zero size, and M4A header is readable via dhowden/tag. Quick, catches corrupt downloads without heavy processing.

### Claude's Discretion
- audible-cli output parsing patterns for progress percentage, speed, and error categorization
- Rate limiter implementation details (token bucket vs simple sleep between requests)
- DB schema additions for tracking retry counts, error messages, and download timestamps
- How the download command selects which books to download (all undownloaded, or allow filtering by ASIN/author)
- Whether to support `--limit N` flag to cap batch size

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Specs
- `.planning/PROJECT.md` -- Core value (fault-tolerant downloads), constraints (Go, M4A only, rate limiting, fault tolerance)
- `.planning/REQUIREMENTS.md` -- DL-01 through DL-09, TEST-07, TEST-08 are in scope
- `.planning/ROADMAP.md` SS Phase 4 -- Success criteria and dependency on Phase 3

### Prior Phase Context
- `.planning/phases/01-foundation-configuration/01-CONTEXT.md` -- DB schema, config paths (staging_path, download.* keys), CLI patterns
- `.planning/phases/02-local-library-scanning/02-CONTEXT.md` -- Metadata extraction (dhowden/tag), spinner/progress patterns, --json flag, error handling
- `.planning/phases/03-audible-integration/03-CONTEXT.md` -- audible-cli wrapper strategy (D-02, D-03), download command stub, pre-flight auth check, sync model

### Technology
- `CLAUDE.md` SS Technology Stack -- os/exec for subprocess, charmbracelet/bubbles for progress, modernc.org/sqlite for DB
- `CLAUDE.md` SS Conventions -- Established patterns (project layout, DB driver, migrations, config, CLI, testing, error handling)

### Existing Code
- `internal/config/config.go` -- Already defines download.rate_limit_seconds (5), download.max_retries (3), download.backoff_multiplier (2.0), staging_path
- `internal/db/books.go` -- Book struct with status field; ValidStatuses includes "downloading", "downloaded", "error"

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/config/config.go` -- Download config keys already defined and validated (rate_limit_seconds > 0, max_retries >= 0)
- `internal/db/books.go` -- Book struct with CRUD, status transitions (downloading/downloaded/error already valid)
- `internal/db/db.go` -- Database Open/Close with migration runner. Phase 4 adds migration for download tracking columns.
- `internal/cli/root.go` -- Cobra root with --quiet flag. Phase 3 adds download command structure.
- `internal/cli/scan.go` -- Spinner pattern for progress feedback (charmbracelet/bubbles)

### Established Patterns
- Cobra commands in `internal/cli/`, one file per command, RunE for error propagation
- `cmd.OutOrStdout()` for testable output
- `--quiet` and `--json` flags on commands
- `os/exec` for subprocess management (established by Phase 3 audible-cli wrapper)
- testify/assert + testify/require for tests
- In-memory SQLite for DB tests
- Book status transitions validated in Go code, not DB constraints

### Integration Points
- Phase 3's `internal/audible/` package provides the download wrapper function (stubbed in Phase 3, implemented in Phase 4)
- `earworm download` command from Phase 3 (--dry-run functional, actual download added here)
- New `internal/download/` package for pipeline orchestration (rate limiter, retry state machine, progress tracker)
- Migration extends books table with download tracking fields (retry_count, last_error, download_started_at)

</code_context>

<specifics>
## Specific Ideas

- The two-stage Ctrl+C pattern: first signal sets a "stopping" flag checked between books; second signal is an immediate os.Exit with cleanup
- Rate limiting should use the configurable delay between requests (download.rate_limit_seconds), not a fixed value
- Error categorization: parse audible-cli stderr for keywords like "401", "unauthorized" (auth), "429", "rate limit", "too many" (rate limit), "timeout", "connection" (network)
- Completion summary format: "Downloaded X/Y books (Z failed, Nm Ns elapsed)" with per-failure detail lines below

</specifics>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 04-download-pipeline*
*Context gathered: 2026-04-04*
