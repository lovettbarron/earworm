# Phase 13: CSV Import & Guarded Cleanup - Research

**Researched:** 2026-04-10
**Domain:** CSV parsing, safe file deletion, CLI confirmation patterns in Go
**Confidence:** HIGH

## Summary

Phase 13 adds two distinct capabilities: (1) importing plans from CSV files via `earworm plan import FILE.csv`, and (2) a guarded `earworm cleanup` command that processes delete operations from completed plans with trash-dir default and double confirmation. Both features build on the existing plan infrastructure (db.CreatePlan, db.AddOperation, planengine.Executor) and audit logging system.

The CSV import uses Go's stdlib `encoding/csv` which already handles CRLF normalization per RFC 4180. BOM stripping requires a 3-byte prefix check for UTF-8 BOM (0xEF, 0xBB, 0xBF). The cleanup command needs a new `cleanup.trash_dir` config key, a move-to-trash implementation (not os.Remove), and a two-step confirmation prompt. The existing planengine.Executor already handles delete operations via os.Remove -- cleanup must NOT use that path; it needs its own trash-based deletion that preserves files by default.

**Primary recommendation:** Build CSV import as a function in `internal/planengine/` (or a new `internal/csvimport/` package) with the CLI command in `internal/cli/plan.go`. Build cleanup as a new `internal/cli/cleanup.go` command that queries completed plans for delete operations and moves files to trash-dir instead of permanent deletion.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PLAN-04 | User can import plans from CSV spreadsheets to bridge manual analysis into the plan system | Go stdlib `encoding/csv` handles RFC 4180 + CRLF. BOM stripping is a 3-byte prefix check. Validation errors reported with line numbers via csv.Reader.FieldPos or manual line tracking. Existing db.CreatePlan + db.AddOperation provide the storage layer. |
| FOPS-03 | User can run a guarded cleanup command with trash-dir default, double confirmation, and audit logging -- separated from plan apply | New `cleanup.trash_dir` viper config key. os.Rename for same-filesystem trash move, copy+delete fallback for cross-filesystem (EXDEV). Double confirmation via bufio.Scanner stdin prompt. Audit via existing db.LogAudit. Must filter to only delete ops from completed plans. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go, single binary distribution
- **CLI:** Cobra commands in `internal/cli/`, one file per command, RunE pattern
- **Database:** modernc.org/sqlite with driver name "sqlite", WAL mode
- **Config:** Viper with YAML, config at ~/.config/earworm/config.yaml
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests
- **Error handling:** Cobra RunE pattern, wrap errors with `fmt.Errorf("context: %w", err)`
- **Established patterns:** Package-level Cobra flag vars reset in test helper, nested subcommand flags need separate reset loop

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| encoding/csv (stdlib) | Go 1.23+ | CSV parsing | RFC 4180 compliant, handles CRLF, built into Go. No external dependency needed. |
| os (stdlib) | Go 1.23+ | File operations (rename/remove for trash) | Trash-dir move is a rename or copy operation, stdlib handles both |
| bufio (stdlib) | Go 1.23+ | Interactive confirmation prompts | Reading stdin for double confirmation prompt |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| spf13/viper | v1.21.0 | cleanup.trash_dir config | Already in project, add new config key |
| spf13/cobra | v1.10.2 | CLI command registration | Already in project, add `plan import` and `cleanup` commands |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| encoding/csv | gocsv (gocarina/gocsv) | Struct mapping is nice but adds a dependency for a simple format. Not justified. |
| os.Rename for trash | XDG trash spec (freedesktop) | XDG trash is Linux-specific and complex. Simple directory move is sufficient for earworm's use case. |
| bufio stdin prompt | promptui or survey | Heavy TUI dependency for two yes/no prompts. Not justified. |

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── cli/
│   ├── plan.go          # Add planImportCmd subcommand
│   └── cleanup.go       # New file: cleanup command
├── planengine/
│   ├── engine.go         # Existing executor (unchanged)
│   └── csvimport.go      # New file: CSV parsing + validation
├── config/
│   └── config.go         # Add cleanup.trash_dir default
└── db/
    └── plans.go           # May need ListDeleteOperations query
