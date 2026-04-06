# Technology Stack: v1.1 Library Cleanup

**Project:** Earworm v1.1
**Researched:** 2026-04-06
**Mode:** Incremental (additions to existing v1.0 stack)
**Overall Confidence:** HIGH

## Key Finding: Zero New External Dependencies

v1.1's features are fully covered by Go's standard library plus the existing dependency set. No new `go get` commands needed. This is the ideal outcome for a maintenance-focused milestone.

## Existing Stack (Unchanged)

| Technology | Version | Purpose | Status |
|------------|---------|---------|--------|
| Go | 1.26.1 | Application language | Unchanged |
| spf13/cobra | v1.10.2 | CLI commands | Add new subcommands only |
| spf13/viper | v1.21.0 | Configuration | May add new config keys |
| modernc.org/sqlite | v1.48.1 | Database | Add new migrations (005+) |
| dhowden/tag | v0.0.0-20240417 | M4A metadata reading | Unchanged |
| testify | v1.11.1 | Test assertions | Unchanged |

## Stdlib Capabilities for v1.1 Features

### Plan Infrastructure (DB-Backed Workflow)

**Need:** Plan creation, review, apply, cleanup lifecycle persisted in SQLite.

| Stdlib Package | Purpose | Why Sufficient |
|----------------|---------|----------------|
| `database/sql` | Plan/action CRUD | Already used for books table. Same patterns: INSERT, UPDATE, SELECT with status filtering. |
| `encoding/json` | Plan serialization for `--json` output | Already used in v1.0 status commands. `json.MarshalIndent` for human-readable plan display. |
| `time` | Timestamps for plan lifecycle | Plan created_at, applied_at, completed_at tracking. |

**Migration pattern:** Follow existing `internal/db/migrations/` numbered SQL files (005_add_plans.sql, 006_add_plan_actions.sql, etc.). Embedded via `//go:embed`. No schema migration library needed -- the existing manual migration runner works.

**Schema additions needed:**
- `plans` table: id, name, description, status (draft/reviewed/applying/applied/failed), created_at, applied_at
- `plan_actions` table: id, plan_id, action_type (move/flatten/split/delete/write_metadata), source_path, dest_path, status (pending/applied/failed/skipped), error_msg, applied_at
- `execution_log` table: id, plan_id, action_id, operation, detail, timestamp

### Structural File Operations (Flatten/Split/Move)

**Need:** Move files between directories, flatten nested structures, split multi-book folders, verify integrity with SHA-256.

| Stdlib Package | Purpose | Why Sufficient |
|----------------|---------|----------------|
| `os` | File/directory operations | `os.Rename`, `os.MkdirAll`, `os.Remove`, `os.ReadDir`. Already used in organize package. |
| `io` | Stream copying for cross-device moves | `io.Copy` for cross-filesystem moves. Already proven in v1.0's organize/mover.go. |
| `path/filepath` | Path manipulation | `filepath.WalkDir` for deep scanning. `filepath.Rel` for relative path computation. |
| `crypto/sha256` | File integrity verification | `sha256.New()` + `io.Copy()` pattern. Stream-based, handles large M4A files (100MB+) without loading into memory. |
| `crypto/subtle` | Hash comparison | `subtle.ConstantTimeCompare` for secure hash verification after file moves. |
| `encoding/hex` | Hash string representation | `hex.EncodeToString` for human-readable SHA-256 hashes in logs and plan output. |

**Implementation pattern for SHA-256 verification:**
```go
func HashFile(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", err
    }
    return hex.EncodeToString(h.Sum(nil)), nil
}
```

**Cross-filesystem move pattern:** Already exists in `internal/organize/mover.go`. Reuse for plan-based moves. Hash before move, move, hash after, compare.

### CSV Import

**Need:** Parse user-provided CSV files to create plans (bridge manual analysis to plan system).

| Stdlib Package | Purpose | Why Sufficient |
|----------------|---------|----------------|
| `encoding/csv` | CSV parsing | RFC 4180 compliant. Handles quoted fields, multiline values, custom delimiters. More than enough for structured plan import. |
| `os` | File reading | Standard file open/close. |
| `strconv` | Type conversion | Parse numeric fields from CSV strings if needed. |

