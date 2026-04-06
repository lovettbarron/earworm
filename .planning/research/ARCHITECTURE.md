# Architecture Patterns

**Domain:** v1.1 Library Cleanup — integration into existing Go CLI audiobook manager
**Researched:** 2026-04-06

## Current Architecture (v1.0)

```
cmd/earworm/          -- Binary entry point
internal/
  cli/                -- Cobra command definitions (one file per command)
  config/             -- Viper setup, defaults, validation
  db/                 -- SQLite database, migrations, Book CRUD (raw sql.DB)
  scanner/            -- Library directory scanning, ASIN extraction
  metadata/           -- M4A tag reading (dhowden/tag, ffprobe, folder)
  audible/            -- audible-cli subprocess wrapper
  pipeline/           -- Download pipeline (rate limiter, backoff, progress)
  organize/           -- File moves: staging -> library (cross-FS, Libation paths)
  audiobookshelf/     -- ABS API client (scan trigger)
  goodreads/          -- Goodreads CSV exporter
  venv/               -- Python venv auto-management for audible-cli
  daemon/             -- Polling loop for unattended operation
```

**Key patterns established:**
- Raw `*sql.DB` passed directly (no repository interface layer)
- Package-level functions (`db.InsertBook(db, book)`) not methods on a struct
- Embedded SQL migrations via `//go:embed`, sequential numbered files (001-004)
- `organize.MoveFile` handles cross-filesystem moves with size verification
- Scanner returns `[]DiscoveredBook` + `[]SkippedDir` (currently ASIN-only)
- Metadata extraction is read-only with fallback chain: tag -> ffprobe -> folder
- CLI uses Cobra RunE pattern, one file per command in `internal/cli/`

## Recommended Architecture for v1.1

### New Packages

| Package | Responsibility | Communicates With |
|---------|---------------|-------------------|
| `internal/plan/` | Plan lifecycle: create, review, apply, status | `db`, `fileops`, `audit` |
| `internal/fileops/` | Structural file operations: flatten, split, verify, delete | `organize` (reuse MoveFile), `audit` |
| `internal/csvimport/` | Parse CSV into plan operations | `plan`, `db` |
| `internal/audit/` | Execution logging and audit trail | `db` |

### Modified Packages

| Package | Changes | Why |
|---------|---------|-----|
| `internal/db/` | New migration(s) for plans, plan_operations, audit_log tables; new CRUD files | Plan persistence and audit trail |
| `internal/scanner/` | Deep scan mode: scan ALL folders, classify issues | Detect non-Audible books, structural problems |
| `internal/metadata/` | Add `WriteMetadataJSON()` function | metadata.json sidecar generation |
| `internal/cli/` | New commands: `plan`, `cleanup`, `import` | User-facing plan workflow |
| `internal/config/` | New config keys for cleanup behavior defaults | Guarded defaults |

### Packages NOT Modified

| Package | Why Unchanged |
|---------|--------------|
| `internal/audible/` | Cleanup features don't interact with Audible downloads |
| `internal/pipeline/` | Download pipeline is unrelated to library restructuring |
| `internal/audiobookshelf/` | Existing scan trigger is sufficient post-cleanup |
| `internal/goodreads/` | Unrelated to library structure operations |
| `internal/venv/` | Python venv management is download-only |
| `internal/daemon/` | Daemon polling is for download cycle, not cleanup |

## Component Design

### 1. Plan Infrastructure (`internal/plan/`)

The plan is the central abstraction for v1.1. Every structural change goes through: **create plan -> review -> apply -> (optional) cleanup**.

```go
// Plan represents a set of operations to perform on the library.
type Plan struct {
    ID             int64
    Name           string
    Status         string    // "draft", "reviewed", "applying", "applied", "failed"
    Source         string    // "manual", "csv", "scan", "claude"
    CreatedAt      time.Time
    AppliedAt      *time.Time
    OperationCount int
    Notes          string
}

// Operation represents a single action within a plan.
type Operation struct {
    ID         int64
    PlanID     int64
    Seq        int       // execution order within plan
    Type       string    // "move", "rename", "flatten", "split", "metadata", "delete"
    SourcePath string
    DestPath   string    // empty for delete, metadata ops
    Metadata   string    // JSON blob for operation-specific data
    Status     string    // "pending", "applied", "failed", "skipped"
    Error      string
    AppliedAt  *time.Time
}
```

