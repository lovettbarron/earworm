# Phase 10: Deep Library Scanner - Research

**Researched:** 2026-04-06
**Domain:** Go filesystem traversal, issue detection heuristics, SQLite persistence for scan results
**Confidence:** HIGH

## Summary

Phase 10 extends the existing ASIN-only scanner to discover ALL folders in the library and detect structural issues. The current scanner (`internal/scanner/scanner.go`) only processes folders containing ASINs -- non-ASIN folders are logged as "skipped" and forgotten. The deep scanner must walk every directory, classify it, populate the `library_items` table (from Phase 9), and persist detected issues in a new `scan_issues` table.

The key design challenge is issue detection heuristics. Eight issue types must be detected: `no_asin`, `nested_audio`, `multi_book`, `missing_metadata`, `wrong_structure`, `orphan_files`, `empty_dir`, `cover_missing`. Each has different detection logic, severity, and suggested action. These must be persisted in DB with severity and suggested action, surviving CLI restarts.

The existing scanner must not be broken -- `earworm scan` continues to work as before. The new `--deep` flag activates the extended behavior. This means the deep scanner is additive: it processes everything the ASIN scanner processes plus non-ASIN content, and additionally runs issue detection on all items.

**Primary recommendation:** Add a `--deep` flag to the existing scan command. Create a new `internal/scanner/deep.go` for the deep scan logic and `internal/scanner/issues.go` for issue detection. Add migration 006 for the `scan_issues` table. Populate `library_items` from Phase 9 during deep scan. Keep the existing ASIN scan path completely untouched.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCAN-01 | User can deep-scan all library folders (not just ASIN-bearing) and detect issues: no_asin, nested_audio, multi_book, missing_metadata, wrong_structure, orphan_files, empty_dir, cover_missing | Deep scanner walks all dirs, classifies each, runs issue detectors, populates library_items + scan_issues |
| SCAN-03 | Detected scan issues are persisted in DB with severity, category, and suggested action | scan_issues table with path FK to library_items, issue_type, severity, suggested_action columns |
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
- **File format:** M4A/M4B only for v1 (audio file detection uses .m4a and .m4b extensions)

## Standard Stack

### Core (Already in Project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| modernc.org/sqlite | v1.48.1 | SQLite database | Already in go.mod, CGo-free |
| database/sql (stdlib) | Go 1.26.1 | SQL interface | Standard Go DB access |
| path/filepath (stdlib) | Go 1.26.1 | Filesystem path handling | Used extensively in existing scanner |
| os (stdlib) | Go 1.26.1 | Directory reading, file stats | Used in existing scanner for ReadDir, Stat |
| io/fs (stdlib) | Go 1.26.1 | WalkDir interface | Used in existing recursive scan |

### Supporting (Already in Project)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| dhowden/tag | v0.0.0-20240417053706 | M4A metadata reading | For missing_metadata and cover_missing detection |
| log/slog (stdlib) | Go 1.26.1 | Structured logging | Warn/info during scan |

### No New Dependencies Required

This phase requires zero new Go dependencies. All functionality builds on stdlib filesystem operations and the existing metadata package.

## Architecture Patterns

### New Files to Create
```
internal/
├── scanner/
│   ├── scanner.go             # existing - ASIN scan (DO NOT MODIFY)
│   ├── asin.go                # existing - ASIN extraction (DO NOT MODIFY)
│   ├── deep.go                # NEW - DeepScan function, full traversal
│   ├── deep_test.go           # NEW
│   ├── issues.go              # NEW - issue detection logic
│   └── issues_test.go         # NEW
├── db/
│   ├── scan_issues.go         # NEW - ScanIssue CRUD
│   ├── scan_issues_test.go    # NEW
│   └── migrations/
│       └── 006_scan_issues.sql # NEW
├── cli/
│   └── scan.go                # MODIFY - add --deep flag
```

### Pattern 1: Migration 006 (scan_issues table)