**Why NOT csvutil or go-csvlib:** The CSV format for plan import will be earworm-defined (action_type, source, destination, etc.). Simple column-position or header-based parsing with `csv.Reader.Read()` is straightforward. Struct-tag mapping libraries add a dependency for ~10 lines of saved code.

**Expected CSV format:**
```
action,source_path,dest_path,notes
move,"/lib/Author/Wrong Title","/lib/Author/Right Title",Fix title
flatten,"/lib/Author/Book/nested/","/lib/Author/Book/",Remove nesting
delete,"/lib/Author/Book/.DS_Store","",macOS artifact
```

### Metadata Application (metadata.json)

**Need:** Write `metadata.json` files alongside audiobook folders. Read-only operation on audio files -- only creates new JSON files.

| Stdlib Package | Purpose | Why Sufficient |
|----------------|---------|----------------|
| `encoding/json` | JSON marshaling | `json.MarshalIndent` with 2-space indent for human-readable metadata files. |
| `os` | File writing | `os.WriteFile` for atomic-ish writes (write to temp, rename). |

**metadata.json structure** (Audiobookshelf-compatible):
```json
{
  "title": "Book Title",
  "author": "Author Name",
  "asin": "B00XXXXX",
  "series": "Series Name",
  "seriesSequence": "1"
}
```

**Write pattern:** Write to `metadata.json.tmp`, then `os.Rename` to `metadata.json` for crash safety. Avoids partial writes if interrupted.

### Execution Logging and Audit Trail

**Need:** Persistent log of all plan operations for debugging and accountability.

| Stdlib Package | Purpose | Why Sufficient |
|----------------|---------|----------------|
| `database/sql` | Log persistence | SQLite `execution_log` table. INSERT-only (append-only audit log). |
| `log/slog` | Structured runtime logging | Already used. Add plan-specific log attributes (plan_id, action_id, operation). |
| `time` | Timestamps | Microsecond-precision timestamps for operation ordering. |
| `fmt` | Detail formatting | Format operation details for human-readable log entries. |

**Design:** Dual logging -- slog for runtime terminal/file output, SQLite execution_log table for persistent queryable audit trail. The `earworm plan log <plan-id>` command queries the table.

### Claude Code Skill

**Need:** `.claude/skills/earworm-cleanup/SKILL.md` for conversational library cleanup orchestration.

**No Go dependencies.** This is a markdown file with YAML frontmatter that instructs Claude Code how to use earworm's CLI commands for library management.

**Skill location:** `.claude/skills/earworm-cleanup/SKILL.md`

**Skill capabilities:**
- Guide Claude through scan -> plan -> review -> apply workflow
- Reference earworm CLI commands and their `--json` output
- Include CSV format specification for plan import
- Define safety guardrails (always dry-run first, confirm deletions)

### Deep Library Scanning

**Need:** Scan all folders (not just ASIN-bearing ones), detect structural issues.

| Stdlib Package | Purpose | Why Sufficient |
|----------------|---------|----------------|
| `path/filepath` | Directory walking | `filepath.WalkDir` (Go 1.16+) is more efficient than `filepath.Walk` -- uses `fs.DirEntry` to avoid unnecessary `Stat` calls. |
| `os` | File system queries | `os.Stat`, `os.ReadDir` for directory inspection. |
| `strings` | Path/name analysis | Pattern matching for issue detection (empty dirs, nested structures, naming anomalies). |

**Why WalkDir over Walk:** `filepath.WalkDir` avoids calling `os.Lstat` on every file, which matters when scanning large NAS-mounted libraries over SMB/NFS where each stat is a network round-trip.

## What NOT to Add

