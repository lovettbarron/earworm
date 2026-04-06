# Phase 2: Local Library Scanning - Context

**Gathered:** 2026-04-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Scan an existing Libation-style audiobook directory on a local or NAS-mounted filesystem, index discovered books by ASIN into the SQLite database, and display library status via CLI commands (`earworm scan`, `earworm status`). Machine-readable JSON output for all list/status commands. Clear error messages with recovery guidance.

</domain>

<decisions>
## Implementation Decisions

### ASIN Extraction
- **D-01:** Use fuzzy matching for ASIN extraction — recognize ASINs in square brackets `[B0...]`, parentheses `(B0...)`, and standalone B0-prefixed strings in folder names. Not limited to strict Libation `Author/Title [ASIN]/` pattern only.
- **D-02:** Folders with no recognizable ASIN are skipped with a warning. They are NOT indexed. User sees a summary of skipped folders at the end of the scan.
- **D-03:** Default scan depth is two levels (Author/Title) from the library root. A `--recursive` flag (or config option) enables full recursive tree walking to find ASIN-bearing folders at any depth. Default is two-level.

### Metadata Reading
- **D-04:** Extract comprehensive metadata from M4A files: title, author, narrator, duration, genre, year, series, cover art presence, chapter count — everything dhowden/tag can provide.
- **D-05:** Metadata fallback chain: dhowden/tag (primary) -> ffprobe if available (secondary) -> folder name parsing (tertiary). Each book's metadata source is tracked so the user knows the quality level.

### Status Display
- **D-06:** `earworm status` uses compact one-line-per-book format: `Author - Title [ASIN] (status_flag)`. Status flags indicate OK, partial metadata, missing files, etc.
- **D-07:** `--json` flag on status/list commands outputs machine-readable JSON (requirement LIB-06).
- **D-08:** Filtering support (e.g., --author, --status) at Claude's discretion based on what feels natural for the CLI design.

### Error Handling & Scan Behavior
- **D-09:** Permission errors and inaccessible directories are skipped with a warning. Scan continues on accessible paths. Summary of skipped paths shown at the end.
- **D-10:** Scan progress shown via spinner with live counter ("Scanning... 47 books found"). Provides feedback on slow NAS mounts.
- **D-11:** Rescanning is incremental — compare filesystem state against DB, add new books, update changed metadata, mark missing books as 'removed'. Preserves history and status tracking.

### Claude's Discretion
- Whether to include basic filtering flags (--author, --status) on `earworm status`, or keep it simple and let users pipe to grep.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Specs
- `.planning/PROJECT.md` — Core value, constraints (Go, M4A only, rate limiting, fault tolerance)
- `.planning/REQUIREMENTS.md` — LIB-01, LIB-02, LIB-06, CLI-03, TEST-03, TEST-04 are in scope
- `.planning/ROADMAP.md` §Phase 2 — Success criteria and dependency on Phase 1

### Technology
- `CLAUDE.md` §Technology Stack — dhowden/tag for M4A metadata, modernc.org/sqlite for DB, Cobra for CLI, lipgloss/bubbles for terminal output

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- No existing code yet — Phase 1 (Foundation & Configuration) must be completed first. Scanner will build on the SQLite schema and config system from Phase 1.

### Established Patterns
- None yet — this phase will establish patterns for file walking, metadata extraction, and CLI output formatting that later phases will follow.

### Integration Points
- SQLite database from Phase 1 (book table schema, CRUD operations)
- Config system from Phase 1 (library root path, scan depth settings)
- Cobra command structure from Phase 1 (`earworm scan`, `earworm status` subcommands)

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 02-local-library-scanning*
*Context gathered: 2026-04-03*
