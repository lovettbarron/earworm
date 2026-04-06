# Phase 1: Foundation & Configuration - Context

**Gathered:** 2026-04-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Set up the Go project skeleton with CLI framework (Cobra/Viper), SQLite database layer with migration system, configuration management, and initial documentation. Users can install Earworm, configure settings, and interact with `earworm version` and `earworm config` commands. No library scanning or Audible interaction — those are Phase 2+.

</domain>

<decisions>
## Implementation Decisions

### Project Layout
- **D-01:** Standard Go project layout — `cmd/earworm/` for main entry point, `internal/` for private packages (`cli/`, `config/`, `db/`, `scanner/`, `audible/` stubbed for later phases)
- **D-02:** Go module path: `github.com/lovettbarron/earworm`

### Configuration
- **D-03:** YAML config file format, managed by Viper
- **D-04:** Config file location: `~/.config/earworm/config.yaml` (XDG-compliant)
- **D-05:** SQLite database lives alongside config: `~/.config/earworm/earworm.db`

### CLI Commands
- **D-06:** Phase 1 ships two commands: `earworm version` (build info) and `earworm config` (show/set/init subcommands)
- **D-07:** Informative output by default — show useful context (what happened, key details). Add `--quiet` flag for silent mode.

### Database
- **D-08:** Embedded SQL migration files via Go `embed` package. A `schema_versions` table tracks applied migrations.
- **D-09:** Initial schema includes `books` table (ASIN primary key, title, author, status, local_path, timestamps) and `schema_versions` table — ready for Phase 2 scanning.

### Claude's Discretion
- Specific config keys and defaults (library_path, staging_path, audible_cli_path, rate limit settings)
- Error message wording and formatting style
- Test file organization within packages
- README structure and content depth

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

No external specs — requirements fully captured in decisions above and in:

### Project Planning
- `.planning/PROJECT.md` — Project vision, constraints, key decisions
- `.planning/REQUIREMENTS.md` — LIB-03, LIB-04, CLI-01, CLI-02, CLI-04, TEST-01, TEST-02 are Phase 1 requirements
- `.planning/ROADMAP.md` — Phase 1 success criteria and dependency chain
- `CLAUDE.md` — Technology stack decisions, recommended libraries with versions

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield project. Only LICENSE and CLAUDE.md exist.

### Established Patterns
- None yet. Phase 1 establishes the patterns all subsequent phases follow.

### Integration Points
- `cmd/earworm/main.go` will be the binary entry point
- `internal/cli/` will register Cobra commands — future phases add commands here
- `internal/db/` will expose a DB handle used by all data-access code in later phases
- `internal/config/` will provide config values to all packages via Viper

</code_context>

<specifics>
## Specific Ideas

- `earworm version` should show version and git commit hash (goreleaser injects these at build time)
- `earworm config show` displays current config; `earworm config init` creates default config file
- Books table schema: ASIN as TEXT PRIMARY KEY, title, author, status (default 'unknown'), local_path, created_at, updated_at

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-foundation-configuration*
*Context gathered: 2026-04-03*