**Plan engine design:** The plan engine is a state machine, not a pipeline. Plans transition through statuses atomically. Operations within a plan execute sequentially with individual success/failure tracking. A failed operation does NOT roll back previous operations -- it marks the plan as "failed" and stops, preserving what was done in the audit log.

**Why no rollback:** File operations on a NAS over SMB/NFS are not transactional. Attempting rollback after partial completion risks data loss. A "rollback" that itself fails midway leaves the library in a worse state than stopping did. Instead: audit everything, stop on failure, let the user decide how to proceed.

```go
// Engine executes plans against the filesystem.
type Engine struct {
    db      *sql.DB
    fileops *fileops.Executor
    audit   *audit.Logger
}

func (e *Engine) Apply(ctx context.Context, planID int64, dryRun bool) (*ApplyResult, error)
func (e *Engine) CreateFromScan(issues []scanner.FolderIssue, name string) (int64, error)
func (e *Engine) CreateFromCSV(rows []csvimport.CSVRow, name string) (int64, error)
```

**Dry-run is the default.** The `Apply` method accepts a `dryRun` bool. When true, it walks all operations, validates paths exist, checks for conflicts, and returns what WOULD happen without touching the filesystem. The CLI defaults to dry-run and requires `--confirm` to actually execute.

### 2. Database Schema Additions (`internal/db/`)

New migration file: `005_add_plans.sql`

```sql
CREATE TABLE plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    source TEXT NOT NULL DEFAULT 'manual',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    applied_at DATETIME,
    notes TEXT DEFAULT ''
);

CREATE TABLE plan_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES plans(id),
    seq INTEGER NOT NULL,
    op_type TEXT NOT NULL,
    source_path TEXT NOT NULL,
    dest_path TEXT DEFAULT '',
    metadata TEXT DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT DEFAULT '',
    applied_at DATETIME,
    UNIQUE(plan_id, seq)
);

CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER REFERENCES plans(id),
    operation_id INTEGER REFERENCES plan_operations(id),
    action TEXT NOT NULL,
    source_path TEXT,
    dest_path TEXT,
    checksum TEXT,
    bytes_affected INTEGER,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    success INTEGER NOT NULL DEFAULT 1,
    error TEXT DEFAULT ''
);

CREATE INDEX idx_plan_operations_plan_id ON plan_operations(plan_id);
CREATE INDEX idx_audit_log_plan_id ON audit_log(plan_id);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
```

**Why separate from books table:** Plans operate on arbitrary folders, not just ASIN-tracked books. A non-Audible book has no ASIN and thus no entry in the books table. The plan system must be decoupled from book identity.

**DB function pattern:** Follow the established convention -- package-level functions taking `*sql.DB`:

```go
// internal/db/plans.go
func InsertPlan(db *sql.DB, plan Plan) (int64, error)
func GetPlan(db *sql.DB, id int64) (*Plan, error)
func ListPlans(db *sql.DB) ([]Plan, error)
func UpdatePlanStatus(db *sql.DB, id int64, status string) error

// internal/db/operations.go
func InsertOperation(db *sql.DB, op Operation) error
func ListPlanOperations(db *sql.DB, planID int64) ([]Operation, error)
func UpdateOperationResult(db *sql.DB, opID int64, status, errMsg string) error

// internal/db/audit.go
func InsertAuditEntry(db *sql.DB, entry AuditEntry) error
func ListAuditEntries(db *sql.DB, planID int64) ([]AuditEntry, error)
```

### 3. Deep Scanner Enhancement (`internal/scanner/`)

The current scanner only finds ASIN-bearing folders. v1.1 needs a deep scan that finds ALL folders and classifies them.

```go
// FolderIssue describes a problem detected in a library folder.
type FolderIssue struct {
    Path        string
    IssueType   string   // "no_asin", "nested_audio", "multi_book", "empty", "naming_mismatch"
    Description string
    Suggestion  string   // human-readable fix suggestion
}

// DeepScanResult extends the existing scan with issue detection.
type DeepScanResult struct {
    Discovered []DiscoveredBook
    Skipped    []SkippedDir
    Issues     []FolderIssue
    TotalDirs  int
    AudioFiles int
}

func DeepScan(root string) (*DeepScanResult, error)
```

**Integration point:** `DeepScan` reuses `ExtractASIN`, `findAudioFiles`, and the existing walk logic. It adds issue detection as a second pass. The existing `ScanLibrary` and `IncrementalSync` functions are NOT modified -- deep scan is additive.