```sql
-- Scan issues: detected problems in library folders
CREATE TABLE IF NOT EXISTS scan_issues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    issue_type TEXT NOT NULL,
    severity TEXT NOT NULL,           -- 'error', 'warning', 'info'
    message TEXT NOT NULL DEFAULT '',
    suggested_action TEXT NOT NULL DEFAULT '',
    scan_run_id TEXT NOT NULL DEFAULT '',  -- groups issues from one scan run
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scan_issues_path ON scan_issues(path);
CREATE INDEX IF NOT EXISTS idx_scan_issues_type ON scan_issues(issue_type);
CREATE INDEX IF NOT EXISTS idx_scan_issues_run ON scan_issues(scan_run_id);
```

Key design decisions:
- **No FK to library_items**: The scan_issues path is informational, not a strict FK. An issue might reference a path not yet in library_items (e.g., a file, not a directory). Keeping them decoupled avoids cascading complexity.
- **scan_run_id**: A UUID or timestamp string grouping all issues from one `--deep` scan. Enables clearing stale issues: DELETE WHERE scan_run_id != current_run before inserting new ones. This gives "latest scan results" semantics.
- **No updated_at**: Issues are immutable per scan run. Old issues are deleted, new ones inserted.

### Pattern 2: Issue Type Definitions

```go
// issues.go

// IssueType represents a category of library issue.
type IssueType string

const (
    IssueNoASIN           IssueType = "no_asin"
    IssueNestedAudio      IssueType = "nested_audio"
    IssueMultiBook        IssueType = "multi_book"
    IssueMissingMetadata  IssueType = "missing_metadata"
    IssueWrongStructure   IssueType = "wrong_structure"
    IssueOrphanFiles      IssueType = "orphan_files"
    IssueEmptyDir         IssueType = "empty_dir"
    IssueCoverMissing     IssueType = "cover_missing"
)

// Severity represents issue severity.
type Severity string

const (
    SeverityError   Severity = "error"
    SeverityWarning Severity = "warning"
    SeverityInfo    Severity = "info"
)

// DetectedIssue represents a problem found during deep scanning.
type DetectedIssue struct {
    Path            string
    IssueType       IssueType
    Severity        Severity
    Message         string
    SuggestedAction string
}
```

### Pattern 3: Issue Detection Heuristics

Each detector is a pure function that takes directory info and returns zero or more DetectedIssue values. This makes them independently testable.

| Issue Type | Detection Logic | Severity | Suggested Action |
|------------|----------------|----------|------------------|
| `no_asin` | Directory contains audio files but ExtractASIN returns false | warning | "Add ASIN to folder name: rename to 'Title [ASIN]'" |
| `nested_audio` | Audio files exist in subdirectories of the book folder (not directly in it) | warning | "Flatten: move audio files up to book directory" |
| `multi_book` | Audio file metadata (title/artist) varies significantly across files in one directory, OR file count is very high with distinct naming patterns | warning | "Split: separate into individual book directories" |
| `missing_metadata` | No audio metadata extractable (tag + ffprobe both fail) AND no metadata.json sidecar | info | "Write metadata.json sidecar with known information" |
| `wrong_structure` | Directory does not follow Author/Title [ASIN] convention (e.g., too deep, wrong nesting) | info | "Restructure: move to Author/Title [ASIN] format" |
| `orphan_files` | Non-audio files in book directory (excluding known types: .jpg, .png, .json, .nfo, .cue, .txt) | info | "Review and remove or relocate orphan files" |
| `empty_dir` | Directory contains no files and no subdirectories | warning | "Delete empty directory" |
| `cover_missing` | Audio files present but no cover image (.jpg, .png) in directory and metadata has no embedded artwork | info | "Add cover image to book directory" |

```go
// DetectIssues runs all issue detectors on a directory and returns found issues.
func DetectIssues(dirPath string, dirEntries []os.DirEntry, meta *BookMetadata) []DetectedIssue {
    var issues []DetectedIssue
    issues = append(issues, detectEmptyDir(dirPath, dirEntries)...)
    issues = append(issues, detectNoASIN(dirPath, dirEntries)...)
    issues = append(issues, detectNestedAudio(dirPath, dirEntries)...)
    issues = append(issues, detectOrphanFiles(dirPath, dirEntries)...)
    issues = append(issues, detectCoverMissing(dirPath, dirEntries, meta)...)
    issues = append(issues, detectMissingMetadata(dirPath, meta)...)
    issues = append(issues, detectWrongStructure(dirPath)...)
    issues = append(issues, detectMultiBook(dirPath, dirEntries)...)
    return issues
}
```

