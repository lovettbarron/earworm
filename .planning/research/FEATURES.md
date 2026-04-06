# Feature Landscape: v1.1 Library Cleanup

**Domain:** Plan-based audiobook library cleanup and organization
**Researched:** 2026-04-06
**Confidence:** HIGH (domain patterns well-established via Terraform plan/apply, beets import workflow, media library tools)

## Context

v1.0 handles ASIN-bearing books: scan, download, organize, notify. v1.1 addresses everything v1.0 skips -- folders without ASINs, misstructured directories, missing metadata, multi-book folders, and nested audio. The core pattern is plan-then-apply with zero destructive defaults, modeled after Terraform's plan/apply workflow and beets' interactive import.

Existing v1.0 capabilities this builds on:
- Scanner identifies ASIN-bearing folders, skips non-ASIN folders (reports as `SkippedDir` with reason `no_asin`)
- SQLite DB tracks books by ASIN with status lifecycle
- Organize package moves files into `Author/Title [ASIN]/` structure
- MoveFile handles cross-filesystem copies

## Table Stakes

Features users expect in any plan-based library cleanup tool. Missing these means the system feels unsafe or incomplete.

| Feature | Why Expected | Complexity | Depends On |
|---------|--------------|------------|------------|
| Deep library scan (all folders) | Current scanner only indexes ASIN folders. Non-ASIN folders are invisible -- users cannot fix what they cannot see. Every library cleanup tool starts with full inventory. | MEDIUM | Existing scanner infrastructure; new `library_items` table or extended scan model |
| Issue detection during scan | Identifying problems is the whole point. Audiobookshelf users report: nested audio in wrong directory level, multi-book folders, missing metadata, orphan files. Must categorize issues, not just list folders. | MEDIUM | Deep scan |
| Plan creation from scan results | Terraform's `plan` step is table stakes for any tool that modifies user files. Users must see proposed changes before anything happens. DB-persisted plans survive CLI restarts. | HIGH | Deep scan, issue detection, new `plans` and `plan_operations` tables |
| Plan review (human-readable diff) | Terraform shows `+ create`, `~ update`, `- destroy`. Users must review exactly what will change, with clear operation types and affected paths. | MEDIUM | Plan creation |
| Plan apply with confirmation | `terraform apply` requires explicit "yes" to proceed. Earworm must do the same. No auto-apply without `--auto-approve` equivalent. | MEDIUM | Plan review |
| Dry-run / preview mode | beets has `-p` (pretend), Terraform has speculative plans. Must be able to preview without persisting a plan. | LOW | Plan creation |
| Undo / rollback information | Users need confidence they can recover. At minimum, log what was done and where originals were. Full undo is a differentiator; logging what happened is table stakes. | MEDIUM | Execution logging |
| Non-destructive defaults | No deletion without explicit opt-in. Moves and renames are recoverable; deletes are not. Separating deletions from structural operations is a safety pattern seen in beets (separate `remove` vs `import`) and media cleaners (dry-run-first). | LOW | Architecture decision, not code |

## Differentiators

Features that set Earworm's cleanup apart from BadaBoomBooks, manual scripting, or Audiobookshelf's limited built-in tools.