```

### Pattern 1: CSV Import with Line-Level Validation
**What:** Parse CSV, validate each row, collect errors with line numbers, create plan only if all rows valid (or report partial with errors)
**When to use:** Any CSV import that needs user-friendly error reporting

Expected CSV format:
```csv
op_type,source_path,dest_path
delete,/library/Bad Book/,
move,/library/Old/book.m4a,/library/New/book.m4a
```

```go
// internal/planengine/csvimport.go
type CSVImportResult struct {
    PlanID     int64
    RowCount   int
    ErrorCount int
    Errors     []CSVRowError
}

type CSVRowError struct {
    Line    int
    Column  string
    Message string
}

func ImportCSV(db *sql.DB, planName string, r io.Reader) (*CSVImportResult, error) {
    // 1. Strip UTF-8 BOM if present (first 3 bytes: 0xEF, 0xBB, 0xBF)
    // 2. Create csv.Reader
    // 3. Read header row, validate required columns exist
    // 4. Read each data row, validate:
    //    - op_type is in db.ValidOpTypes
    //    - source_path is non-empty
    //    - dest_path is non-empty for move operations
    // 5. If no errors: CreatePlan, AddOperation for each row, return result
    // 6. If errors: return result with Errors populated, no plan created
}
```

### Pattern 2: Guarded Cleanup with Trash-Dir
**What:** Move files to a trash directory instead of permanent deletion. Require double confirmation. Only process delete operations from completed plans.
**When to use:** Any destructive file operation that should be reversible

```go
// internal/cli/cleanup.go
// Step 1: Query completed plans for delete operations
// Step 2: Display what will be trashed
// Step 3: First confirmation: "Move N files to trash? [y/N]"
// Step 4: Second confirmation: "This is irreversible from the library perspective. Confirm? [y/N]"
// Step 5: For each file: os.Rename(sourcePath, trashDir/basename) with EXDEV fallback
// Step 6: Log audit entry for each deletion
// Step 7: Mark operations as completed in DB
```

### Pattern 3: BOM Stripping
**What:** Handle UTF-8 BOM that Excel and Google Sheets add to CSV exports
```go
func StripBOM(r io.Reader) io.Reader {
    br := bufio.NewReader(r)
    b, err := br.Peek(3)
    if err == nil && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
        br.Discard(3)
    }
    return br
}
```

### Anti-Patterns to Avoid
- **Permanent deletion as default:** Always default to trash-dir move. The `--permanent` flag should exist but require explicit opt-in.
- **Importing CSV and immediately applying:** Import creates a draft plan. User must separately review and apply.
- **Accepting arbitrary delete paths:** Cleanup must only process delete operations from completed plans, never accept ad-hoc paths.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CSV parsing | Custom tokenizer | encoding/csv | RFC 4180 edge cases (quoted fields, embedded newlines, escaping) are deceptively hard |
| CRLF normalization | Manual \r\n replacement | encoding/csv (built-in) | Go's csv.Reader already strips \r before \n per docs |
| Config management | Manual YAML reading | viper.SetDefault + viper.GetString | Already established pattern in project |
| Audit logging | Custom log files | db.LogAudit | Existing audit_log table and LogAudit function handle this |

## Common Pitfalls

### Pitfall 1: UTF-8 BOM in CSV from Excel/Google Sheets
**What goes wrong:** First column header has invisible BOM prefix, causing header matching to fail
**Why it happens:** Excel saves CSV with BOM (0xEF 0xBB 0xBF). Go's csv.Reader does not strip BOM.
**How to avoid:** Read first 3 bytes, compare to BOM sequence, discard if present before passing to csv.Reader
**Warning signs:** "unknown column" errors on the first column when importing Excel-exported CSV

### Pitfall 2: Cross-Filesystem Trash Move
**What goes wrong:** os.Rename fails with EXDEV when trash-dir is on a different filesystem than the library (e.g., library on NAS, trash on local disk)
**Why it happens:** os.Rename cannot move across filesystem boundaries
**How to avoid:** Catch EXDEV error, fall back to copy + verify hash + delete source (same pattern as organize.MoveFile and fileops.VerifiedMove)
**Warning signs:** "invalid cross-device link" error message

### Pitfall 3: Duplicate Filenames in Trash
**What goes wrong:** Two files with the same basename from different directories clobber each other in trash
**Why it happens:** Trash dir is flat, but library has nested structure
**How to avoid:** Use a naming scheme that preserves uniqueness: either mirror directory structure in trash, or append a timestamp/hash suffix
**Warning signs:** Fewer files in trash than expected after cleanup

### Pitfall 4: Cleanup Without Completed Plan Guard
**What goes wrong:** User deletes files that haven't been through the plan review/apply cycle
**Why it happens:** Cleanup accepts arbitrary paths instead of only processing completed plan operations
**How to avoid:** Query only `plan_operations WHERE op_type = 'delete' AND status = 'completed'` from plans with `status = 'completed'`. Alternatively, require plan_id argument and check plan status.
**Warning signs:** Files deleted without appearing in any plan review output

### Pitfall 5: Test Helper Flag Reset
**What goes wrong:** New CLI flags (cleanupTrashDir, cleanupPermanent, etc.) leak between tests
**Why it happens:** Cobra flag vars are package-level and persist between test runs
**How to avoid:** Add all new flag variables to the executeCommand reset block in cli_test.go. Add cleanup subcommand flag reset loop similar to the plan subcommand loop.
**Warning signs:** Tests pass individually but fail when run together

### Pitfall 6: CSV Line Numbers Off By One
**What goes wrong:** Reported line numbers don't match what user sees in their spreadsheet
**Why it happens:** csv.Reader counts from 0, header row is line 1 in the file, data starts at line 2
**How to avoid:** Track line number as `headerLine + 1 + rowIndex` for error reporting. Account for the header row.
**Warning signs:** User opens CSV at reported line and sees a different row

## Code Examples

### CSV Import Function Skeleton
```go
// Source: Go stdlib encoding/csv docs + project patterns
package planengine

