# Domain Pitfalls

**Domain:** Plan-based library cleanup operations for audiobook manager (v1.1)
**Researched:** 2026-04-06

## Critical Pitfalls

Mistakes that cause data loss, rewrites, or major issues.

### Pitfall 1: Plan State Divergence from Filesystem Reality

**What goes wrong:** A plan is created based on a scan, but by the time the user reviews and applies it, the filesystem has changed. Files were moved, renamed, or deleted externally (by Audiobookshelf, another tool, manual intervention, or even a concurrent earworm daemon). The plan references paths that no longer exist, or worse, paths that now point to different content.

**Why it happens:** The gap between plan creation and plan execution can be minutes, hours, or days. The filesystem is mutable state that earworm does not own exclusively. The existing v1.0 daemon mode compounds this -- it could be running downloads and organizes while the user reviews a cleanup plan.

**Consequences:** File moves to wrong destinations, data loss from overwriting files that changed, cryptic errors from missing source paths, or silent corruption where the plan succeeds but the result is wrong.

**Prevention:**
- Re-validate every plan action at apply time: check that source paths exist and match expected SHA-256 hashes before executing any operation
- Store file hashes at plan creation time, verify at apply time
- Fail the entire plan (or at minimum the affected action) if any source file has changed
- Add a `plan_created_at` timestamp and warn (or refuse) if the plan is stale beyond a configurable threshold
- Never run cleanup plans while the daemon is active -- check for lockfile/PID

**Detection:** Plan apply produces "file not found" errors, hash mismatches in verification step, or organized books appear in wrong locations.

**Phase to address:** Plan infrastructure phase (the very first phase). This must be baked into the plan data model from day one, not bolted on.

### Pitfall 2: Partial Plan Execution Without Recovery

**What goes wrong:** A plan with 50 actions fails at action 23 (disk full, permission error, network drop on NAS). The first 22 actions executed successfully. The system is now in an inconsistent state: half-reorganized library that neither matches the old layout nor the planned layout.

**Why it happens:** File operations are not transactional. Unlike database operations, you cannot rollback a file move. The existing `MoveFile` in v1.0 already handles individual file copy-verify-delete, but there is no plan-level recovery.

**Consequences:** Library is in a broken state. Audiobookshelf re-scans and loses track of books. User must manually figure out what completed and what did not.

**Prevention:**
- Log every completed action to the DB as it succeeds (action-level status tracking, not just plan-level)
- Record `source_path`, `dest_path`, `sha256_before`, `sha256_after` for every file operation
- On failure, stop execution and report exactly what completed and what remains
- Support `earworm plan resume <id>` to pick up from the last successful action
- Support `earworm plan rollback <id>` that uses the audit log to reverse completed actions (move files back)
- Each action must be idempotent: if dest already exists with correct hash, skip rather than fail

**Detection:** Plan status stuck in "executing", error in logs, library scan shows unexpected structure.

**Phase to address:** Plan infrastructure phase. The execution log schema must support resume/rollback from the start.

### Pitfall 3: Delete Operations Without Sufficient Guards

**What goes wrong:** The cleanup command deletes files that the user did not intend to lose. This could be the only copy of a non-Audible audiobook, personal recordings, or files that are not part of the earworm-managed library.

**Why it happens:** Cleanup operates on a NAS directory that may contain files earworm did not create. The existing scanner only tracks ASIN-bearing directories -- non-ASIN content is invisible to the current data model. A blanket "delete empty dirs" or "delete unrecognized files" command could destroy user data.

**Consequences:** Permanent data loss. Unlike move/rename operations, deletes cannot be undone without backups. Trust in the tool is destroyed.

**Prevention:**
- Separate cleanup into its own command (`earworm cleanup`) that ONLY does deletions -- never combine deletes with moves/renames in the same operation
- Require explicit double-confirmation for any delete: `--confirm-delete` flag plus interactive Y/N prompt
- Always show a complete list of files to be deleted with sizes before any confirmation prompt
- Default to moving to a `.earworm-trash/` directory rather than actual deletion; add `--permanent` flag for real deletes
- Never delete files that are not tracked in the plan system -- if a file was not part of a plan action, it cannot be cleaned up
- Log every deletion with full path, size, and hash to the audit trail