### Pattern 4: Deep Scan Flow

```go
// deep.go

// DeepScanResult holds results from a deep library scan.
type DeepScanResult struct {
    TotalDirs    int
    WithASIN     int
    WithoutASIN  int
    IssuesFound  int
    ItemsCreated int
    ItemsUpdated int
}

// DeepScanLibrary walks the entire library, populates library_items,
// and detects issues for every directory.
func DeepScanLibrary(root string, database *sql.DB, metadataFn func(string) (*BookMetadata, error)) (*DeepScanResult, error) {
    runID := generateRunID() // timestamp-based, e.g., "20260406T153000"
    
    // Clear previous scan issues
    db.ClearScanIssues(database, runID) // or delete all, then insert fresh
    
    result := &DeepScanResult{}
    
    err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil { /* handle permission errors gracefully */ }
        if !d.IsDir() { return nil }
        if path == root { return nil }
        
        entries, _ := os.ReadDir(path)
        
        // Classify and create/update library_item
        item := classifyDirectory(path, entries)
        db.UpsertLibraryItem(database, item)
        
        // Extract metadata for issue detection
        meta := extractMetadataForIssueDetection(path, metadataFn)
        
        // Detect issues
        issues := DetectIssues(path, entries, meta)
        for _, issue := range issues {
            db.InsertScanIssue(database, issue, runID)
        }
        
        result.TotalDirs++
        // ... update counts
        
        return nil
    })
    
    return result, err
}
```

### Pattern 5: CLI Integration (scan.go modification)

```go
// Add to existing scan.go
var scanDeep bool

func init() {
    scanCmd.Flags().BoolVar(&scanDeep, "deep", false, "scan all folders including those without ASINs")
    // existing --recursive flag stays
}

func runScan(cmd *cobra.Command, args []string) error {
    // ... existing library path validation ...
    // ... existing database setup ...
    
    if scanDeep {
        return runDeepScan(cmd, database, libPath)
    }
    
    // ... existing ASIN scan code (completely untouched) ...
}

func runDeepScan(cmd *cobra.Command, database *sql.DB, libPath string) error {
    // Deep scan logic -- calls scanner.DeepScanLibrary
    // Display summary with issue counts by type and severity
    // Show top issues with suggested actions
}
```

### Anti-Patterns to Avoid

- **Do NOT modify scanner.go or the existing ScanLibrary function:** The ASIN scan path must remain untouched. Deep scan is additive, in a new file.
- **Do NOT use the books table for non-ASIN content:** That is what library_items (from Phase 9) is for. Books table is ASIN-keyed only.
- **Do NOT embed complex detection logic in the CLI layer:** Keep detection heuristics in scanner package, CLI just calls and displays.
- **Do NOT make issue detection depend on external tools:** Keep detectors based on filesystem state and existing metadata package. ffprobe availability should not cause detection to fail.
- **Do NOT store issues as JSON blobs:** Use a proper table with typed columns for queryability.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Filesystem walking | Custom recursive walker | filepath.WalkDir (stdlib) | Already used in existing recursive scan, handles symlinks and errors correctly |
| Audio file detection | New audio file finder | metadata.FindAudioFiles (existing) | Already handles .m4a and .m4b, case-insensitive |
| ASIN extraction | New ASIN parser | scanner.ExtractASIN (existing) | Already handles brackets, parens, standalone ASINs |
| Metadata extraction | New metadata reader | metadata.ExtractMetadata (existing) | Already implements tag -> ffprobe -> folder fallback chain |
| UUID/run ID | uuid library | Time-based string: time.Now().Format("20060102T150405") | No need for true UUIDs for a local-only CLI tool |

## Common Pitfalls

### Pitfall 1: Breaking Existing Scan
**What goes wrong:** Modifying runScan or ScanLibrary introduces a regression in the existing ASIN scan.
**Why it happens:** The deep scan and ASIN scan share entry points and data structures.
**How to avoid:** Deep scan lives entirely in new files (deep.go, issues.go). The only modification to scan.go is adding the --deep flag check at the top of runScan with an early return to a separate function. Existing scan tests must continue to pass unchanged.
**Warning signs:** Any existing scan_test.go test failing.