**Issue detection heuristics:**
- `nested_audio`: Audio files exist in subdirectories of a book folder (should be flat)
- `multi_book`: Multiple distinct titles detected in one folder (by metadata comparison)
- `no_asin`: Folder with audio files but no ASIN (non-Audible book)
- `empty`: Folder in library structure with no audio files
- `naming_mismatch`: Folder name doesn't match audio file metadata

### 4. File Operations (`internal/fileops/`)

Structural operations need their own package because they are fundamentally different from `organize` (which moves from staging to library). These operate WITHIN the library.

```go
// Executor performs verified file operations with audit logging.
type Executor struct {
    audit  *audit.Logger
    dryRun bool
}

// Flatten moves audio files from subdirectories into the parent book directory.
func (e *Executor) Flatten(bookDir string) (*FlattenResult, error)

// Split separates a multi-book folder into individual book folders.
func (e *Executor) Split(sourceDir string, targets []SplitTarget) (*SplitResult, error)

// Delete removes a file or empty directory, logging to audit trail.
func (e *Executor) Delete(path string) error

// VerifyChecksum computes SHA-256 of a file and compares to expected.
func VerifyChecksum(path string, expected string) (bool, string, error)

// FileMove records a single file move with verification.
type FileMove struct {
    Source     string
    Dest       string
    SHA256     string
    BytesMoved int64
}
```

**Reuse from organize:** `fileops` imports and uses `organize.MoveFile` for the actual move operation (which already handles cross-FS moves with size verification). It adds SHA-256 verification on top for split operations.

**SHA-256 for splits:** When splitting a multi-book folder, files are being reorganized and the user needs confidence nothing was corrupted. Compute SHA-256 before move, verify after. For flatten operations (lifting files one level), the existing size verification in MoveFile is sufficient.

### 5. Metadata Writing (`internal/metadata/`)

Add to the existing metadata package. Currently read-only; v1.1 adds JSON sidecar writing.

```go
// MetadataJSON represents the metadata.json file written to book directories.
type MetadataJSON struct {
    Title        string `json:"title"`
    Author       string `json:"author"`
    Narrator     string `json:"narrator,omitempty"`
    Series       string `json:"series,omitempty"`
    SeriesPos    string `json:"series_position,omitempty"`
    ASIN         string `json:"asin,omitempty"`
    Genre        string `json:"genre,omitempty"`
    Year         int    `json:"year,omitempty"`
    Duration     int    `json:"duration_seconds,omitempty"`
    ChapterCount int    `json:"chapter_count,omitempty"`
    Source       string `json:"metadata_source"`
    GeneratedAt  string `json:"generated_at"`
    GeneratedBy  string `json:"generated_by"`
}

// WriteMetadataJSON writes a metadata.json file to the given directory.
// Does NOT overwrite existing metadata.json unless force is true.
func WriteMetadataJSON(dir string, meta MetadataJSON, force bool) error
```

**Why metadata.json, not audio tag writing:** Audio tag writing requires CGo (taglib) or WASM, adding complexity and risk. metadata.json is a sidecar file that Audiobookshelf reads natively. It achieves the goal (fix metadata for non-Audible books) without risking audio file corruption. This is correct for a library manager.

### 6. CSV Import (`internal/csvimport/`)

```go
// CSVRow represents a single row from a cleanup CSV.
type CSVRow struct {
    SourcePath string
    Action     string  // "move", "rename", "flatten", "split", "metadata", "delete", "skip"
    DestPath   string
    Title      string
    Author     string
    ASIN       string
    Notes      string
}

// ParseError describes a validation problem with a CSV row.
type ParseError struct {
    Line    int
    Column  string
    Message string
}

// ParseCSV reads a CSV file and returns validated rows plus any parse errors.
func ParseCSV(path string) ([]CSVRow, []ParseError, error)
```

**CSV format:** Use Go's `encoding/csv` from stdlib. No external dependency. The CSV bridges manual spreadsheet analysis into the plan system.

**Expected columns:** `source_path,action,dest_path,title,author,asin,notes`. Header row required. Unknown columns ignored (forward-compatible).

### 7. Audit Trail (`internal/audit/`)