| Library | Why Tempting | Why Not |
|---------|-------------|---------|
| `jszwec/csvutil` | Struct-tag CSV mapping | Only one CSV format to parse. 10 lines of manual parsing vs a dependency. |
| `go-resty/resty` | HTTP client for ABS API | Already decided against in v1.0. Still only 2-3 endpoints. |
| `hashicorp/go-multierror` | Aggregate errors from batch operations | Go 1.20+ `errors.Join` covers this. Stdlib. |
| `google/uuid` | Plan IDs | Already in go.mod as indirect dep. Use integer auto-increment IDs instead -- simpler, SQLite-native, sufficient for a local CLI tool. |
| `fsnotify/fsnotify` | Watch library for changes | Already in go.mod as indirect dep (viper). Not needed -- earworm is run-and-exit or polled via daemon. |
| Schema migration library (goose, migrate) | DB migrations | Existing hand-rolled migration runner works. Adding a migration framework for 3-4 new migrations is overkill. |
| `tidwall/gjson` | JSON querying | Not needed. We write JSON, not query complex nested JSON. |

## New Migrations Required

| Migration | Tables/Columns | Purpose |
|-----------|---------------|---------|
| `005_add_plans.sql` | `plans` table | Plan lifecycle tracking |
| `006_add_plan_actions.sql` | `plan_actions` table | Individual operations within a plan |
| `007_add_execution_log.sql` | `execution_log` table | Audit trail for applied operations |
| `008_add_scan_issues.sql` | `scan_issues` table | Deep scan issue tracking (orphan folders, naming problems, structural issues) |

## New Internal Packages

No external packages, but new internal packages are needed:

| Package | Purpose | Key Stdlib Dependencies |
|---------|---------|------------------------|
| `internal/plan/` | Plan CRUD, lifecycle management | `database/sql`, `encoding/json` |
| `internal/fileops/` | Flatten, split, move with SHA-256 verification | `os`, `io`, `crypto/sha256`, `path/filepath` |
| `internal/csvimport/` | CSV parsing into plan actions | `encoding/csv`, `os` |
| `internal/audit/` | Execution logging | `database/sql`, `log/slog`, `time` |

## New CLI Commands

| Command | Package | Purpose |
|---------|---------|---------|
| `earworm scan --deep` | `internal/cli/` | Deep library scan (extend existing scan) |
| `earworm plan create` | `internal/cli/` | Create plan from scan issues or manual |
| `earworm plan list` | `internal/cli/` | List plans with status |
| `earworm plan show <id>` | `internal/cli/` | Show plan details and actions |
| `earworm plan apply <id>` | `internal/cli/` | Execute plan (with --dry-run) |
| `earworm plan import <csv>` | `internal/cli/` | Import CSV as plan |
| `earworm plan log <id>` | `internal/cli/` | Show execution log for plan |
| `earworm cleanup <id>` | `internal/cli/` | Guarded deletion (plan's delete actions only, explicit confirm) |

## Configuration Additions (Viper)

| Key | Type | Default | Purpose |
|-----|------|---------|---------|
| `scan.deep` | bool | false | Enable deep scanning by default |
| `plan.auto_hash` | bool | true | SHA-256 verify after file operations |
| `plan.backup_deletes` | bool | true | Move to trash instead of hard delete |
| `plan.trash_dir` | string | `~/.config/earworm/trash/` | Soft-delete destination |

## Installation

```bash
# No new dependencies needed for v1.1
# Existing go.mod is sufficient
go mod tidy
```

## Sources

- [Go crypto/sha256 package](https://pkg.go.dev/crypto/sha256) -- stdlib SHA-256, confirmed io.Copy streaming pattern
- [Go encoding/csv package](https://pkg.go.dev/encoding/csv) -- RFC 4180 compliant CSV parser
- [Go encoding/json package](https://pkg.go.dev/encoding/json) -- MarshalIndent for metadata files
- [Go filepath.WalkDir](https://pkg.go.dev/path/filepath#WalkDir) -- efficient directory traversal without extra Stat calls
- [Claude Code Skills documentation](https://code.claude.com/docs/en/skills) -- .claude/skills/ format with YAML frontmatter
- [SHA-256 file hashing in Go](https://transloadit.com/devtips/verify-file-integrity-with-go-and-sha256/) -- io.Copy pattern for large files
- [Go errors.Join](https://pkg.go.dev/errors#Join) -- Go 1.20+ multi-error aggregation, replaces hashicorp/go-multierror