### Pitfall 2: Symlink Loops
**What goes wrong:** filepath.WalkDir follows symlinks and enters an infinite loop in libraries with symlinked directories.
**Why it happens:** NAS mounts sometimes use symlinks for organization.
**How to avoid:** filepath.WalkDir by default does NOT follow symlinks for directories (since Go 1.16). The existing recursive scanner already uses WalkDir safely. However, add a visited-paths set as a safety net, keyed on os.FileInfo.Sys() device+inode or just path after filepath.EvalSymlinks.
**Warning signs:** Scan hanging or extremely high directory count.

### Pitfall 3: Performance on Large Libraries
**What goes wrong:** Scanning thousands of directories with metadata extraction takes too long.
**Why it happens:** Each directory requires ReadDir + potential audio file tag reading.
**How to avoid:** Make metadata extraction optional/lazy during deep scan. For issue detection, most heuristics only need directory listing (entries), not full audio metadata. Only extract metadata when needed (missing_metadata and cover_missing checks). Use a progress indicator (existing spinner pattern).
**Warning signs:** Scan taking >60 seconds on a library with ~1000 folders.

### Pitfall 4: Multi-Book Detection False Positives
**What goes wrong:** Legitimate multi-disc or multi-part audiobooks flagged as multi_book.
**Why it happens:** Multiple audio files with different metadata (disc 1 vs disc 2 of same book).
**How to avoid:** Multi-book detection should be conservative. Check for significantly different titles (Levenshtein distance or simple prefix matching), not just different track numbers or disc numbers. If album/title metadata is the same across files, it is one book. Start with a simple heuristic (file naming patterns suggesting separate works) and tune later.
**Warning signs:** Every multi-file audiobook flagged as multi_book.

### Pitfall 5: scan_issues Accumulation
**What goes wrong:** Running `--deep` multiple times creates duplicate issues.
**Why it happens:** No cleanup of previous scan results.
**How to avoid:** Use scan_run_id approach: each deep scan generates a run ID, clears all previous issues, and inserts fresh. Alternatively, DELETE FROM scan_issues before inserting. The run_id approach preserves history if needed, but for v1.1, a simple clear-and-reinsert is simpler and sufficient.
**Warning signs:** Issue count doubling on each scan.

### Pitfall 6: Path Mismatch Between library_items and scan_issues
**What goes wrong:** scan_issues.path doesn't match library_items.path because of normalization differences.
**Why it happens:** Different code paths constructing paths with/without trailing slashes.
**How to avoid:** Use db.NormalizePath (from Phase 9) consistently in both deep scanner and issue persistence. Always normalize before storing.
**Warning signs:** JOIN between library_items and scan_issues returning empty results despite both having data.

## Code Examples

### Existing Scanner Pattern (verified from scanner.go)
```go
// The deep scanner should follow the same pattern: walk dirs, classify, return results
err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
    if err != nil {
        if os.IsPermission(err) {
            // Log and skip, don't fail entire scan
            slog.Warn("permission denied", "path", path)
            return nil
        }
        return err
    }
    if !d.IsDir() { return nil }
    if path == root { return nil }
    // ... process directory ...
    return nil
})
```

### Issue Detection (pure function pattern)
```go
// Each detector is a pure function taking directory info, returning issues
func detectEmptyDir(dirPath string, entries []os.DirEntry) []DetectedIssue {
    if len(entries) == 0 {
        return []DetectedIssue{{
            Path:            dirPath,
            IssueType:       IssueEmptyDir,
            Severity:        SeverityWarning,
            Message:         "Directory is empty",
            SuggestedAction: "Delete empty directory",
        }}
    }
    return nil
}

func detectNestedAudio(dirPath string, entries []os.DirEntry) []DetectedIssue {
    // Check if any subdirectories contain audio files
    hasDirectAudio := false
    for _, e := range entries {
        if !e.IsDir() {
            ext := strings.ToLower(filepath.Ext(e.Name()))
            if ext == ".m4a" || ext == ".m4b" {
                hasDirectAudio = true
                break
            }
        }
    }
    
    for _, e := range entries {
        if e.IsDir() {
            subPath := filepath.Join(dirPath, e.Name())
            subAudio := metadata.FindAudioFiles(subPath)
            if len(subAudio) > 0 {
                return []DetectedIssue{{
                    Path:            dirPath,
                    IssueType:       IssueNestedAudio,
                    Severity:        SeverityWarning,
                    Message:         fmt.Sprintf("Audio files found in subdirectory %s", e.Name()),
                    SuggestedAction: "Flatten: move audio files up to book directory",
                }}
            }
        }
    }
    return nil
}
```