import (
    "bufio"
    "database/sql"
    "encoding/csv"
    "fmt"
    "io"
    "strings"

    "github.com/lovettbarron/earworm/internal/db"
)

func ImportCSV(database *sql.DB, planName string, r io.Reader) (*CSVImportResult, error) {
    // Strip BOM
    br := bufio.NewReader(r)
    if bom, err := br.Peek(3); err == nil && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
        br.Discard(3)
    }

    reader := csv.NewReader(br)
    reader.TrimLeadingSpace = true

    // Read header
    header, err := reader.Read()
    if err != nil {
        return nil, fmt.Errorf("read csv header: %w", err)
    }

    // Map column names to indices
    colMap := make(map[string]int)
    for i, h := range header {
        colMap[strings.ToLower(strings.TrimSpace(h))] = i
    }

    // Validate required columns exist
    for _, required := range []string{"op_type", "source_path"} {
        if _, ok := colMap[required]; !ok {
            return nil, fmt.Errorf("missing required column %q", required)
        }
    }

    // Parse rows
    var rows []db.PlanOperation
    var errors []CSVRowError
    lineNum := 1 // header is line 1

    for {
        lineNum++
        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            errors = append(errors, CSVRowError{Line: lineNum, Message: err.Error()})
            continue
        }

        opType := strings.TrimSpace(record[colMap["op_type"]])
        sourcePath := strings.TrimSpace(record[colMap["source_path"]])
        destPath := ""
        if idx, ok := colMap["dest_path"]; ok && idx < len(record) {
            destPath = strings.TrimSpace(record[idx])
        }

        // Validate
        if !db.IsValidOpType(opType) { // Note: need to export this
            errors = append(errors, CSVRowError{Line: lineNum, Column: "op_type", Message: fmt.Sprintf("invalid op type %q", opType)})
        }
        if sourcePath == "" {
            errors = append(errors, CSVRowError{Line: lineNum, Column: "source_path", Message: "source_path is required"})
        }
        if (opType == "move" || opType == "flatten") && destPath == "" {
            errors = append(errors, CSVRowError{Line: lineNum, Column: "dest_path", Message: "dest_path required for " + opType})
        }

        if len(errors) == 0 {
            rows = append(rows, db.PlanOperation{
                Seq:        lineNum - 1,
                OpType:     opType,
                SourcePath: sourcePath,
                DestPath:   destPath,
            })
        }
    }

    if len(errors) > 0 {
        return &CSVImportResult{ErrorCount: len(errors), Errors: errors}, nil
    }

    // Create plan and add operations
    planID, err := db.CreatePlan(database, planName, fmt.Sprintf("Imported from CSV (%d operations)", len(rows)))
    if err != nil {
        return nil, fmt.Errorf("create plan from csv: %w", err)
    }

    for i := range rows {
        rows[i].PlanID = planID
        if _, err := db.AddOperation(database, rows[i]); err != nil {
            return nil, fmt.Errorf("add operation %d: %w", i+1, err)
        }
    }

    return &CSVImportResult{PlanID: planID, RowCount: len(rows)}, nil
}
```

### Trash-Dir Move with EXDEV Fallback
```go
// Source: project pattern from internal/organize/mover.go
func MoveToTrash(sourcePath, trashDir string) error {
    // Build unique trash path: trashDir/<timestamp>_<basename>
    base := filepath.Base(sourcePath)
    trashPath := filepath.Join(trashDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), base))

    // Ensure trash dir exists
    if err := os.MkdirAll(trashDir, 0755); err != nil {
        return fmt.Errorf("create trash dir: %w", err)
    }

    // Try rename first (fast, same filesystem)
    err := os.Rename(sourcePath, trashPath)
    if err == nil {
        return nil
    }

    // EXDEV fallback: copy + verify + delete
    if errors.Is(err, syscall.EXDEV) {
        return copyAndDelete(sourcePath, trashPath)
    }
    return fmt.Errorf("move to trash: %w", err)
}
```

### Double Confirmation Prompt
```go
func confirmCleanup(w io.Writer, r io.Reader, fileCount int) bool {
    scanner := bufio.NewScanner(r)

    fmt.Fprintf(w, "Move %d files to trash? [y/N]: ", fileCount)
    if !scanner.Scan() || strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
        return false
    }

    fmt.Fprintf(w, "Are you sure? This removes files from the library. [y/N]: ")
    if !scanner.Scan() || strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
        return false
    }

    return true
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| encoding/csv FieldPos() | csv.Reader tracks line internally | Go 1.19+ | Can use reader.FieldPos() to get exact line/column for error reporting |
| Manual CRLF handling | csv.Reader strips CR automatically | Always (Go csv) | No manual CRLF work needed |