| Feature | Value Proposition | Complexity | Depends On |
|---------|-------------------|------------|------------|
| Plan-as-data (DB-persisted plans) | Unlike Terraform's file-based plans, Earworm plans live in SQLite. This enables: partial apply, plan diffing between runs, plan merging from multiple sources (scan + CSV import). No other audiobook tool has structured plan infrastructure. | HIGH | New DB tables: `plans`, `plan_operations` |
| CSV import for plan creation | Bridge between manual analysis and automated execution. User exports library list, annotates in spreadsheet (correct author, correct title, action), imports as plan. No audiobook tool supports this workflow. | MEDIUM | Plan infrastructure |
| metadata.json sidecar writing | Audiobookshelf reads metadata.json with highest priority (above folder name, audio tags, OPF). Writing this file lets Earworm fix metadata without touching audio files. Safer than tag embedding. ABS will pick it up on next scan. | MEDIUM | Deep scan (to know what needs metadata), Audiobookshelf metadata.json format |
| Structural operations with integrity verification | Flatten nested audio (move files up), split multi-book folders (separate into individual book folders), with SHA-256 before/after verification. No audiobook tool verifies file integrity after structural moves. | HIGH | Plan infrastructure, cross-filesystem move (existing) |
| Separated deletion workflow | Deletions are never part of a structural plan. Separate `earworm cleanup` command requires explicit confirmation per batch. Modeled after how beets separates `remove` from `import`, and how Terraform separates `destroy` from `apply`. | MEDIUM | Plan infrastructure, execution log (to know what is safe to delete) |
| Execution audit trail | Every operation logged to DB with timestamp, operation type, source path, destination path, SHA-256 hashes, success/failure, error message. Enables post-hoc review, debugging, and undo planning. | MEDIUM | New `execution_log` table |
| Claude Code skill for conversational orchestration | Natural language interface: "fix my library" becomes scan + plan + review cycle. Claude can interpret ambiguous folder names, suggest correct metadata, and orchestrate multi-step cleanup. No media tool has AI-assisted library management. | MEDIUM | All plan infrastructure, Claude Code custom skill format |
| Plan diffing between runs | Re-scan after partial apply, see what changed. Plans track generation number, operations track completion status. Re-scanning produces a new plan that accounts for already-applied operations. | MEDIUM | Plan infrastructure, deep scan |

## Anti-Features

Features to explicitly NOT build in v1.1.

