# Phase 3: Audible Integration - Context

**Gathered:** 2026-04-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Connect to Audible via wrapped audible-cli subprocess, authenticate the user, sync their full Audible library metadata into the local SQLite database, detect which books are available remotely but not yet downloaded locally, and provide a dry-run preview of what would be downloaded. No actual downloading — that's Phase 4.

</domain>

<decisions>
## Implementation Decisions

### audible-cli Wrapping Strategy
- **D-01:** Earworm manages auth directly — `earworm auth` wraps `audible quickstart` to guide the user through Audible login interactively. Earworm stores/references the audible-cli profile path in its own config.
- **D-02:** Structured subprocess invocation via `os/exec`. Build commands programmatically, parse stdout/stderr line-by-line, map exit codes to typed Go errors. Each audible-cli command gets a dedicated Go wrapper function.
- **D-03:** Full wrapper inventory for Phase 3: wrap `quickstart` (auth), `library list` (sync), `library export` (richer metadata), and `download` (stubbed interface for Phase 4). Build the complete wrapper interface now even though download isn't used until Phase 4.

### Sync & Data Model
- **D-04:** Extend the existing `books` table with new columns for remote metadata: audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position. A book can exist as local-only, remote-only, or both (matched on ASIN).
- **D-05:** Remote wins on sync — remote metadata always overwrites local metadata fields. Audible is the source of truth for title, author, narrators, etc. Local-only fields (status, local_path) are preserved and never overwritten by sync.
- **D-06:** Always full sync — every `earworm sync` pulls the complete Audible library and upserts all books. Simple, reliable, handles deletions. audible-cli doesn't support incremental fetching anyway.

### New Book Detection & Dry-Run
- **D-07:** A book is "new" (needs download) if its ASIN exists in the Audible library but has no local_path or status is not 'downloaded'/'organized'. Simple set difference after sync.
- **D-08:** Dry-run is triggered via `earworm download --dry-run`. Shows each book that would be downloaded: Author - Title [ASIN] (runtime). Total count at the end. Respects `--json` flag for machine-readable output.

### Auth UX & Error Recovery
- **D-09:** `earworm auth` uses pass-through interactive mode — invokes `audible quickstart` with stdin/stdout connected to the terminal. User interacts directly with audible-cli's prompts. Earworm wraps the process but doesn't intercept the interactive flow.
- **D-10:** Auth failures during sync are detected by parsing audible-cli error output. Show clear guidance: "Authentication expired. Run `earworm auth` to re-authenticate." Abort sync cleanly.
- **D-11:** Pre-flight auth check before every sync — run a lightweight audible-cli command to verify auth is valid before starting the full sync. Fail fast with guidance rather than discovering auth issues mid-sync.

### Claude's Discretion
- Specific audible-cli output parsing patterns (varies by audible-cli version)
- Which lightweight command to use for pre-flight auth check
- Schema migration numbering and column types for new metadata fields
- Error message wording and formatting
- Test fixture design for fake subprocess testing

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Specs
- `.planning/PROJECT.md` — Core value, constraints (Go, wrap audible-cli, rate limiting)
- `.planning/REQUIREMENTS.md` — AUD-01, AUD-02, AUD-03, AUD-04, LIB-05, TEST-05, TEST-06 are in scope
- `.planning/ROADMAP.md` §Phase 3 — Success criteria and dependency on Phase 2

### Prior Phase Context
- `.planning/phases/01-foundation-configuration/01-CONTEXT.md` — DB schema, config paths, CLI command patterns
- `.planning/phases/02-local-library-scanning/02-CONTEXT.md` — Metadata extraction patterns, --json flag, error handling patterns

### Technology
- `CLAUDE.md` §Technology Stack — os/exec for subprocess, modernc.org/sqlite for DB, Cobra for CLI
- `CLAUDE.md` §Conventions — Established patterns from Phase 1 (project layout, DB driver, migrations, config, CLI, testing, error handling)

### External
- [audible-cli GitHub](https://github.com/mkb79/audible-cli) — Command reference, output formats, auth flow

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/db/db.go` — Database Open/Close with migration runner. Phase 3 adds migration 002 for new columns.
- `internal/db/books.go` — Book struct and CRUD. Phase 3 extends the struct and adds sync-specific functions (UpsertBook, ListNewBooks).
- `internal/cli/root.go` — Cobra root command. Phase 3 adds `auth` and `sync` subcommands.
- `internal/config/config.go` — Viper config with `audible_cli_path` already defined as a config key.

### Established Patterns
- Cobra commands in `internal/cli/`, one file per command
- `cmd.OutOrStdout()` for testable output
- `--quiet` and `--json` flag patterns
- `os/exec` is the prescribed subprocess tool (from CLAUDE.md stack)
- testify/assert + testify/require for tests
- In-memory SQLite for DB tests, viper.Reset() for config tests

### Integration Points
- New `internal/audible/` package for audible-cli wrapper functions
- New CLI commands: `earworm auth`, `earworm sync`
- Extended `earworm download --dry-run` (download command structure, actual downloading in Phase 4)
- Migration 002 extends books table schema

</code_context>

<specifics>
## Specific Ideas

- The download command wrapper should be built now (Phase 3) but only the `--dry-run` flag is functional. Actual download execution is Phase 4.
- Pre-flight auth check should be fast — a single lightweight audible-cli command to verify the token is still valid.
- Sync summary should show: X books synced, Y new (not downloaded), Z already local.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 03-audible-integration*
*Context gathered: 2026-04-03*