## Open Questions

1. **CSV Column Naming Convention**
   - What we know: Need at minimum op_type, source_path, dest_path columns
   - What's unclear: Should we support additional metadata columns (e.g., "reason", "priority")? Should column names be case-insensitive?
   - Recommendation: Case-insensitive header matching, ignore extra columns. Keep it simple for v1.

2. **Cleanup Scope: Per-Plan or All Completed Plans**
   - What we know: Cleanup processes delete operations from completed plans
   - What's unclear: Should `earworm cleanup` process ALL completed plans' delete ops, or require `earworm cleanup --plan-id N`?
   - Recommendation: Default to showing all pending deletes from completed plans. Optional `--plan-id` filter. User sees full picture by default.

3. **isValidOpType Export**
   - What we know: `isValidOpType` in db/plans.go is unexported (lowercase)
   - What's unclear: Should we export it or duplicate validation?
   - Recommendation: Export it as `IsValidOpType` -- CSV import needs it and it's a clean public API.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None (stdlib `go test`) |
| Quick run command | `go test ./internal/planengine/ ./internal/cli/ -run "CSV\|Cleanup\|Import" -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PLAN-04a | CSV import creates plan from valid CSV | unit | `go test ./internal/planengine/ -run TestImportCSV_Valid -count=1` | No -- Wave 0 |
| PLAN-04b | BOM stripping works on Excel CSV | unit | `go test ./internal/planengine/ -run TestImportCSV_BOM -count=1` | No -- Wave 0 |
| PLAN-04c | Row-level validation errors with line numbers | unit | `go test ./internal/planengine/ -run TestImportCSV_Errors -count=1` | No -- Wave 0 |
| PLAN-04d | CLI plan import command creates plan | integration | `go test ./internal/cli/ -run TestPlanImport -count=1` | No -- Wave 0 |
| FOPS-03a | Cleanup moves files to trash-dir (not permanent delete) | unit | `go test ./internal/planengine/ -run TestCleanup_TrashMove -count=1` | No -- Wave 0 |
| FOPS-03b | Cleanup only processes delete ops from completed plans | unit | `go test ./internal/planengine/ -run TestCleanup_OnlyCompleted -count=1` | No -- Wave 0 |
| FOPS-03c | Double confirmation prompt rejects on first N | unit | `go test ./internal/cli/ -run TestCleanup_Confirm -count=1` | No -- Wave 0 |
| FOPS-03d | Cleanup logs audit entries | unit | `go test ./internal/planengine/ -run TestCleanup_Audit -count=1` | No -- Wave 0 |
| FOPS-03e | CLI cleanup command end-to-end | integration | `go test ./internal/cli/ -run TestCleanupCommand -count=1` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/planengine/ ./internal/cli/ -run "CSV\|Cleanup\|Import" -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/planengine/csvimport_test.go` -- covers PLAN-04a, PLAN-04b, PLAN-04c
- [ ] `internal/planengine/cleanup_test.go` -- covers FOPS-03a, FOPS-03b, FOPS-03d (or in same file as engine tests)
- [ ] `internal/cli/cleanup_test.go` -- covers FOPS-03c, FOPS-03e (or added to plan_test.go for import tests)
- [ ] Export `IsValidOpType` from db/plans.go for CSV validation

