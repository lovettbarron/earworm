# Phase 6: Integrations & Polish - Context

**Gathered:** 2026-04-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Complete the audiobook workflow with four capabilities: (1) Audiobookshelf library scan triggers after downloads, (2) Goodreads CSV export for library sync, (3) daemon/polling mode for unattended operation, and (4) README documentation covering all v1 commands. This phase delivers INT-01 through INT-04, CLI-05, and TEST-11.

</domain>

<decisions>
## Implementation Decisions

### Audiobookshelf Integration
- **D-01:** Trigger ABS library scan once after full batch completes (end of download/organize pipeline). No per-book scans.
- **D-02:** If ABS is unreachable, warn and continue. Downloads/organization are not blocked by ABS availability. Books are already in the library — user can trigger scan manually later.
- **D-03:** Silent skip if `audiobookshelf.url` is unconfigured. No nagging or hints. Users who don't use ABS never see it mentioned.
- **D-04:** Add standalone `earworm notify` command for manual ABS scan trigger. Useful when ABS was down during download, or user organized files outside Earworm.
- **D-05:** Use `net/http` stdlib for ABS API calls (`POST /api/libraries/{id}/scan` with Bearer token auth). No third-party HTTP client needed.

### Goodreads Sync
- **D-06:** CSV export approach — `earworm goodreads` generates a Goodreads-compatible CSV from the Audible library. User uploads it to Goodreads manually. No web scraping, no external tool wrapping.
- **D-07:** One-way sync: Audible → Goodreads only. Export library as CSV for Goodreads import.
- **D-08:** Exported books land on the "read" shelf by default. No shelf configuration needed for v1.

### Daemon/Polling Mode
- **D-09:** Dedicated `earworm daemon` subcommand. Runs in foreground, polls on interval, runs full sync→download→organize→notify cycle each poll. Ctrl+C to stop. Easy to wrap with systemd/launchd.
- **D-10:** Default polling interval: 6 hours. Configurable via `daemon.polling_interval` config key. Conservative default suitable for NAS setups.
- **D-11:** Quiet by default between polls — only log when something happens (new books found, downloads started, errors). `--verbose` flag for heartbeat/cycle logs.
- **D-12:** Full pipeline cycle always — each poll runs sync, download, organize, notify. No partial step configuration. Simple mental model.
- **D-13:** Reuse Phase 4's two-stage Ctrl+C pattern: first SIGINT finishes current book then stops daemon. Second SIGINT kills immediately.

### Documentation
- **D-14:** README covers: installation (Go binary + audible-cli setup), quickstart guide (auth→sync→download), full command reference with flags.
- **D-15:** Include step-by-step audible-cli setup instructions directly in README (install Python, pip install audible-cli, earworm auth). One place for users to follow.

### Claude's Discretion
- Goodreads CSV format details (column names, date formats) — match Goodreads import expectations
- ABS API error parsing and retry logic details within the "warn and continue" constraint
- Daemon tick/poll implementation (time.Ticker vs time.After)
- README structure and ordering of sections
- Whether `earworm notify` needs `--quiet` and `--json` flags (probably yes, for consistency)
- Whether daemon mode needs a `--once` flag for testing (run one cycle then exit)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Specs
- `.planning/PROJECT.md` — Core value, constraints (Go, M4A only, rate limiting), Audiobookshelf API notes
- `.planning/REQUIREMENTS.md` — INT-01, INT-02, INT-03, INT-04, CLI-05, TEST-11 are in scope
- `.planning/ROADMAP.md` §Phase 6 — Success criteria and dependency on Phase 5

### Prior Phase Context
- `.planning/phases/01-foundation-configuration/01-CONTEXT.md` — DB schema, config paths, CLI patterns
- `.planning/phases/03-audible-integration/03-CONTEXT.md` — audible-cli wrapper patterns, sync model, auth check
- `.planning/phases/04-download-pipeline/04-CONTEXT.md` — Signal handling (D-05), progress patterns (D-01), rate limiting, pipeline orchestration
- `.planning/phases/05-file-organization/05-CONTEXT.md` — Organize trigger (D-11, D-12), integration with download pipeline

### Existing Code
- `internal/config/config.go` — `audiobookshelf.url`, `audiobookshelf.token`, `audiobookshelf.library_id` already defined with empty defaults
- `internal/cli/download.go` — Two-stage Ctrl+C pattern, signal handling (reuse for daemon)
- `internal/cli/organize.go` — Organize command patterns
- `internal/db/books.go` — Book struct with author, title, ASIN — source data for Goodreads CSV export

### Technology
- `CLAUDE.md` §Technology Stack — net/http for Audiobookshelf API, os/exec for subprocess, Cobra for CLI
- `CLAUDE.md` §Audiobookshelf API Notes — `POST /api/libraries/{id}/scan`, `GET /api/libraries`, Bearer token auth
- `CLAUDE.md` §Conventions — Established patterns (project layout, DB driver, CLI, testing, error handling)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/config/config.go` — ABS config keys already defined and defaulted to empty strings
- `internal/cli/download.go` — Two-stage signal handling pattern (lines 86-93) — reuse for daemon mode
- `internal/db/books.go` — Book struct with all metadata needed for Goodreads CSV export
- `internal/cli/root.go` — Cobra root with `--quiet` flag pattern
- `internal/cli/spinner.go` — Spinner/progress patterns for CLI feedback

### Established Patterns
- Cobra commands in `internal/cli/`, one file per command, RunE for error propagation
- `cmd.OutOrStdout()` for testable output
- `--quiet` and `--json` flags on commands
- `net/http` stdlib for HTTP calls (per CLAUDE.md stack decision)
- testify/assert + testify/require for tests
- In-memory SQLite for DB tests

### Integration Points
- New `internal/audiobookshelf/` package for ABS API client (scan trigger, library list)
- New `internal/goodreads/` package for CSV export generation
- New `internal/daemon/` package for polling loop orchestration
- New CLI commands: `earworm notify`, `earworm goodreads`, `earworm daemon`
- Download pipeline calls ABS notify after batch completes (hook into existing pipeline)

</code_context>

<specifics>
## Specific Ideas

- ABS scan is just `POST /api/libraries/{id}/scan` with Bearer token — a ~20-line HTTP function
- Goodreads CSV format: ISBN, Title, Author, Shelf, Date Read columns. Map ASIN→ISBN if possible, otherwise leave blank.
- Daemon loop: `time.Ticker` at configured interval, select on ticker and signal channel. Each tick calls sync→download→organize→notify in sequence.
- `earworm notify` is useful as the standalone recovery path — similar to how `earworm organize` is the recovery path for Phase 5.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 06-integrations-polish*
*Context gathered: 2026-04-04*
