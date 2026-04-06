# Project Research Summary

**Project:** Earworm v1.1 — Library Cleanup
**Domain:** Plan-based audiobook library organization and structural repair
**Researched:** 2026-04-06
**Confidence:** HIGH

## Executive Summary

Earworm v1.1 extends a functional v1.0 audiobook download manager into a library cleanup system for a NAS-mounted audiobook collection. The central pattern, drawn from Terraform's plan/apply workflow and beets' interactive import, is: scan all folders to detect structural issues, generate a named plan of operations, require human review before execution, then apply operations with per-action audit logging. This pattern is well-established in infrastructure and media management tooling, and maps cleanly to the existing Cobra/SQLite/Go architecture.

The recommended approach is fully achievable within the existing Go dependency set — no new external libraries are required. Four new internal packages (plan, fileops, csvimport, audit) extend the codebase, and four new SQLite migrations add the plans, plan_operations, audit_log, and scan_issues tables. The existing organize.MoveFile and scanner packages are reused rather than replaced. Deep scanning adds a new code path alongside the existing ASIN-only scanner, keeping v1.0 behavior intact.

The dominant risk is data loss on NAS-mounted filesystems. Three mitigations must be present from day one: (1) SHA-256 hash verification for all file moves (not just size checks), (2) per-operation status tracking so partial plan failures leave a clear recovery path, and (3) deletion operations strictly separated into a guarded earworm cleanup command with explicit double-confirmation. The plan infrastructure phase, which establishes these safety primitives, is the prerequisite for all other v1.1 work.

## Key Findings

### Recommended Stack

No new external dependencies are required for v1.1. The existing stack (Go 1.26.1, Cobra v1.10.2, Viper v1.21.0, modernc.org/sqlite v1.48.1, dhowden/tag, testify) covers all feature needs. Stdlib packages crypto/sha256, encoding/csv, encoding/json, path/filepath, and database/sql provide the remaining primitives.

**Core technologies used by v1.1:**
- `database/sql` + modernc.org/sqlite: plan/operation/audit persistence — CGo-free, existing migration pattern
- `crypto/sha256` + `io.Copy`: streaming SHA-256 for cross-filesystem move verification — handles 100MB+ M4A files
- `encoding/csv`: CSV plan import with BOM stripping required for Excel compatibility
- `encoding/json`: metadata.json sidecar writing for Audiobookshelf compatibility
- `path/filepath.WalkDir`: efficient deep library scanning over NAS mounts (avoids extra Stat calls)

### Expected Features

**Must have (table stakes):**
- Deep library scan of all folders — users cannot fix what they cannot see; v1.0 only indexes ASIN folders
- Issue detection and categorization — no_asin, nested_audio, multi_book, missing_metadata, wrong_structure, orphan_files, empty_dir
- DB-persisted plan creation from scan results — plans survive CLI restarts, enable review
- Human-readable plan review (diff-format) — users must see exactly what will change before confirmation
- Plan apply with per-operation status tracking and SHA-256 verification
- Execution audit log — every file operation recorded with paths, hashes, timestamps
- metadata.json sidecar writing — Audiobookshelf reads this with highest priority, safer than tag embedding

**Should have (differentiators):**
- CSV import for plan creation — bridge from manual spreadsheet analysis to automated execution
- Guarded cleanup command (earworm cleanup) — separated deletion path with double-confirm and trash-dir default
- Flatten nested audio — common structural issue, implementable with existing MoveFile

**Defer to v1.2+:**
- Split multi-book folders — requires metadata analysis for book boundary detection, high edge-case risk
- Claude Code skill — depends on all plan infrastructure being stable; add after CLI workflow is proven
- Plan diffing between runs / reversal plan generation from execution log

### Architecture Approach

v1.1 follows an additive extension pattern: new packages are added alongside existing ones, and no v1.0 code paths (scan, download, organize, daemon) are modified. The plan is the central abstraction — a DB-persisted state machine that transitions draft -> reviewed -> applying -> applied/failed, with individual operation status tracked at each step. File operations are performed by a new fileops.Executor that wraps the existing organize.MoveFile and adds SHA-256 verification and audit logging via struct injection.

**Major components:**
1. `internal/plan/` — Plan lifecycle state machine (create, review, apply, status)
2. `internal/fileops/` — Structural file operations (flatten, move, delete) with SHA-256 verification
3. `internal/audit/` — Insert-only audit logger, injected into fileops and plan engine
4. `internal/csvimport/` — CSV-to-plan-operations parser with BOM handling and row validation
5. `internal/db/` additions — plans, plan_operations, audit_log, scan_issues tables (migrations 005-008)
6. `internal/scanner/` addition — DeepScan() alongside existing ScanLibrary(), adds FolderIssue detection
7. `internal/metadata/` addition — WriteMetadataJSON() for Audiobookshelf sidecar files
8. `internal/cli/` additions — plan.go, cleanup.go, import.go command files

### Critical Pitfalls