## Sources

### Primary (HIGH confidence)
- Go stdlib encoding/csv docs -- CRLF handling confirmed ("Carriage returns before newline characters are silently removed")
- Go stdlib encoding/csv docs -- RFC 4180 compliance confirmed
- Existing codebase: internal/db/plans.go -- Plan, PlanOperation structs, CreatePlan, AddOperation, ValidOpTypes
- Existing codebase: internal/planengine/engine.go -- Executor.Apply, delete operation handling via os.Remove
- Existing codebase: internal/cli/plan.go -- existing plan subcommands pattern
- Existing codebase: internal/cli/cli_test.go -- executeCommand test helper with flag reset pattern
- Existing codebase: internal/fileops/hash.go -- VerifiedMove pattern for cross-filesystem moves
- Existing codebase: internal/config/config.go -- SetDefaults pattern for new config keys

### Secondary (MEDIUM confidence)
- UTF-8 BOM bytes (0xEF, 0xBB, 0xBF) -- well-established standard, verified against Unicode specification

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all stdlib, no new dependencies
- Architecture: HIGH -- follows established project patterns exactly
- Pitfalls: HIGH -- BOM and EXDEV are well-known Go issues, confirmed by codebase patterns

**Research date:** 2026-04-10
**Valid until:** 2026-05-10 (stable domain, no fast-moving dependencies)