```go
// Logger writes audit entries to the database.
type Logger struct {
    db *sql.DB
}

func NewLogger(db *sql.DB) *Logger

// Log records a single action in the audit trail.
func (l *Logger) Log(entry AuditEntry) error

// AuditEntry represents a single auditable action.
type AuditEntry struct {
    PlanID      *int64
    OperationID *int64
    Action      string  // "file_moved", "file_deleted", "metadata_written", etc.
    SourcePath  string
    DestPath    string
    Checksum    string
    Bytes       int64
    Success     bool
    Error       string
}
```

**Why a struct instead of package-level functions:** Unlike the existing db package pattern, the audit logger is injected into `fileops.Executor` and `plan.Engine`. Using a struct avoids threading `*sql.DB` through every file operation call. This is the one intentional divergence from the established pattern, and it's justified by the injection need.

### 8. CLI Commands (`internal/cli/`)

New command files following the established one-file-per-command pattern:

| File | Command | Subcommands | Purpose |
|------|---------|-------------|---------|
| `plan.go` | `earworm plan` | `create`, `list`, `show`, `apply`, `status` | Plan lifecycle |
| `cleanup.go` | `earworm cleanup` | (none, but --confirm required) | Guarded deletion of applied plan artifacts |
| `import.go` | `earworm import csv` | (none) | CSV file to plan conversion |

**Command tree:**
```
earworm plan create --from-scan          # Create plan from deep scan issues
earworm plan create --from-csv FILE      # Create plan from CSV import
earworm plan list                        # List all plans with status
earworm plan show PLAN_ID                # Show operations in a plan
earworm plan apply PLAN_ID               # Dry-run by default
earworm plan apply PLAN_ID --confirm     # Actually execute
earworm plan status PLAN_ID              # Show apply progress and audit trail
earworm cleanup PLAN_ID --confirm        # Delete source files for completed moves
earworm import csv FILE                  # Shorthand for plan create --from-csv
```

**`--confirm` pattern:** Both `plan apply` and `cleanup` default to dry-run/preview. The `--confirm` flag is required to mutate the filesystem. This extends the established pattern from v1.0's `download --dry-run`.

### 9. Claude Code Skill

The Claude Code skill is a `.claude/` directory file that teaches Claude how to interact with earworm commands. It does NOT require MCP, server integration, or code changes to earworm.

```
.claude/commands/library-cleanup.md
```

The skill file documents:
- How to run `earworm scan --deep --json` and interpret output
- How to create plans from scan issues or CSV
- How to review and apply plans
- Safety rules (never skip --confirm, always dry-run first)

**Why not MCP:** MCP would require earworm to run as a server, fundamentally changing its run-and-exit CLI architecture. The Claude Code skill approach lets Claude orchestrate existing CLI commands as subprocesses. This is correct for v1.1 -- MCP is a v2 consideration if ever.

## Data Flow

### Plan Creation Flow

```
Deep Scan ──> FolderIssues ──> plan.CreateFromScan() ──> Plan (draft)
CSV File  ──> csvimport.Parse ──> plan.CreateFromCSV() ──> Plan (draft)
```

### Plan Execution Flow

```
Plan (draft)
  |
  +-- earworm plan show ID       --> Display operations (review)
  |
  +-- earworm plan apply ID      --> Dry-run: validate all ops, report
  |
  +-- earworm plan apply ID --confirm
        |
        +-- For each operation (sequential):
        |     +-- fileops.Executor performs operation
        |     +-- audit.Logger records result
        |     +-- db updates operation status
        |
        +-- On success: Plan status --> "applied"
        +-- On failure: Plan status --> "failed", stop at failed op
```

### Cleanup Flow (Separated from Apply)

```
earworm cleanup PLAN_ID --confirm
  |
  +-- Verify plan status is "applied"
  +-- List operations that left source files (moves/splits)
  +-- Display what will be deleted
  +-- Delete source files, audit each deletion
```

**Why cleanup is separate:** Deletions are irreversible. By separating them from the apply step, the user can verify the restructured library works correctly (play a book in Audiobookshelf, check metadata) before committing to removing originals. This is the single most important safety design decision in v1.1.

## Patterns to Follow

### Pattern 1: Operation as Data
Operations are database records, not imperative function calls. This enables dry-run, review, audit, and resume-after-failure.

### Pattern 2: Dry-Run Default
Every command that mutates files defaults to showing what it would do. `--confirm` required for real execution.

### Pattern 3: Audit Everything
Every file operation records: what was done, source, dest, checksum, timestamp, success/failure.