**Detection:** User reports missing files. Audiobookshelf shows books as "missing."

**Phase to address:** Must be the LAST phase before the Claude Code skill. Cleanup is the most dangerous operation and should build on all the safety infrastructure from earlier phases.

### Pitfall 4: NAS/SMB Silent Corruption During Cross-Filesystem Moves

**What goes wrong:** Files moved across filesystem boundaries (local to NAS, or between NAS mounts) appear to succeed but are silently corrupted. The existing `copyVerifyDelete` in mover.go only checks file size, not content hash. Size can match while content is corrupt, especially over SMB.

**Why it happens:** SMB file copies can produce corrupt files where source and target byte sizes are identical but SHA-256 hashes differ. This is a documented issue with SMB implementations, particularly on macOS. Network interruptions during writes may not always produce errors. NAS writes may not be immediately visible due to caching (write-back cache, SMB oplocks).

**Consequences:** Audio files silently corrupted. User discovers corruption weeks later when trying to play a book, and the original is already deleted.

**Prevention:**
- Upgrade verification from size-check to SHA-256 hash comparison for ALL cross-filesystem moves in v1.1
- Hash the source file before copy, hash the destination after copy, compare
- Add a brief delay (or fsync) before reading back on network filesystems, similar to PhotoStructure's approach of retrying verification
- Keep the source file until hash verification passes -- never delete source before verified copy
- Consider making the existing mover.go `copyVerifyDelete` accept a verification strategy (size-only for backward compat, hash for plan operations)
- Store hashes in the plan/audit tables so verification can be re-run later

**Detection:** `earworm verify` command that re-hashes organized files against stored hashes.

**Phase to address:** Structural operations phase (flatten/split). Must be implemented before any file-moving plan actions ship.

## Moderate Pitfalls

### Pitfall 5: Plan Schema That Does Not Support All Action Types

**What goes wrong:** The plan database schema is designed around the first action types (e.g., "move" and "rename") but cannot represent later action types (flatten, split, write-metadata, delete) without schema changes. Adding columns or tables mid-milestone creates migration headaches and breaks existing plans.

**Why it happens:** Natural tendency to build incrementally without designing the full action type taxonomy upfront. The existing migration system handles schema evolution fine, but in-progress plans stored in the DB may not survive schema changes gracefully.

**Prevention:**
- Design the plan action schema to be action-type-agnostic from the start: use a `type` enum column plus a JSON `params` column for action-specific data
- Define all known action types in the initial schema: `move`, `rename`, `flatten`, `split`, `write_metadata`, `delete`, `verify`
- Each action type has a defined set of required params validated at plan creation, not at execution
- Include a `status` column per action: `pending`, `executing`, `completed`, `failed`, `skipped`, `rolled_back`
- Version the plan format so old plans can be migrated if the schema evolves

**Detection:** New feature requires schema changes that invalidate existing plans.

**Phase to address:** Plan infrastructure phase (first phase).

### Pitfall 6: Flatten/Split Heuristics That Misidentify Book Boundaries

**What goes wrong:** The flatten operation assumes all audio files in nested subdirectories belong to one book. The split operation assumes multi-book folders can be separated by some heuristic (file naming, metadata). Both assumptions fail on edge cases: box sets with disc folders, series collections, audiobooks with bonus content, or books where Part 1 and Part 2 are separate purchases.

**Why it happens:** Audiobook file organization is wildly inconsistent. Some folders have `Disc 1/`, `Disc 2/` subfolders for a single book. Others have `Book 1/`, `Book 2/` for different books. File naming may use track numbers, chapter names, or arbitrary names. There is no universal convention.

**Consequences:** Files from different books merged into one folder (flatten error), or files from one book split into separate entries (split error). Audiobookshelf then misidentifies the library structure.