### ScanIssue CRUD (following books.go pattern)
```go
// scan_issues.go

type ScanIssue struct {
    ID              int64
    Path            string
    IssueType       string
    Severity        string
    Message         string
    SuggestedAction string
    ScanRunID       string
    CreatedAt       time.Time
}

func InsertScanIssue(db *sql.DB, issue ScanIssue) error {
    _, err := db.Exec(
        `INSERT INTO scan_issues (path, issue_type, severity, message, suggested_action, scan_run_id)
         VALUES (?, ?, ?, ?, ?, ?)`,
        issue.Path, issue.IssueType, issue.Severity, issue.Message,
        issue.SuggestedAction, issue.ScanRunID,
    )
    if err != nil {
        return fmt.Errorf("insert scan issue: %w", err)
    }
    return nil
}

func ClearScanIssues(db *sql.DB) error {
    _, err := db.Exec(`DELETE FROM scan_issues`)
    if err != nil {
        return fmt.Errorf("clear scan issues: %w", err)
    }
    return nil
}

func ListScanIssues(db *sql.DB) ([]ScanIssue, error) { ... }

func ListScanIssuesByPath(db *sql.DB, path string) ([]ScanIssue, error) { ... }

func ListScanIssuesByType(db *sql.DB, issueType string) ([]ScanIssue, error) { ... }
```

