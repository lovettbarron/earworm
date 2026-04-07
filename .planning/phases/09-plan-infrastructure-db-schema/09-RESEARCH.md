# Phase 9: Plan Infrastructure & DB Schema - Research

**Researched:** 2026-04-07
**Domain:** Go SQLite schema design, plan/operation CRUD, audit logging
**Confidence:** HIGH

## Summary

Phase 9 establishes the foundational data layer for plan-based library operations. The existing codebase has a well-established pattern: modernc.org/sqlite with embedded SQL migrations, `*sql.DB` passed to package-level functions, and testify-based tests using in-memory SQLite. This phase extends that pattern with three new tables (library_items, plans + plan_operations, audit_log) and corresponding Go CRUD packages.

The key design challenge is modeling the relationship between plans, their typed operations, and the audit trail. The existing `books` table is ASIN-keyed and cannot represent non-Audible content. A new `library_items` table with path-based primary key fills this gap. Plans are a named container of ordered operations, each with a type (move, flatten, split, delete, write_metadata) and status tracking.

**Primary recommendation:** Follow the existing db package patterns exactly -- add migration 005, create new Go files for each domain (library_items.go, plans.go, audit.go) with package-level functions taking `*sql.DB`, and comprehensive table-driven tests.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PLAN-01 | User can create named plans with typed action records (move, flatten, split, delete, write_metadata) and per-action status tracking | Plans table + plan_operations table with status enum, Go CRUD functions in plans.go |
| SCAN-02 | Library items are tracked in a path-keyed DB table so plans can reference non-Audible content | library_items table with path TEXT PRIMARY KEY, Go CRUD in library_items.go |
| INTG-01 | All plan operations produce a full audit trail with timestamps, before/after state, and success/failure | audit_log table with JSON before/after columns, AuditLogger interface in audit.go |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go -- single binary, no CGo
- **Database:** modernc.org/sqlite with driver name "sqlite" (NOT "sqlite3"), WAL mode enabled
- **Migrations:** Embedded SQL via `//go:embed migrations/*.sql`, sequential numbered files, schema_versions tracking
- **Config:** Viper with YAML, config at ~/.config/earworm/config.yaml, DB at ~/.config/earworm/earworm.db
- **CLI:** Cobra commands in internal/cli/, one file per command, root has --quiet and --config flags
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests, viper.Reset() between config tests
- **Error handling:** Cobra RunE pattern, wrap errors with fmt.Errorf("context: %w", err)
- **Project structure:** `cmd/earworm/` entry point, `internal/` for private packages

## Standard Stack

### Core (Already in Project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| modernc.org/sqlite | v1.48.1 | SQLite database | Already in go.mod, CGo-free, established in project |
| database/sql (stdlib) | Go 1.26.1 | SQL interface | Standard Go database access pattern |
| encoding/json (stdlib) | Go 1.26.1 | JSON serialization for audit before/after state | Stdlib, zero dependency |

### Testing
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| testify/assert | v1.11.1 | Test assertions | Already in go.mod, all tests |
| testify/require | v1.11.1 | Fatal test assertions | Already in go.mod, setup/preconditions |

### No New Dependencies Required

This phase requires zero new Go dependencies. Everything is built on `database/sql` + `modernc.org/sqlite` already in go.mod, plus stdlib `encoding/json` for audit state serialization.

## Architecture Patterns

### New Files to Create
```
internal/
├── db/
│   ├── db.go                    # existing - Open, migrations
│   ├── books.go                 # existing - Book CRUD
│   ├── library_items.go         # NEW - LibraryItem CRUD
│   ├── library_items_test.go    # NEW
│   ├── plans.go                 # NEW - Plan + PlanOperation CRUD
│   ├── plans_test.go            # NEW
│   ├── audit.go                 # NEW - AuditLog CRUD
│   ├── audit_test.go            # NEW
│   └── migrations/
│       ├── 001_initial.sql       # existing
│       ├── 002_add_metadata_fields.sql  # existing
│       ├── 003_add_audible_fields.sql   # existing
│       ├── 004_add_download_tracking.sql # existing
│       └── 005_plan_infrastructure.sql   # NEW
```