**Prevention:**
- Flatten and split should ALWAYS be plan-based with human review -- never automatic
- Present the proposed changes clearly: "These 15 files in 3 subdirectories will be moved to a single directory"
- Use metadata (dhowden/tag) to validate: if files in nested dirs have different album/title tags, warn that this may be multiple books
- For split: require explicit user input on boundaries (via CSV import or interactive review), do not try to auto-detect
- Default to conservative: when in doubt, do not flatten/split -- flag for manual review instead

**Detection:** Audiobookshelf shows wrong chapter counts, books merged together, or single books split into fragments.

**Phase to address:** Structural operations phase.

### Pitfall 7: metadata.json Conflicts with Audiobookshelf's Own Metadata

**What goes wrong:** Writing `metadata.json` files triggers Audiobookshelf to re-read metadata, which may conflict with metadata Audiobookshelf already has (from its own scanning, manual edits, or matched providers). The earworm-written metadata may be less complete or outright wrong compared to what Audiobookshelf discovered on its own.

**Why it happens:** Audiobookshelf reads metadata files during library scans. If earworm writes a `metadata.json` with partial data (e.g., no narrator, no series info), Audiobookshelf may overwrite its richer metadata with earworm's sparser version.

**Consequences:** Audiobookshelf metadata regresses after library scan. User loses manually curated metadata.

**Prevention:**
- Research exactly how Audiobookshelf handles `metadata.json` precedence (does it overwrite or merge?) before implementing
- Only write fields earworm is confident about -- omit fields rather than write empty values
- Use the Audiobookshelf metadata format (OPF or its native JSON), not a custom one
- Make metadata writing opt-in, not default: `earworm plan apply --write-metadata`
- Consider writing `metadata.json` only for books not yet in Audiobookshelf (new/unscanned)
- Add a `--no-metadata` flag to skip metadata writes entirely

**Detection:** Audiobookshelf shows wrong/missing metadata after earworm plan apply + scan trigger.

**Phase to address:** Metadata phase. Requires investigation of Audiobookshelf's metadata.json handling before implementation.

### Pitfall 8: CSV Import Encoding and Parsing Edge Cases

**What goes wrong:** CSV files exported from Excel contain a UTF-8 BOM (byte order mark) that Go's `encoding/csv` does not handle. The BOM becomes part of the first header name (`\uFEFFtitle` instead of `title`), breaking column lookups. Other edge cases: Windows line endings, embedded commas in titles, semicolon delimiters (European locale), and non-UTF-8 encodings.

**Why it happens:** The CSV import bridges manual analysis (likely done in a spreadsheet) to the plan system. Users will create CSVs in Excel, Google Sheets, or text editors with varying encoding behaviors. Go's stdlib CSV parser is strict and does not handle BOM. This is a known Go issue (golang/go#33887): BOM + quoted fields causes parse errors.

**Consequences:** Import silently produces wrong data (BOM in first field), or fails entirely on malformed rows. Plan created from bad import data will execute wrong operations.

**Prevention:**
- Strip UTF-8 BOM from input before passing to csv.Reader (use bufio.Reader Peek to detect 3-byte BOM `\xEF\xBB\xBF`)
- Normalize line endings (CRLF to LF) before parsing
- Validate every row against expected schema: required columns present, paths exist, ASINs are valid format
- Report validation errors per-row with line numbers, do not silently skip bad rows
- Support both comma and semicolon delimiters (auto-detect or `--delimiter` flag)
- Provide a `--validate-only` flag that checks the CSV without creating a plan
- Include a sample CSV template in the help output or docs

**Detection:** Plan review shows garbled first column, missing books, or wrong paths.

**Phase to address:** CSV import phase.

### Pitfall 9: Audit Log Growing Unbounded

**What goes wrong:** Every file operation logs source path, dest path, two SHA-256 hashes, timestamps, and status to SQLite. A large library cleanup touching 500+ books with multiple files each generates thousands of audit rows. Over time, repeated plan executions bloat the database.