1. **Plan state diverging from filesystem reality** — Hash source files at plan creation time and re-verify at apply time. Fail the plan if any source has changed. Warn on stale plans. Never apply while daemon is active.
2. **Partial plan execution without recovery** — Log every completed action to DB immediately on success. Record source, dest, sha256_before, sha256_after per operation. Support earworm plan resume to pick up from last successful action.
3. **NAS/SMB silent corruption during cross-filesystem moves** — Upgrade size-check to SHA-256 for ALL plan-based file moves. Hash before copy, hash after, compare. Keep source until verified copy confirmed.
4. **Delete operations without sufficient guards** — Strict separation: deletions only in earworm cleanup, never in plan apply. Require --confirm-delete plus interactive prompt. Default to trash dir not permanent deletion.
5. **Non-ASIN books have no representation in current data model** — Add scan_issues table keyed by path, not ASIN. Plans reference filesystem paths, not the ASIN-keyed books table.

## Implications for Roadmap

Based on the build-order dependency graph in ARCHITECTURE.md and the phase-specific warnings in PITFALLS.md:

### Phase 1: Plan Infrastructure and DB Schema
**Rationale:** Everything else depends on this. Plans, operations, and audit tables must exist before any scanner enhancements or file operations can be wired up. Schema design decisions (type+params JSON blob, separate tables from books) must be correct from the start.
**Delivers:** DB migrations 005-008, internal/db/plans.go, internal/db/operations.go, internal/db/audit.go, internal/audit/logger.go
**Addresses:** Table stakes — plan creation, review, audit trail
**Avoids:** Pitfall 1 (stale plan validation baked in), Pitfall 2 (per-operation status from day one), Pitfall 5 (extensible type+params schema), Pitfall 11 (execution locking), Pitfall 12 (no Book.Status overloading), Pitfall 13 (path-keyed tables)

### Phase 2: Deep Library Scanner
**Rationale:** The plan engine needs something to scan before it can create plans. Deep scan can be developed in parallel with Phase 1 since it only extends the existing scanner package.
**Delivers:** scanner.DeepScan(), scanner.FolderIssue types, scan_issues table population, earworm scan --deep CLI flag
**Addresses:** Table stakes — full library inventory, issue categorization
**Avoids:** Pitfall 13 (discovers non-ASIN books), NAS scanning performance (filepath.WalkDir avoids extra Stat calls)

### Phase 3: Structural File Operations
**Rationale:** Required before the plan engine can execute. The fileops.Executor wraps existing organize.MoveFile and adds SHA-256 verification — this is where NAS corruption protection is implemented.
**Delivers:** internal/fileops/ package (Flatten, Delete, VerifyChecksum), SHA-256 move verification
**Addresses:** Table stakes — plan apply engine prerequisite; should-have — flatten nested audio
**Avoids:** Pitfall 4 (SHA-256 not size-check for NAS moves), Pitfall 6 (flatten always plan-based, never automatic)

### Phase 4: Plan Engine and CLI Commands
**Rationale:** With DB schema, deep scan, and file operations in place, the plan engine wires them together. This phase delivers the primary user workflow: scan -> create plan -> show plan -> apply plan.
**Delivers:** internal/plan/Engine, earworm plan create/list/show/apply/status commands, dry-run default, --confirm required for mutation
**Addresses:** All table stakes (plan creation, review, apply, execution logging)
**Avoids:** Pitfall 1 (hash validation at apply time), Pitfall 2 (resume after partial failure), Pitfall 10 (grouped summary view before detail)

### Phase 5: Metadata Writing
**Rationale:** Parallelizable with Phases 3-4 since WriteMetadataJSON has no dependency on the plan engine. Ships as a plan operation type in Phase 4.
**Delivers:** metadata.WriteMetadataJSON(), metadata plan action type, Audiobookshelf-compatible sidecar files
**Addresses:** Table stakes — metadata.json for missing metadata issue type; differentiator — non-destructive metadata fix
**Avoids:** Pitfall 7 (omit fields rather than write empty values; metadata writing opt-in)

### Phase 6: CSV Import
**Rationale:** Requires plan types from Phase 4 to construct plan operations from CSV rows. Low risk, well-scoped.
**Delivers:** internal/csvimport/ParseCSV(), earworm plan import FILE.csv command
**Addresses:** Differentiator — bridge from manual spreadsheet analysis to plan system
**Avoids:** Pitfall 8 (BOM stripping, CRLF normalization, row-level validation with line numbers, --validate-only flag)

### Phase 7: Guarded Cleanup Command
**Rationale:** Deletion is the most dangerous operation and must build on all safety infrastructure from earlier phases. Ships last among core features.
**Delivers:** earworm cleanup PLAN_ID --confirm with trash-dir default, double-confirmation UX, audit logging of every deletion
**Addresses:** Should-have — guarded deletion of empty dirs and orphan files
**Avoids:** Pitfall 3 (delete in separate command only, trash dir default, no undeclared-path deletions)