| Anti-Feature | Why Tempting | Why Avoid | What to Do Instead |
|--------------|-------------|-----------|-------------------|
| Audio file tag writing / embedding | "Fix metadata everywhere" | Modifying audio files risks corruption, especially on NAS mounts with SMB/NFS. Audiobookshelf overwrites embedded tags anyway on rescan if metadata.json exists. | Write metadata.json sidecar only. Let Audiobookshelf handle tag embedding if user wants it. |
| Automatic plan execution (no review) | "Just fix everything" | Violates the safety-first principle. One wrong rename on a 500-book library is catastrophic. Even Terraform requires `--auto-approve` to skip review. | Always require explicit confirmation. Provide `--auto-approve` flag only for scripted/CI use with prominent warnings. |
| Format conversion during cleanup | "Convert M4A to M4B while reorganizing" | Scope explosion. Conversion is a separate concern with its own error modes (ffmpeg dependency, quality loss, chapter handling). Mixing it with structural operations makes rollback impossible. | Keep cleanup purely structural (move, rename, split, flatten) + metadata (metadata.json). Conversion is a separate tool/workflow. |
| Duplicate detection and merging | "Find and merge duplicate books" | Duplicate detection is a similarity problem (same book, different editions/narrators). False positives delete unique content. Audiobookshelf has an open issue for this (#3705) because it is genuinely hard. | Flag potential duplicates in scan output as an issue type. Let user decide via plan review. Never auto-merge. |
| Direct Audiobookshelf metadata sync | "Pull metadata from ABS, push metadata to ABS" | Two-way sync is a distributed systems problem. Conflict resolution, stale data, race conditions. ABS metadata.json is one-way and sufficient. | Write metadata.json locally. Trigger ABS scan. ABS reads the file. One-way, deterministic. |
| Recursive undo / full rollback | "Undo everything from the last apply" | True rollback requires storing original file state, which means copying every file before moving it. On a NAS with 500 audiobooks at 500MB each, that is 250GB of undo storage. | Log all operations in execution_log with source/destination paths. User can manually reverse specific operations. Provide `earworm plan undo` that generates a reversal plan from execution log. |
| Watch mode for continuous cleanup | "Monitor library and auto-fix new issues" | Continuous file watching on NAS mounts is unreliable (inotify doesn't work over SMB/NFS). Daemon mode already exists for downloads; mixing cleanup into it creates dangerous automation. | Manual scan + plan + apply cycle. User runs when ready. |

## Feature Dependencies

```
[Deep Library Scan]
    +--extends--> existing scanner (new: scan ALL folders, not just ASIN-bearing)
    +--produces--> library inventory with issue annotations
    |
    +--enables--> [Issue Detection]
    |                 +--categorizes--> nested_audio, multi_book, missing_metadata,
    |                                   no_asin, orphan_files, wrong_structure
    |
    +--enables--> [Plan Creation]
                      +--requires--> [DB Plan Tables] (plans, plan_operations)
                      +--input-from--> scan results OR CSV import
                      |
                      +--enables--> [Plan Review]
                      |                 +--renders--> human-readable operation diff
                      |                 +--enables--> [Plan Apply]
                      |                                   +--requires--> confirmation
                      |                                   +--performs--> rename, move, flatten, split
                      |                                   +--writes--> metadata.json
                      |                                   +--verifies--> SHA-256 integrity
                      |                                   +--logs-to--> [Execution Log]
                      |
                      +--enables--> [Guarded Cleanup]
                                        +--separate command--> earworm cleanup
                                        +--only deletions--> empty dirs, orphan files
                                        +--requires--> double confirmation

[CSV Import]
    +--creates--> plan operations from spreadsheet
    +--feeds-into--> [Plan Creation] (alternative to scan-based plan)

[Execution Log]
    +--enables--> audit trail review
    +--enables--> reversal plan generation
    +--enables--> Claude Code skill context

[Claude Code Skill]
    +--orchestrates--> scan, plan, review, apply
    +--requires--> all plan infrastructure
    +--reads--> execution log for context
```

## Issue Types (Deep Scan Output)

The deep scanner should categorize every non-conforming folder into actionable issue types:

| Issue Type | Description | Suggested Operation | Example |
|------------|-------------|--------------------|---------| 
| `no_asin` | Folder exists in library but has no ASIN in name. Cannot be matched to Audible. | User provides ASIN via CSV or interactive prompt; plan renames folder | `Author/Some Book Title/` |
| `nested_audio` | Audio files exist in subdirectories of a book folder instead of directly in it. Audiobookshelf may not detect them. | Flatten: move audio files up to book folder root | `Author/Title [ASIN]/CD1/track01.m4a` |
| `multi_book` | Single folder contains audio files from multiple distinct books (detected by metadata mismatch or file count anomaly). | Split: create separate folders per book | `Author/Collection/book1.m4a, book2.m4a` (different titles in tags) |
| `missing_metadata` | Book folder exists and is structurally correct but lacks metadata.json. Audiobookshelf will use lower-priority sources. | Generate metadata.json from DB or audio file tags | `Author/Title [ASIN]/` with no metadata.json |
| `wrong_structure` | Folder doesn't match `Author/Title [ASIN]/` convention. May be flat, too deeply nested, or differently named. | Rename/restructure to match convention | `Title - Author/` or `Author/Series/Title/` |
| `orphan_files` | Non-audio files (logs, temp files, .DS_Store, Thumbs.db) in book directories. | Cleanup candidate (deletion, handled by separate cleanup command) | `.DS_Store`, `desktop.ini`, `.nfo` files |
| `empty_dir` | Directory exists but contains no audio files. May be leftover from failed organize. | Cleanup candidate | `Author/Title [ASIN]/` with 0 audio files |
| `cover_missing` | Book folder has audio but no cover image. Audiobookshelf will show placeholder. | Not auto-fixable in v1.1, but flag for user awareness | -- |

## CLI Command Surface

Proposed commands for v1.1 (extends existing 12+ commands):

| Command | Purpose | Key Flags |
|---------|---------|-----------|
| `earworm scan --deep` | Full library scan including non-ASIN folders | `--deep` (new flag on existing command) |
| `earworm plan create` | Generate plan from latest deep scan results | `--from-csv FILE`, `--issues TYPES`, `--dry-run` |
| `earworm plan list` | List all plans with status | `--json` |
| `earworm plan show ID` | Display plan operations in diff format | `--json`, `--verbose` |
| `earworm plan apply ID` | Execute plan operations with confirmation | `--auto-approve`, `--operation-ids` (partial apply) |
| `earworm plan import FILE.csv` | Create plan from annotated CSV | `--dry-run` |
| `earworm cleanup` | Delete orphan files and empty dirs flagged by scan | `--dry-run`, `--yes` (still requires per-batch confirmation) |
| `earworm log` | View execution audit trail | `--plan ID`, `--since DATE`, `--json` |

## MVP Recommendation for v1.1

### Must Ship (Core Value)

1. **Deep library scan** -- without this, nothing else works. Extend existing scanner to report all folders with issue categorization.
2. **Plan infrastructure (DB tables + CRUD)** -- the foundation for everything. `plans` and `plan_operations` tables with status tracking.
3. **Plan create from scan** -- automated plan generation from detected issues. This is the primary user flow.
4. **Plan review (show)** -- human-readable diff output. Users must see what will change.
5. **Plan apply with confirmation** -- execute structural operations (rename, move, flatten) with SHA-256 verification.
6. **Execution logging** -- every operation recorded. Non-negotiable for a tool that modifies user files.
7. **metadata.json writing** -- the primary metadata fix mechanism. Audiobookshelf reads it with highest priority.

### Should Ship (High Value, Manageable Scope)

8. **CSV import for plan creation** -- bridges manual analysis workflow. Many users already have spreadsheets of their library.
9. **Guarded cleanup command** -- separated deletion path with double confirmation.
10. **Flatten nested audio** -- common structural issue, straightforward to implement with existing MoveFile.

### Defer to v1.2+ (Valuable but Risky Scope)

11. **Split multi-book folders** -- requires metadata analysis to determine book boundaries. High complexity, edge cases.
12. **Claude Code skill** -- depends on all plan infrastructure being stable. Add once the CLI workflow is proven.
13. **Plan diffing between runs** -- nice-to-have, requires plan versioning logic.
14. **Reversal plan generation from execution log** -- useful but adds complexity to the audit trail.

## Complexity Assessment

| Feature Area | Estimated Complexity | Risk | Notes |
|-------------|---------------------|------|-------|
| Deep scan extension | MEDIUM | LOW | Extends existing scanner. Main work is issue categorization logic. |
| Plan DB schema | MEDIUM | LOW | New tables, standard CRUD. Follows existing migration pattern. |
| Plan creation from scan | MEDIUM | MEDIUM | Mapping issues to operations requires decision logic per issue type. |
| Plan review rendering | LOW | LOW | String formatting of operations into diff-like output. |
| Plan apply engine | HIGH | MEDIUM | Must handle partial failures, SHA-256 verification, cross-filesystem moves. Existing MoveFile helps. |
| metadata.json writing | MEDIUM | LOW | Well-defined Audiobookshelf format. JSON serialization. |
| CSV import parsing | LOW | LOW | Standard CSV with defined columns. Validation is the work. |
| Execution logging | MEDIUM | LOW | Insert-only table. Straightforward. |
| Guarded cleanup | MEDIUM | MEDIUM | Deletion safety is the concern, not implementation complexity. |
| Structural operations (flatten) | MEDIUM | MEDIUM | File moves with integrity checks. Edge cases around name collisions. |
| Structural operations (split) | HIGH | HIGH | Requires metadata analysis to determine book boundaries in multi-book folders. |
| Claude Code skill | MEDIUM | LOW | Custom skill file format, wraps existing CLI commands. |

## Sources

- [Terraform plan command](https://developer.hashicorp.com/terraform/cli/commands/plan) -- plan/apply workflow pattern
- [Terraform apply command](https://developer.hashicorp.com/terraform/cli/commands/apply) -- confirmation and auto-approve patterns
- [beets CLI reference](https://beets.readthedocs.io/en/stable/reference/cli.html) -- import workflow, timid mode, pretend/dry-run
- [Audiobookshelf book scanner guide](https://www.audiobookshelf.org/guides/book-scanner/) -- metadata.json format, metadata priority system
- [Audiobookshelf duplicate issue #3705](https://github.com/advplyr/audiobookshelf/issues/3705) -- duplicate detection complexity
- [Audiobookshelf metadata overwrite issue #2155](https://github.com/advplyr/audiobookshelf/issues/2155) -- metadata priority behavior
- [Audiobookshelf flexible library structure #2208](https://github.com/advplyr/audiobookshelf/issues/2208) -- folder structure edge cases
- [BadaBoomBooks](https://github.com/WirlyWirly/BadaBoomBooks) -- audiobook organizer, rename workflow, metadata.opf writing
- [AudiobookOrganiser](https://github.com/jamesbrindle/AudiobookOrganiser) -- rename and organize patterns
- [beets pipeline](https://github.com/adammillerio/beets-pipeline) -- workflow configuration patterns

---
*Feature research for: v1.1 Library Cleanup (plan-based audiobook library organization)*
*Researched: 2026-04-06*