### Pattern 1: Migration File (005_plan_infrastructure.sql)

The existing migration pattern uses plain SQL ALTER TABLE / CREATE TABLE statements, one migration per file, sequential numbering. The new migration creates all three tables in one file since they are logically related and deployed together.

**Schema design:**

```sql
-- Library items: path-keyed table for all library content (ASIN and non-ASIN)
CREATE TABLE IF NOT EXISTS library_items (
    path TEXT PRIMARY KEY,
    item_type TEXT NOT NULL DEFAULT 'unknown',  -- 'book', 'audiobook', 'podcast', 'unknown'
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    asin TEXT NOT NULL DEFAULT '',               -- empty for non-Audible content
    folder_name TEXT NOT NULL DEFAULT '',
    file_count INTEGER NOT NULL DEFAULT 0,
    total_size_bytes INTEGER NOT NULL DEFAULT 0,
    has_cover INTEGER NOT NULL DEFAULT 0,
    metadata_source TEXT NOT NULL DEFAULT '',    -- 'tag', 'folder', 'sidecar', ''
    last_scanned_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_library_items_asin ON library_items(asin);
CREATE INDEX IF NOT EXISTS idx_library_items_type ON library_items(item_type);

-- Plans: named containers for a set of operations
CREATE TABLE IF NOT EXISTS plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',  -- 'draft', 'ready', 'running', 'completed', 'failed', 'cancelled'
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);

-- Plan operations: individual actions within a plan
CREATE TABLE IF NOT EXISTS plan_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES plans(id),
    seq INTEGER NOT NULL,                      -- execution order within the plan
    op_type TEXT NOT NULL,                     -- 'move', 'flatten', 'split', 'delete', 'write_metadata'
    source_path TEXT NOT NULL,
    dest_path TEXT NOT NULL DEFAULT '',        -- empty for delete/flatten (computed at runtime)
    status TEXT NOT NULL DEFAULT 'pending',    -- 'pending', 'running', 'completed', 'failed', 'skipped'
    error_message TEXT NOT NULL DEFAULT '',
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_plan_operations_plan_id ON plan_operations(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_operations_status ON plan_operations(status);

-- Audit log: immutable record of all plan mutations
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,                 -- 'plan', 'operation', 'library_item'
    entity_id TEXT NOT NULL,                   -- plan ID, operation ID, or path
    action TEXT NOT NULL,                      -- 'create', 'status_change', 'update', 'delete'
    before_state TEXT NOT NULL DEFAULT '',     -- JSON snapshot
    after_state TEXT NOT NULL DEFAULT '',      -- JSON snapshot
    success INTEGER NOT NULL DEFAULT 1,       -- 0 = failure, 1 = success
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
```

### Pattern 2: Go Struct + CRUD Functions (Following books.go Pattern)

The project uses package-level functions with `*sql.DB` as first parameter -- not methods on a struct, not repository pattern. Follow this exactly.

```go
// plans.go - following existing books.go pattern

// ValidPlanStatuses defines allowed plan status values.
var ValidPlanStatuses = []string{"draft", "ready", "running", "completed", "failed", "cancelled"}

// ValidOpTypes defines allowed operation types.
var ValidOpTypes = []string{"move", "flatten", "split", "delete", "write_metadata"}

// ValidOpStatuses defines allowed operation status values.
var ValidOpStatuses = []string{"pending", "running", "completed", "failed", "skipped"}

type Plan struct {
    ID          int64
    Name        string
    Description string
    Status      string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type PlanOperation struct {
    ID           int64
    PlanID       int64
    Seq          int
    OpType       string
    SourcePath   string
    DestPath     string
    Status       string
    ErrorMessage string
    CompletedAt  *time.Time
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// CreatePlan creates a new plan and returns its ID.
func CreatePlan(db *sql.DB, name, description string) (int64, error) { ... }

// GetPlan retrieves a plan by ID. Returns nil if not found.
func GetPlan(db *sql.DB, id int64) (*Plan, error) { ... }

// ListPlans returns plans filtered by status (empty = all).
func ListPlans(db *sql.DB, status string) ([]Plan, error) { ... }

// UpdatePlanStatus updates a plan's status.
func UpdatePlanStatus(db *sql.DB, id int64, status string) error { ... }

// AddOperation adds an operation to a plan.
func AddOperation(db *sql.DB, op PlanOperation) (int64, error) { ... }

// ListOperations returns all operations for a plan, ordered by seq.
func ListOperations(db *sql.DB, planID int64) ([]PlanOperation, error) { ... }

// UpdateOperationStatus updates an operation's status and error.
func UpdateOperationStatus(db *sql.DB, id int64, status, errorMsg string) error { ... }
```