**Why it happens:** Audit logs are append-only by design. No cleanup policy is implemented because "we might need this data."

**Consequences:** SQLite database grows to hundreds of MB. CLI operations slow down. Backup/restore of config directory becomes unwieldy.

**Prevention:**
- Design audit log with a retention policy from the start: completed plans older than N days (configurable, default 90) can be archived or pruned
- Use a separate table for audit entries (not inline on the plan/action tables) so it can be truncated independently
- Add `earworm plan prune --older-than 90d` command
- Keep plan summaries (counts, timestamps) even when detail rows are pruned
- Consider storing hashes only for the most recent plan execution, not historical ones

**Detection:** `earworm.db` file size growing significantly after repeated plan executions.

**Phase to address:** Execution logging phase.

## Minor Pitfalls

### Pitfall 10: Plan Review UX That Overwhelms Users

**What goes wrong:** A plan touching 200 books dumps 200+ lines of move operations to the terminal. The user cannot meaningfully review the plan and just approves it blindly, defeating the purpose of the review step.

**Prevention:**
- Group actions by type and show summary counts first: "Plan: 45 moves, 12 flattens, 3 splits, 8 deletes"
- Show detail only on request: `earworm plan show <id> --detail` or `--type=delete`
- Highlight dangerous operations (deletes, splits) separately from safe ones (metadata writes)
- Support `earworm plan show <id> --json` for piping to other tools

**Phase to address:** Plan infrastructure phase (review command).

### Pitfall 11: Race Condition Between Concurrent Plan Applies

**What goes wrong:** User accidentally runs `earworm plan apply` twice (separate terminals, or script error). Both instances try to execute the same file operations, leading to "file not found" errors, double-moves, or corrupted state.

**Prevention:**
- Use SQLite advisory locking or a filesystem lockfile during plan execution
- Mark plan status as "executing" atomically before starting, reject concurrent apply attempts
- The existing v1.0 codebase does not appear to have global locking -- this needs to be added for v1.1

**Phase to address:** Plan infrastructure phase.

### Pitfall 12: Existing v1.0 Status Model Conflicts with Plan Operations

**What goes wrong:** The v1.0 Book model has a single `status` field with values like "scanned", "organized", "downloaded". Plan operations need to track per-book plan state (e.g., "pending_move", "moved", "pending_metadata") that conflicts with the existing status semantics. Overloading the status field creates confusion between "this book's library state" and "this book's plan state."

**Prevention:**
- Do NOT add plan states to the existing `ValidStatuses` enum on the Book model
- Plans and plan actions should be separate tables with their own status tracking
- A book's status in the `books` table reflects its library state, not its plan state
- Join books to plan_actions via ASIN (or path for non-ASIN items) when displaying combined state

**Phase to address:** Plan infrastructure phase (schema design).

### Pitfall 13: Non-ASIN Books Cannot Be Represented in Current Data Model

**What goes wrong:** The existing `books` table is keyed by ASIN. Non-Audible audiobooks (manually added, from other sources) have no ASIN. The deep scanner will discover these folders but has nowhere to store them in the current schema. Attempting to use synthetic ASINs creates confusion between real and fake identifiers.

**Prevention:**
- Add a new table (e.g., `library_folders` or `library_items`) keyed by path, not ASIN, to represent ALL discovered directories
- Keep the `books` table for ASIN-keyed Audible content
- Plan actions should reference `library_items` (path-based) not `books` (ASIN-based), since plans operate on filesystem paths
- The deep scanner populates `library_items`; the existing ASIN scanner continues to populate `books`

**Phase to address:** Deep scanning phase (must precede plan infrastructure since plans need something to reference).

### Pitfall 14: Claude Code Skill Operating Without Plan Safety Rails

**What goes wrong:** The Claude Code skill for conversational orchestration bypasses the plan-review-apply workflow and directly executes file operations, or generates plans that are auto-applied without human review.

