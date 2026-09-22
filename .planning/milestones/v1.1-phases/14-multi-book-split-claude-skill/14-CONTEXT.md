# Phase 14: Multi-Book Split & Claude Skill - Context

**Gathered:** 2026-04-11
**Status:** Ready for planning

<domain>
## Phase Boundary

This phase delivers two capabilities:
1. **Multi-book folder splitting** — detect and separate multiple books co-located in a single directory into individual book directories
2. **Claude Code skill** — conversational plan creation via Claude Code, never plan execution

</domain>

<decisions>
## Implementation Decisions

### Split Detection Strategy
- **D-01:** Hybrid detection — use metadata (title/author/narrator via dhowden/tag + ffprobe) combined with filename pattern analysis to propose file groupings
- **D-02:** Always require user confirmation of the proposed grouping before creating a split plan. Present via existing `earworm plan show` dry-run view
- **D-03:** When detection confidence is low (sparse/ambiguous metadata), skip the folder and flag it as "needs manual review" in scan results. User can create a manual plan via CSV import

### Split Output Structure
- **D-04:** Split directories use Libation naming convention (Author/Title [ASIN]/ or Author/Title/ if no ASIN)
- **D-05:** Shared files (covers, metadata) are copied to ALL split directories so each book is self-contained
- **D-06:** Original parent directory is NOT auto-removed after split — left for the `earworm cleanup` command. Conservative approach

### Claude Code Skill Scope
- **D-07:** Skill can orchestrate: `scan --deep`, `plan create`, `plan show` (dry-run), `status`/`list`
- **D-08:** Skill format is SKILL.md in `.claude/skills/` — standard Claude Code skill, auto-discovered
- **D-09:** Explicit deny-list guardrails in SKILL.md: NEVER run `plan apply`, `cleanup`, `download`, `organize`

### Skill Interaction Model
- **D-10:** Both slash command and natural language triggers — slash command for direct actions, natural language for exploratory conversations
- **D-11:** When creating a plan, skill runs `plan show` (JSON mode), formats output conversationally, and asks user for approval before saving. User can request adjustments

### Claude's Discretion
- Internal implementation of the split detection algorithm (grouping heuristics, confidence thresholds)
- SKILL.md trigger pattern specifics
- JSON output parsing and conversational formatting in the skill

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing Split Infrastructure
- `internal/scanner/issues.go` — IssueMultiBook detection, multi-book issue flagging with metadata comparison
- `internal/db/plans.go` — ValidOpTypes includes "split", plan CRUD operations
- `internal/metadata/metadata.go` — ExtractMetadata with tag/ffprobe/folder fallback chain
- `internal/metadata/tag.go` — extractWithTag for M4A metadata reading
- `internal/metadata/ffprobe.go` — extractWithFFprobe fallback

### Plan System (split operations build on this)
- `internal/planengine/` — Plan execution engine, cleanup, CSV import patterns
- `internal/cli/plan.go` — Plan CLI commands including show/create/import
- `internal/organize/path.go` — BuildBookPath for Libation-compatible naming

### File Operations Patterns
- `internal/fileops/` — File move, SHA-256 verification, cross-filesystem support
- `internal/planengine/cleanup.go` — MoveToTrash pattern (reference for safe file operations)

### Requirements
- `.planning/REQUIREMENTS.md` — FOPS-04 (multi-book split), INTG-02 (Claude Code skill)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `scanner.IssueMultiBook` detection already identifies multi-book folders with metadata comparison
- `metadata.ExtractMetadata()` provides the title/author/narrator data needed for grouping
- `organize.BuildBookPath()` generates Libation-compatible directory names
- `fileops` package handles cross-filesystem moves with SHA-256 verification
- `db.ValidOpTypes` already includes "split" as a valid operation type
- `planengine.ImportCSV()` provides the fallback path for ambiguous folders

### Established Patterns
- Plan operations: create plan → dry-run preview → user confirms → apply with `--confirm`
- SHA-256 verification on all file moves (fileops pattern)
- Audit logging via `db.LogAudit()` for all mutations
- Cobra CLI with `RunE` pattern, `--json` flag for machine output
- Package-level test seam injection (function vars for subprocess mocking)

### Integration Points
- Scanner → Split: scan detects multi_book → split creates plans from those issues
- Split → Plan system: split operations stored as plan operations with type "split"
- Skill → CLI: skill invokes earworm CLI commands via subprocess (Bash tool)
- Skill → User: conversational flow with plan preview before creation

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 14-multi-book-split-claude-skill*
*Context gathered: 2026-04-11*