### Pattern 3: Audit Logger

The audit log should be written through helper functions that serialize state to JSON. This keeps the audit concern centralized.

```go
// audit.go

type AuditEntry struct {
    ID           int64
    EntityType   string  // "plan", "operation", "library_item"
    EntityID     string
    Action       string  // "create", "status_change", "update", "delete"
    BeforeState  string  // JSON
    AfterState   string  // JSON
    Success      bool
    ErrorMessage string
    CreatedAt    time.Time
}

// LogAudit writes an audit entry. before/after are JSON-serializable values.
func LogAudit(db *sql.DB, entry AuditEntry) error { ... }

// ListAuditEntries returns audit entries for a given entity, newest first.
func ListAuditEntries(db *sql.DB, entityType, entityID string) ([]AuditEntry, error) { ... }
```

**Integration pattern:** Plan CRUD functions should call LogAudit internally. For example, CreatePlan should log an audit entry with action "create" and the plan state as after_state. UpdatePlanStatus should capture before and after state.

**Transaction consideration:** When a plan status change and audit log must be atomic, wrap both in a single transaction. The existing codebase uses `db.Begin()` / `tx.Commit()` in migrations -- apply the same pattern for plan+audit writes.

```go
// UpdatePlanStatusAudited updates plan status and writes audit log atomically.
func UpdatePlanStatusAudited(db *sql.DB, id int64, newStatus string) error {
    // 1. Read current plan (before state)
    // 2. Begin transaction
    // 3. UPDATE plans SET status = ?
    // 4. INSERT INTO audit_log (...)
    // 5. Commit
}
```

### Anti-Patterns to Avoid

- **Do NOT use an ORM:** The project uses raw SQL with `database/sql`. No GORM, no sqlx, no ent. Stay consistent.
- **Do NOT create a repository interface:** The project uses package-level functions. An interface layer adds complexity for zero benefit at this scale.
- **Do NOT store Go structs as BLOB:** Use JSON text for audit state serialization. SQLite stores TEXT efficiently and it's human-readable for debugging.
- **Do NOT use AUTOINCREMENT on library_items:** Path is the natural primary key. AUTOINCREMENT would add an unnecessary surrogate key.
- **Do NOT split migration 005 into multiple files:** All three tables are one logical unit; shipping them together avoids partial migration states.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON serialization | Custom serializer | encoding/json Marshal/Unmarshal | Stdlib, handles all Go types, audit states are simple structs |
| UUID generation | UUID library | INTEGER AUTOINCREMENT for plans/ops, path TEXT for library_items | Natural keys exist; UUIDs add complexity without benefit for a local-only CLI tool |
| Migration framework | goose/migrate | Existing embedded SQL migration runner in db.go | Already works, already tested, adding a framework is unnecessary dependency |
| SQL query builder | squirrel/sqlc | Raw SQL strings | Consistent with existing codebase; queries are simple CRUD, not complex joins |

## Common Pitfalls

### Pitfall 1: SQLite Foreign Key Enforcement
**What goes wrong:** SQLite does NOT enforce foreign keys by default. `plan_operations.plan_id REFERENCES plans(id)` is a no-op without `PRAGMA foreign_keys = ON`.
**Why it happens:** SQLite's default is foreign_keys=OFF for backward compatibility.
**How to avoid:** Add `PRAGMA foreign_keys = ON` in db.Open() after WAL mode pragma, or enforce referential integrity in Go code. Given the existing codebase does not set this pragma and adding it could affect existing behavior, safer to enforce in Go code (check plan exists before inserting operation).
**Warning signs:** Operations referencing non-existent plan IDs succeed silently.