### Phase 8: Claude Code Skill
**Rationale:** The conversational orchestration layer is only safe and useful once the CLI workflow is stable. A skill that creates plans but never applies them is safe; build it after the underlying commands are solid.
**Delivers:** .claude/commands/library-cleanup.md skill file with scan -> plan -> review orchestration guidance
**Addresses:** Differentiator — AI-assisted library cleanup orchestration
**Avoids:** Pitfall 14 (skill creates plans only, human must apply)

### Phase Ordering Rationale

- Phases 1 and 2 can be developed in parallel (no cross-dependencies; Phase 1 is DB, Phase 2 is scanner)
- Phase 5 (metadata writing) is also parallelizable once Phase 1 migrations are in place
- Phase 3 must precede Phase 4 (plan engine needs fileops to execute operations)
- Phases 6 and 7 require Phase 4 (plan types and engine must exist)
- Phase 8 requires Phase 7 (all CLI commands must be stable)
- This ordering ensures that safety infrastructure (audit log, SHA-256 verification, execution lock) is in place before the features that depend on it are built

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 5 (metadata writing):** Audiobookshelf's metadata.json merge-vs-overwrite behavior is not fully documented. Requires testing against a live ABS instance before implementation. Pitfall 7 is unresolved.
- **Phase 4 (plan engine, resume):** The earworm plan resume UX and rollback-from-audit-log behavior needs design work during planning — no established pattern in the existing codebase.

Phases with standard patterns (skip research-phase):
- **Phase 1 (DB schema):** Follow established migration pattern exactly. All schema decisions are documented in ARCHITECTURE.md.
- **Phase 2 (deep scanner):** Extends existing ScanLibrary pattern. filepath.WalkDir is standard Go.
- **Phase 3 (file operations):** crypto/sha256 streaming pattern is fully specified in STACK.md. Reuses existing MoveFile.
- **Phase 6 (CSV import):** Go's encoding/csv plus BOM stripping is a known pattern. Pitfall 8 has full prevention strategy.
- **Phase 8 (Claude Code skill):** Markdown file, no code changes to earworm.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Zero new dependencies confirmed. All stdlib packages are standard Go. Existing stack versions verified from go.mod. |
| Features | HIGH | Table stakes derived from Terraform/beets/Audiobookshelf patterns, all well-documented. |
| Architecture | HIGH | Based on direct codebase analysis of v1.0 plus established conventions from CLAUDE.md. Build order derived from actual package dependencies. |
| Pitfalls | HIGH | SMB corruption and CSV BOM sourced from documented bugs. Plan state divergence and partial execution are fundamental patterns. |

**Overall confidence:** HIGH

### Gaps to Address

- **Audiobookshelf metadata.json precedence behavior:** Does ABS merge or overwrite when metadata.json exists alongside embedded audio tags and ABS's own metadata database? Must be verified against a live instance before Phase 5 implementation. Handle by making metadata writing opt-in as a safe default.
- **earworm plan resume UX design:** The recovery workflow after partial plan failure is specified at data model level (per-operation status) but the user-facing command design needs to be worked out during Phase 4 planning.
- **Multi-book folder split heuristics:** Deferred to v1.2+, but the deep scanner's multi_book issue detection still needs a heuristic that minimizes false positives. Audio metadata comparison (album/title tags) is recommended, but edge cases (box sets, disc folders) require conservative defaults.

## Sources

### Primary (HIGH confidence)
- Earworm v1.0 codebase (direct analysis) — existing patterns, package structure, migration system
- [Go crypto/sha256](https://pkg.go.dev/crypto/sha256) — streaming hash verification pattern
- [Go encoding/csv](https://pkg.go.dev/encoding/csv) — RFC 4180 CSV parser
- [Go filepath.WalkDir](https://pkg.go.dev/path/filepath#WalkDir) — efficient directory traversal
- [Audiobookshelf book scanner guide](https://www.audiobookshelf.org/guides/book-scanner/) — metadata.json format and priority
- [Terraform plan/apply](https://developer.hashicorp.com/terraform/cli/commands/plan) — plan/apply workflow pattern
- [beets CLI reference](https://beets.readthedocs.io/en/stable/reference/cli.html) — import workflow, dry-run, separation of delete

### Secondary (MEDIUM confidence)
- [SMB file corruption (doublecmd issue #2018)](https://github.com/doublecmd/doublecmd/issues/2018) — SHA-256 mismatch despite matching file sizes over SMB
- [PhotoStructure file copy strategies](https://photostructure.com/guide/file-copy-strategies/) — NAS verification patterns
- [Go CSV BOM issue #33887](https://github.com/golang/go/issues/33887) — BOM breaks encoding/csv quote handling
- [Audiobookshelf duplicate issue #3705](https://github.com/advplyr/audiobookshelf/issues/3705) — duplicate detection complexity
- [Martin Fowler: Audit Log pattern](https://martinfowler.com/eaaDev/AuditLog.html) — canonical audit log design

### Tertiary (LOW confidence)
- Audiobookshelf metadata.json merge-vs-overwrite behavior — needs live instance verification (see Gaps)

---
*Research completed: 2026-04-06*
*Ready for roadmap: yes*