### Pattern 4: Sequential Operations, Individual Status
Operations within a plan execute one-at-a-time with individual status tracking. If operation 47 fails, you know exactly which 46 succeeded and can resume or fix.

### Pattern 5: Additive Extension
New features ADD packages and functions. Existing v1.0 code paths (scan, download, organize) are not modified. Deep scan is a new function alongside the existing `ScanLibrary`.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Transactional File Operations
**What:** Trying to rollback filesystem changes if a later operation fails.
**Why bad:** NAS over SMB/NFS is not transactional. Partial rollback risks data loss worse than stopping.
**Instead:** Stop on failure, audit everything, let the user decide.

### Anti-Pattern 2: In-Memory Plans
**What:** Building plans as in-memory data structures, executing, then discarding.
**Why bad:** No review step, no audit trail, no resume after crash.
**Instead:** Persist plans to SQLite. Cheap to store, invaluable for debugging.

### Anti-Pattern 3: Coupling Plans to Books Table
**What:** Making plan operations reference the books table by ASIN foreign key.
**Why bad:** Non-Audible books have no ASIN. The plan system must work for ANY folder.
**Instead:** Plans reference filesystem paths. Store ASIN in operation metadata JSON when available, but don't make it a FK.

### Anti-Pattern 4: Audio File Modification
**What:** Writing metadata tags directly into M4A/M4B files.
**Why bad:** Risk of audio corruption, requires CGo/WASM taglib, some files are DRM-protected.
**Instead:** Write metadata.json sidecar files. Audiobookshelf reads these natively.

## Build Order (Dependency-Driven)

```
Phase 1: DB schema + audit logger
    +-- 005_add_plans.sql migration
    +-- internal/db/plans.go + operations.go + audit.go
    +-- internal/audit/logger.go
    Dependencies: existing db package only

Phase 2: Deep scanner
    +-- internal/scanner/ additions (DeepScan, FolderIssue)
    Dependencies: existing scanner package only (can parallel with Phase 1)

Phase 3: File operations
    +-- internal/fileops/ (Flatten, Split, VerifyChecksum, Delete)
    Dependencies: organize.MoveFile, audit.Logger (Phase 1)

Phase 4: Plan engine
    +-- internal/plan/ (Engine, Apply, CreateFromScan)
    Dependencies: db plans (Phase 1), fileops (Phase 3), audit (Phase 1)

Phase 5: Metadata writing
    +-- internal/metadata/ additions (WriteMetadataJSON)
    Dependencies: none (can parallel with Phases 3-4)

Phase 6: CSV import
    +-- internal/csvimport/ (ParseCSV, ToPlan)
    Dependencies: plan types (Phase 4)

Phase 7: CLI commands
    +-- internal/cli/ (plan.go, cleanup.go, import.go)
    Dependencies: all above packages

Phase 8: Claude Code skill
    +-- .claude/commands/library-cleanup.md
    Dependencies: CLI being stable (Phase 7)
```

**Parallelizable:** Phase 2 (deep scanner) and Phase 5 (metadata writing) have no dependencies on other new packages.

## Integration Points Summary

| New Component | Integrates With | Integration Type |
|---------------|----------------|------------------|
| `db/plans.go` | Existing `db.Open()`, migrations | Same DB, new tables |
| `db/audit.go` | Existing `db.Open()`, migrations | Same DB, new table |
| `scanner.DeepScan` | Existing `ExtractASIN`, `findAudioFiles` | Function reuse in same package |
| `fileops.Executor` | `organize.MoveFile` | Cross-package import |
| `fileops.Executor` | `audit.Logger` | Struct injection |
| `plan.Engine` | `fileops`, `audit`, `db` | Struct injection |
| `metadata.WriteMetadataJSON` | Existing `metadata` package | Same package, new exported function |
| `csvimport.ToPlan` | `plan` types | Type import |
| CLI commands | All of above | Cobra command wiring |
| Claude Code skill | CLI commands | External subprocess orchestration |

## Sources

- Codebase analysis of all `internal/` packages (direct code reading)
- Established conventions from v1.0: PROJECT.md, CLAUDE.md
- Existing migration pattern: `internal/db/migrations/001-004*.sql`
- Existing organize pattern: `organize.MoveFile` with cross-FS support
- Existing scanner pattern: `scanner.ScanLibrary`, `DiscoveredBook`, `SkippedDir`
- Audiobookshelf metadata.json sidecar support (ABS documentation)