### Pitfall 2: Audit Log Growth
**What goes wrong:** Audit log grows unbounded, eventually slowing queries.
**Why it happens:** Every plan mutation writes an audit entry, and there's no pruning.
**How to avoid:** For Phase 9, this is fine -- a CLI tool processing hundreds of operations won't generate problematic volume. Add a VACUUM or pruning command later if needed. Index on created_at enables efficient time-bounded queries.
**Warning signs:** Query times on audit_log increasing (unlikely in v1.1 scale).

### Pitfall 3: JSON State Serialization Consistency
**What goes wrong:** Audit before/after states become unparseable if struct fields change between versions.
**Why it happens:** Go struct marshaling depends on field names and types at marshal time.
**How to avoid:** Use a simple, stable JSON structure for audit states. Include version field in serialized state if needed. For v1.1, plan/operation structs are stable enough that this is LOW risk.
**Warning signs:** json.Unmarshal failures on older audit entries after schema changes.

### Pitfall 4: Path Normalization for library_items
**What goes wrong:** Same directory gets two entries because paths differ (trailing slash, case, symlinks).
**Why it happens:** File paths from different sources may not be normalized identically.
**How to avoid:** Normalize paths before storage: filepath.Clean(), ensure no trailing slash, resolve to absolute path. Define a NormalizePath helper and use it consistently.
**Warning signs:** Duplicate library_items with slightly different path values.

### Pitfall 5: Transaction Scope for Audited Operations
**What goes wrong:** Plan status updates succeed but audit log write fails (or vice versa), leaving inconsistent state.
**Why it happens:** Two writes without a transaction.
**How to avoid:** Wrap plan-status-change + audit-log-insert in a single `db.Begin()`/`tx.Commit()` transaction. Both succeed or both fail.
**Warning signs:** Audit entries missing for some status changes; orphaned audit entries.

## Code Examples

### Existing Migration Pattern (verified from codebase)
```sql
-- 005_plan_infrastructure.sql
-- Following pattern of 001-004: plain SQL, no down migrations

CREATE TABLE IF NOT EXISTS library_items ( ... );
CREATE TABLE IF NOT EXISTS plans ( ... );
CREATE TABLE IF NOT EXISTS plan_operations ( ... );
CREATE TABLE IF NOT EXISTS audit_log ( ... );
```

### Existing CRUD Pattern (verified from books.go)
```go
// Package-level function, *sql.DB as first param, error wrapping with fmt.Errorf
func CreatePlan(db *sql.DB, name, description string) (int64, error) {
    result, err := db.Exec(
        `INSERT INTO plans (name, description) VALUES (?, ?)`,
        name, description,
    )
    if err != nil {
        return 0, fmt.Errorf("create plan: %w", err)
    }
    id, err := result.LastInsertId()
    if err != nil {
        return 0, fmt.Errorf("get plan id: %w", err)
    }
    return id, nil
}
```

### Existing Test Pattern (verified from db_test.go)
```go
func TestCreatePlan(t *testing.T) {
    db := setupTestDB(t)  // reuse existing helper -- in-memory SQLite

    id, err := CreatePlan(db, "cleanup-2024", "January cleanup")
    require.NoError(t, err)
    assert.Greater(t, id, int64(0))

    plan, err := GetPlan(db, id)
    require.NoError(t, err)
    require.NotNil(t, plan)
    assert.Equal(t, "cleanup-2024", plan.Name)
    assert.Equal(t, "draft", plan.Status)
}
```