### CLI Display Pattern
```go
// Display issues grouped by severity, matching existing output style
fmt.Fprintf(cmd.OutOrStdout(), "Deep scan complete:\n")
fmt.Fprintf(cmd.OutOrStdout(), "  Directories: %d\n", result.TotalDirs)
fmt.Fprintf(cmd.OutOrStdout(), "  With ASIN:   %d\n", result.WithASIN)
fmt.Fprintf(cmd.OutOrStdout(), "  Without ASIN:%d\n", result.WithoutASIN)
fmt.Fprintf(cmd.OutOrStdout(), "  Issues:      %d\n", result.IssuesFound)

if result.IssuesFound > 0 && !quiet {
    fmt.Fprintf(cmd.ErrOrStderr(), "\nIssues by type:\n")
    for issueType, count := range issueCounts {
        fmt.Fprintf(cmd.ErrOrStderr(), "  %-20s %d\n", issueType, count)
    }
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed -- `go test ./...` |
| Quick run command | `go test ./internal/scanner/ -run TestDeep -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCAN-01 | Deep scan finds all directories | unit | `go test ./internal/scanner/ -run TestDeepScanAllDirs -count=1` | Wave 0 |
| SCAN-01 | Deep scan includes non-ASIN dirs | unit | `go test ./internal/scanner/ -run TestDeepScanNonASIN -count=1` | Wave 0 |
| SCAN-01 | Issue: no_asin detected | unit | `go test ./internal/scanner/ -run TestDetectNoASIN -count=1` | Wave 0 |
| SCAN-01 | Issue: nested_audio detected | unit | `go test ./internal/scanner/ -run TestDetectNestedAudio -count=1` | Wave 0 |
| SCAN-01 | Issue: empty_dir detected | unit | `go test ./internal/scanner/ -run TestDetectEmptyDir -count=1` | Wave 0 |
| SCAN-01 | Issue: orphan_files detected | unit | `go test ./internal/scanner/ -run TestDetectOrphanFiles -count=1` | Wave 0 |
| SCAN-01 | Issue: cover_missing detected | unit | `go test ./internal/scanner/ -run TestDetectCoverMissing -count=1` | Wave 0 |
| SCAN-01 | Issue: missing_metadata detected | unit | `go test ./internal/scanner/ -run TestDetectMissingMetadata -count=1` | Wave 0 |
| SCAN-01 | Issue: wrong_structure detected | unit | `go test ./internal/scanner/ -run TestDetectWrongStructure -count=1` | Wave 0 |
| SCAN-01 | Issue: multi_book detected | unit | `go test ./internal/scanner/ -run TestDetectMultiBook -count=1` | Wave 0 |
| SCAN-01 | --deep flag triggers deep scan | integration | `go test ./internal/cli/ -run TestScanDeep -count=1` | Wave 0 |
| SCAN-03 | Issues persisted in DB | unit | `go test ./internal/db/ -run TestInsertScanIssue -count=1` | Wave 0 |
| SCAN-03 | Issues survive restart (DB read) | unit | `go test ./internal/db/ -run TestListScanIssues -count=1` | Wave 0 |
| SCAN-03 | Issues cleared on re-scan | unit | `go test ./internal/db/ -run TestClearScanIssues -count=1` | Wave 0 |
| ALL | Existing scan unmodified | regression | `go test ./internal/cli/ -run TestScan -count=1` | Existing |
| ALL | Migration 006 applied | unit | `go test ./internal/db/ -run TestMigration006 -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/scanner/ ./internal/db/ -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before /gsd:verify-work

### Wave 0 Gaps
- [ ] `internal/scanner/deep_test.go` -- covers SCAN-01 deep traversal
- [ ] `internal/scanner/issues_test.go` -- covers SCAN-01 issue detection heuristics
- [ ] `internal/db/scan_issues_test.go` -- covers SCAN-03 persistence
- [ ] `internal/cli/scan_test.go` additions -- covers --deep flag integration

*(Test infrastructure exists -- setupTestDB helper, createTestLibrary helpers, testify, in-memory SQLite all already working)*

## Open Questions

1. **Multi-book detection sensitivity**
   - What we know: Need to detect directories containing multiple distinct books
   - What's unclear: How to reliably distinguish multi-book folders from multi-disc single books
   - Recommendation: Start with a simple heuristic based on file naming patterns (e.g., files with completely different base names suggesting different titles). Mark as conservative -- prefer false negatives over false positives. Can be tuned in later phases.

2. **wrong_structure depth threshold**
   - What we know: Libation convention is Author/Title [ASIN] (2 levels from library root)
   - What's unclear: Should 3+ levels deep be flagged? What about flat layout (Title [ASIN] directly in root)?
   - Recommendation: Flag directories that contain audio but are 3+ levels deep from library root. Do not flag flat layout (1 level) since the existing scanner already supports it. The heuristic is: if audio files are found and the directory depth from root is >2, flag as wrong_structure.

3. **Interaction with existing IncrementalSync**
   - What we know: Deep scan populates library_items table. ASIN scan uses books table.
   - What's unclear: Should deep scan also update the books table for ASIN-bearing folders?
   - Recommendation: No. Keep them separate. Deep scan writes to library_items only. The existing ASIN scan writes to books only. They can be linked via ASIN field in library_items. This avoids any risk of the deep scan corrupting the books table state.

## Sources

### Primary (HIGH confidence)
- Existing codebase: `internal/scanner/scanner.go`, `internal/scanner/asin.go` -- verified scan patterns and ASIN extraction
- Existing codebase: `internal/metadata/metadata.go`, `internal/metadata/tag.go`, `internal/metadata/ffprobe.go` -- verified metadata extraction chain
- Existing codebase: `internal/db/books.go` -- verified CRUD patterns for new scan_issues CRUD
- Existing codebase: `internal/cli/scan.go`, `internal/cli/scan_test.go` -- verified CLI patterns and test approach
- Phase 9 research: `.planning/phases/09-plan-infrastructure-db-schema/09-RESEARCH.md` -- library_items table design, NormalizePath

### Secondary (MEDIUM confidence)
- Go filepath.WalkDir documentation -- symlink handling behavior confirmed (does not follow directory symlinks by default)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- zero new dependencies, all patterns established in codebase
- Architecture: HIGH -- direct extension of existing scanner and db packages
- Pitfalls: HIGH -- based on analysis of existing codebase patterns and filesystem edge cases
- Issue detection heuristics: MEDIUM -- multi_book detection needs real-world tuning

**Research date:** 2026-04-06
**Valid until:** 2026-05-06 (stable -- no external dependency changes expected)