**Prevention:**
- The skill should ONLY create plans, never apply them
- Require explicit human confirmation between plan creation and plan execution
- The skill should call `earworm plan create` and `earworm plan show`, never `earworm plan apply`
- Document this boundary clearly in the skill definition

**Phase to address:** Claude Code skill phase (last phase).

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Deep scanning | Scanning NAS with thousands of dirs is slow; users think it hung | Add progress indicator, scan in batches, log dir count |
| Deep scanning | Non-ASIN items have no storage model | New library_items table keyed by path (Pitfall 13) |
| Plan infrastructure | Schema does not accommodate all future action types | Design type+params model upfront (Pitfall 5) |
| Plan infrastructure | No locking between concurrent operations | Add execution lock from day one (Pitfall 11) |
| Plan infrastructure | Plan-book status confusion | Separate tables, do not overload Book.Status (Pitfall 12) |
| Plan infrastructure | Stale plans applied against changed filesystem | Hash-based validation at apply time (Pitfall 1) |
| Structural operations (flatten/split) | Misidentifying book boundaries | Always require human review, use metadata for validation (Pitfall 6) |
| Structural operations | Silent corruption on NAS moves | SHA-256 verification, not just size check (Pitfall 4) |
| CSV import | BOM and encoding issues from Excel | Strip BOM, validate every row (Pitfall 8) |
| Metadata writing | Audiobookshelf metadata regression | Research ABS metadata.json handling first (Pitfall 7) |
| Execution logging | Unbounded audit log growth | Design retention policy from start (Pitfall 9) |
| Cleanup command | Accidental data loss | Separate command, double-confirm, trash dir default (Pitfall 3) |
| Claude Code skill | Bypassing safety workflow | Skill creates plans only, never applies (Pitfall 14) |

## Integration Pitfalls with Existing v1.0

| Concern | Risk | Mitigation |
|---------|------|------------|
| Daemon mode + plan execution | Daemon could move files while plan executes | Mutex/lockfile; plan apply refuses if daemon is active |
| Existing MoveFile only checks size | Insufficient for v1.1 safety needs | Extend with hash verification option, keep backward compat for v1.0 paths |
| Scanner ignores non-ASIN dirs | Deep scan must discover non-ASIN content for cleanup | New scan mode that catalogs ALL directories, not just ASIN-bearing ones |
| Book table is ASIN-keyed | Non-ASIN books cannot be represented | New table for library_items keyed by path, separate from books table |
| Migration system is sequential | Adding 4-5 new migrations in one milestone is fine but test upgrade paths | Ensure each migration is self-contained, test with existing v1.0 databases |
| UpsertBook overwrites all fields | Plan operations updating one field could clobber others | Plan operations should use targeted UPDATE queries, not full upserts |

## Sources

- [PhotoStructure file copy strategies](https://photostructure.com/guide/file-copy-strategies/) -- Verification patterns, NAS retry logic, atomic operations
- [SMB file corruption on macOS](https://github.com/doublecmd/doublecmd/issues/2018) -- SHA-256 mismatch despite matching file sizes over SMB
- [Go CSV BOM issue #33887](https://github.com/golang/go/issues/33887) -- BOM breaks Go's encoding/csv quote handling
- [Go-nuts: Dealing with BOM in encoding/csv](https://groups.google.com/g/golang-nuts/c/OSyFoMfXz7Q) -- Community solutions for BOM stripping
- [Audiobookshelf library structure discussion](https://github.com/advplyr/audiobookshelf/issues/2208) -- How ABS handles folder structures
- [Martin Fowler: Audit Log pattern](https://martinfowler.com/eaaDev/AuditLog.html) -- Canonical audit log design
- [DCG: Destructive Command Guard](https://reading.torqsoftware.com/notes/software/ai-ml/safety/2026-01-26-dcg-destructive-command-guard-safety-philosophy-design-principles/) -- Safety patterns for destructive CLI operations
- [Synology forum: copy with checksum verification](https://forums.spacerex.co/t/best-way-to-copy-files-with-checksum-verification/997) -- Real-world NAS checksum verification discussion