### Audited Status Change Pattern
```go
func UpdatePlanStatusAudited(db *sql.DB, id int64, newStatus string) error {
    if !isValidPlanStatus(newStatus) {
        return fmt.Errorf("invalid plan status %q", newStatus)
    }

    // Capture before state
    plan, err := GetPlan(db, id)
    if err != nil {
        return fmt.Errorf("get plan for audit: %w", err)
    }
    if plan == nil {
        return fmt.Errorf("plan %d not found", id)
    }
    oldStatus := plan.Status

    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }

    if _, err := tx.Exec(
        `UPDATE plans SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
        newStatus, id,
    ); err != nil {
        tx.Rollback()
        return fmt.Errorf("update plan status: %w", err)
    }

    beforeJSON, _ := json.Marshal(map[string]string{"status": oldStatus})
    afterJSON, _ := json.Marshal(map[string]string{"status": newStatus})

    if _, err := tx.Exec(
        `INSERT INTO audit_log (entity_type, entity_id, action, before_state, after_state, success)
         VALUES ('plan', ?, 'status_change', ?, ?, 1)`,
        fmt.Sprintf("%d", id), string(beforeJSON), string(afterJSON),
    ); err != nil {
        tx.Rollback()
        return fmt.Errorf("write audit log: %w", err)
    }

    return tx.Commit()
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed -- `go test ./...` |
| Quick run command | `go test ./internal/db/ -run TestPlan -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PLAN-01 | Create plan with typed operations | unit | `go test ./internal/db/ -run TestCreatePlan -count=1` | Wave 0 |
| PLAN-01 | Operation type validation | unit | `go test ./internal/db/ -run TestAddOperation -count=1` | Wave 0 |
| PLAN-01 | Per-action status tracking | unit | `go test ./internal/db/ -run TestUpdateOperationStatus -count=1` | Wave 0 |
| SCAN-02 | library_items path-keyed table | unit | `go test ./internal/db/ -run TestLibraryItem -count=1` | Wave 0 |
| SCAN-02 | Upsert library item | unit | `go test ./internal/db/ -run TestUpsertLibraryItem -count=1` | Wave 0 |
| INTG-01 | Audit log on plan create | unit | `go test ./internal/db/ -run TestAuditPlanCreate -count=1` | Wave 0 |
| INTG-01 | Audit log on status change | unit | `go test ./internal/db/ -run TestAuditStatusChange -count=1` | Wave 0 |
| INTG-01 | Audit before/after state | unit | `go test ./internal/db/ -run TestAuditBeforeAfter -count=1` | Wave 0 |
| ALL | Migration 005 applied correctly | unit | `go test ./internal/db/ -run TestMigration005 -count=1` | Wave 0 |
| ALL | Persistence across restart | unit | `go test ./internal/db/ -run TestPersistence -count=1` (temp file DB) | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/db/ -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before /gsd:verify-work

### Wave 0 Gaps
- [ ] `internal/db/plans_test.go` -- covers PLAN-01
- [ ] `internal/db/library_items_test.go` -- covers SCAN-02
- [ ] `internal/db/audit_test.go` -- covers INTG-01

*(Test infrastructure exists -- setupTestDB helper, testify, in-memory SQLite all already working)*

## Open Questions

1. **library_items relationship to books table**
   - What we know: The existing `books` table is ASIN-keyed. library_items is path-keyed.
   - What's unclear: Should library_items have a foreign key to books.asin for items that ARE Audible books? Or should they be independent tables?
   - Recommendation: Keep them independent. library_items.asin is an informational field, not a foreign key. The books table is the Audible-centric view; library_items is the filesystem-centric view. They overlap for Audible content but serve different purposes. Phase 10 (deep scanner) will populate library_items including for ASIN-bearing folders.

2. **Plan operation ordering**
   - What we know: Operations need a sequence number for execution order.
   - What's unclear: Should reordering be supported in Phase 9?
   - Recommendation: No. Add seq as an integer, insert in order. Reordering (if ever needed) is a Phase 12+ concern. Keep the CRUD simple.

## Sources

### Primary (HIGH confidence)
- Existing codebase: `internal/db/db.go`, `internal/db/books.go`, `internal/db/migrations/001-004` -- verified migration pattern, CRUD pattern, test pattern
- `go.mod` -- confirmed modernc.org/sqlite v1.48.1, testify v1.11.1, Go 1.26.1

### Secondary (MEDIUM confidence)
- SQLite documentation on foreign keys pragma -- well-known behavior, foreign_keys default OFF

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- zero new dependencies, all patterns established in codebase
- Architecture: HIGH -- direct extension of existing db package patterns
- Pitfalls: HIGH -- common SQLite/Go patterns, verified against codebase

**Research date:** 2026-04-07
**Valid until:** 2026-05-07 (stable -- no external dependency changes expected)
