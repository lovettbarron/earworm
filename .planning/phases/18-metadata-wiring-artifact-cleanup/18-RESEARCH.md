# Phase 18: Metadata Wiring & Artifact Cleanup - Research

**Researched:** 2026-04-12
**Domain:** Plan engine integration, documentation artifact maintenance
**Confidence:** HIGH

## Summary

Phase 18 has two distinct workstreams: (1) a code change to wire `BuildABSMetadata` into the plan engine's `write_metadata` case so metadata sidecars contain real book data instead of empty skeletons, and (2) a documentation cleanup to fix stale checkboxes, frontmatter, and traceability entries identified by the v1.1 milestone audit.

The code change is well-scoped. The plan engine's `executeOp` method at `internal/planengine/engine.go:311-323` currently constructs an empty `ABSMetadata{}` with only empty arrays initialized. The `BuildABSMetadata` function at `internal/fileops/sidecar.go:46` already exists and is tested -- it converts `*metadata.BookMetadata` + ASIN string into a populated `ABSMetadata`. The missing piece is looking up book metadata given only a `SourcePath` (the book directory path from the plan operation).

The documentation changes are purely mechanical: updating checkboxes, frontmatter fields, and traceability table rows in REQUIREMENTS.md, ROADMAP.md, and three SUMMARY files.

**Primary recommendation:** Add a `GetBookByLocalPath` DB function to look up books by `local_path`, use it in the plan engine to fetch real metadata for `write_metadata` operations, with a fallback to `metadata.ExtractMetadata()` for non-ASIN library items. Fix all documentation artifacts as a separate task.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FOPS-02 | User can write Audiobookshelf-compatible metadata.json sidecars without modifying audio files | Core code change: wire BuildABSMetadata into plan engine write_metadata case with real book data lookup |
| SCAN-02 | Library items are tracked in a path-keyed DB table so plans can reference non-Audible content | Documentation fix: update REQUIREMENTS.md checkbox from [ ] to [x] (implementation already complete) |
| FOPS-01 | User can flatten nested audio directories, moving files up to the book folder level | Documentation fix: update REQUIREMENTS.md checkbox from [ ] to [x] (implementation already complete) |
| PLAN-01 | User can create named plans with typed action records and per-action status tracking | Documentation fix: add requirements_completed to SUMMARY 09-02 frontmatter |
| INTG-01 | All plan operations produce a full audit trail with timestamps, before/after state, and success/failure | Documentation fix: add requirements_completed to SUMMARY 09-02 frontmatter |
</phase_requirements>

## Standard Stack

No new libraries needed. This phase uses only existing project packages.

### Core (Existing)
| Package | Location | Purpose | Role in Phase |
|---------|----------|---------|---------------|
| planengine | internal/planengine/ | Plan execution engine | Modify write_metadata case |
| fileops | internal/fileops/ | File operations + sidecar | BuildABSMetadata already exists |
| metadata | internal/metadata/ | Audio metadata extraction | Fallback for non-DB books |
| db | internal/db/ | Database CRUD | Add GetBookByLocalPath |
| scanner | internal/scanner/ | ASIN extraction | ExtractASIN from folder names |

## Architecture Patterns

### Current write_metadata Flow (BROKEN)
```
PlanOperation{OpType: "write_metadata", SourcePath: "/lib/Author/Title [ASIN]"}
  -> executeOp()
  -> fileops.WriteMetadataSidecar(op.SourcePath, fileops.ABSMetadata{...empty...})
  -> writes metadata.json with no title, author, ASIN, etc.
```

### Target write_metadata Flow
```
PlanOperation{OpType: "write_metadata", SourcePath: "/lib/Author/Title [ASIN]"}
  -> executeOp()
  -> resolveBookMetadata(op.SourcePath)  // NEW
     1. Try db.GetBookByLocalPath(sourcePath) -> Book -> convert to metadata.BookMetadata
     2. Fallback: metadata.ExtractMetadata(sourcePath) -> BookMetadata
     3. Extract ASIN from folder name via scanner.ExtractASIN
  -> fileops.BuildABSMetadata(bookMeta, asin)
  -> fileops.WriteMetadataSidecar(op.SourcePath, absMetadata)
  -> writes metadata.json with real title, author, ASIN, etc.
```

### Metadata Resolution Strategy

The plan engine operates on `PlanOperation` structs which only contain `SourcePath` and `DestPath` (string paths). There is no book ID or ASIN field on operations. The engine needs a resolution strategy:

**Option A: DB lookup by local_path (recommended)**
- Add `GetBookByLocalPath(db, path) (*Book, error)` to `internal/db/books.go`
- Query: `SELECT ... FROM books WHERE local_path = ?`
- Convert `db.Book` fields to `metadata.BookMetadata` for `BuildABSMetadata`
- Pros: Uses existing rich metadata from Audible sync (narrators, series, published year)
- Cons: Requires new DB function; only works for books with local_path set

**Option B: metadata.ExtractMetadata from audio files**
- Call `metadata.ExtractMetadata(sourcePath)` to read M4A tags
- Pros: Works for any book directory with audio files
- Cons: Limited to what dhowden/tag can extract (may lack narrator, series)

**Option C: Library items table lookup**
- Call `db.GetLibraryItem(db, sourcePath)` -> has ASIN, title, author
- Then look up full book via `db.GetBook(db, asin)` if ASIN present
- Pros: Path-keyed, always available after deep scan
- Cons: Two-step lookup, library_items may have less metadata

**Recommendation: Layered fallback (A -> C -> B)**
1. Try `GetBookByLocalPath(sourcePath)` -- richest metadata from Audible
2. Try `GetLibraryItem(sourcePath)` -> `GetBook(asin)` -- scan-discovered items
3. Fallback to `metadata.ExtractMetadata(sourcePath)` -- pure file extraction
4. Extract ASIN from folder name via `scanner.ExtractASIN(filepath.Base(sourcePath))`

This ensures write_metadata works for all scenarios: Audible-downloaded books, scanned library items, and untracked directories.

### Key Implementation Detail: Executor Needs DB Access

The `Executor` struct already has a `DB *sql.DB` field (engine.go:17-21), so the DB lookup is straightforward -- no structural changes needed to pass the database connection.

### Converting db.Book to metadata.BookMetadata

```go
func bookToMetadata(book *db.Book) *metadata.BookMetadata {
    return &metadata.BookMetadata{
        Title:        book.Title,
        Author:       book.Author,
        Narrator:     book.Narrator,
        Genre:        book.Genre,
        Year:         book.Year,
        Series:       book.Series,
        HasCover:     book.HasCover,
        Duration:     book.Duration,
        ChapterCount: book.ChapterCount,
        FileCount:    book.FileCount,
    }
}
```

### Anti-Patterns to Avoid
- **Storing metadata in plan operations:** Do not add metadata fields to PlanOperation -- operations should remain path-based. Metadata resolution happens at execution time.
- **Failing on missing metadata:** If no metadata can be resolved, fall back to the current behavior (empty ABSMetadata with initialized arrays) rather than failing the operation. A partial sidecar is better than a failed operation.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ASIN extraction from paths | Custom regex | `scanner.ExtractASIN()` | Already tested, handles B-prefix and ISBN-10 formats |
| Metadata extraction | Custom tag parsing | `metadata.ExtractMetadata()` | Full fallback chain already implemented |
| ABS metadata conversion | Manual field mapping | `fileops.BuildABSMetadata()` | Already handles publishedYear string conversion, nil-safe arrays |

## Common Pitfalls

### Pitfall 1: Import Cycle with scanner Package
**What goes wrong:** Importing `scanner.ExtractASIN` from `planengine` could create an import cycle.
**Why it happens:** Go enforces strict no-circular-imports.
**How to avoid:** Check the import graph. `scanner` imports `db` and `metadata`. `planengine` imports `db` and `fileops`. Adding `scanner` to `planengine` should be safe since `scanner` does not import `planengine`. Verify with `go build ./...` after adding the import. If cycle detected, copy the `asinPattern` regex into planengine (it's 2 lines).
**Warning signs:** Build failure with "import cycle not allowed".

### Pitfall 2: Path Normalization Mismatch
**What goes wrong:** `GetBookByLocalPath` query returns no results because paths differ in trailing slashes, relative vs absolute, or symlink resolution.
**Why it happens:** `books.local_path` may have been set with different path formatting than what the plan engine uses.
**How to avoid:** Apply `filepath.Clean()` and `db.NormalizePath()` before comparison. Consider using LIKE query with trailing % as fallback, or normalize at write time.
**Warning signs:** Tests pass with temp dirs but fail with real NAS paths.

### Pitfall 3: Missing requirements_completed in SUMMARY Frontmatter
**What goes wrong:** Adding `requirements_completed` field to existing SUMMARY YAML frontmatter breaks parsing if not properly formatted.
**Why it happens:** YAML frontmatter has strict indentation rules.
**How to avoid:** Follow exact format of working SUMMARY files (e.g., 13-01-SUMMARY.md) for the frontmatter structure. The field is a YAML list: `requirements_completed: [PLAN-01, INTG-01]`.
**Warning signs:** GSD tools fail to parse modified SUMMARY files.

## Code Examples

### New DB Function: GetBookByLocalPath
```go
// Source: Based on existing GetBook pattern in internal/db/books.go
func GetBookByLocalPath(db *sql.DB, localPath string) (*Book, error) {
    row := db.QueryRow(
        `SELECT `+allColumns+` FROM books WHERE local_path = ?`,
        localPath,
    )
    b, err := scanBook(row)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("get book by local path %s: %w", localPath, err)
    }
    return b, nil
}
```

### Modified executeOp write_metadata Case
```go
// Source: internal/planengine/engine.go, replacing lines 311-323
case "write_metadata":
    bookMeta, asin := e.resolveBookMetadata(op.SourcePath)
    absMeta := fileops.BuildABSMetadata(bookMeta, asin)
    if err := fileops.WriteMetadataSidecar(op.SourcePath, absMeta); err != nil {
        result.Error = err.Error()
        return result
    }
    result.Success = true
```

### Metadata Resolution Helper
```go
func (e *Executor) resolveBookMetadata(bookDir string) (*metadata.BookMetadata, string) {
    // 1. Try DB lookup by local_path
    book, err := db.GetBookByLocalPath(e.DB, bookDir)
    if err == nil && book != nil {
        return bookToMetadata(book), book.ASIN
    }

    // 2. Try library_items -> books lookup
    item, err := db.GetLibraryItem(e.DB, bookDir)
    if err == nil && item != nil && item.ASIN != "" {
        book, err := db.GetBook(e.DB, item.ASIN)
        if err == nil && book != nil {
            return bookToMetadata(book), book.ASIN
        }
    }

    // 3. Fallback to file-based extraction
    meta, err := metadata.ExtractMetadata(bookDir)
    if err == nil && meta != nil {
        asin := ""
        if extracted, ok := scanner.ExtractASIN(filepath.Base(bookDir)); ok {
            asin = extracted
        }
        return meta, asin
    }

    // 4. Empty metadata with ASIN from folder name
    asin := ""
    if extracted, ok := scanner.ExtractASIN(filepath.Base(bookDir)); ok {
        asin = extracted
    }
    return &metadata.BookMetadata{}, asin
}
```

## Documentation Artifacts to Fix

All artifacts identified by the v1.1 milestone audit:

### REQUIREMENTS.md Fixes
| Line | Current | Target |
|------|---------|--------|
| Checkbox for SCAN-02 | `[ ] **SCAN-02**` | Already `[x]` -- VERIFY |
| Checkbox for FOPS-01 | `[ ] **FOPS-01**` | Already `[x]` -- VERIFY |
| Traceability table | Missing SAFE-01..05 | Add 5 rows mapping to Phase 15 |

Note: After re-reading REQUIREMENTS.md, both SCAN-02 and FOPS-01 already show `[x]`. The traceability table already includes SAFE-01..05. These were likely fixed in a prior pass. **Verify at execution time whether any remain stale.**

### ROADMAP.md Progress Table Fixes
| Phase | Current | Target |
|-------|---------|--------|
| Phase 10 | `0/3` Planned | Should reflect actual completion (3/3 Complete) |
| Phase 11 | `1/2` Complete | Should be `2/2` Complete |
| Phase 13 | `1/2` Complete | Should be `2/2` Complete |
| Phase 10 plan checkboxes | `[ ]` for all 3 | `[x]` for all 3 |
| Phase 11 plan checkboxes | `[ ]` for both | `[x]` for both |

### SUMMARY Frontmatter Fixes
| File | Missing Field | Value |
|------|---------------|-------|
| 09-02-SUMMARY.md | `requirements_completed` | `[PLAN-01, INTG-01]` |
| 11-01-SUMMARY.md | `requirements_completed` | `[FOPS-01]` |
| 11-02-SUMMARY.md | `requirements_completed` | `[FOPS-02]` |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.11.1 |
| Config file | go.mod (test deps) |
| Quick run command | `go test ./internal/planengine/ -run TestWriteMetadata -v` |
| Full suite command | `go test ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FOPS-02 | write_metadata uses real book data | unit | `go test ./internal/planengine/ -run TestWriteMetadata -v` | Partial (existing test uses empty metadata) |
| FOPS-02 | GetBookByLocalPath lookup | unit | `go test ./internal/db/ -run TestGetBookByLocalPath -v` | Wave 0 |
| FOPS-02 | resolveBookMetadata fallback chain | unit | `go test ./internal/planengine/ -run TestResolveBookMetadata -v` | Wave 0 |
| SCAN-02 | Checkbox update | manual-only | Visual inspection of REQUIREMENTS.md | N/A |
| FOPS-01 | Checkbox update | manual-only | Visual inspection of REQUIREMENTS.md | N/A |
| PLAN-01 | SUMMARY frontmatter fix | manual-only | Visual inspection of 09-02-SUMMARY.md | N/A |
| INTG-01 | SUMMARY frontmatter fix | manual-only | Visual inspection of 09-02-SUMMARY.md | N/A |

### Sampling Rate
- **Per task commit:** `go test ./internal/planengine/ ./internal/db/ -v`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before verify

### Wave 0 Gaps
- [ ] `internal/db/books_test.go::TestGetBookByLocalPath` -- new DB function test
- [ ] `internal/planengine/engine_test.go::TestWriteMetadata_WithRealBookData` -- integration test with DB book + sidecar verification
- [ ] `internal/planengine/engine_test.go::TestResolveBookMetadata_Fallbacks` -- unit test for resolution chain

## Open Questions

1. **Path normalization for GetBookByLocalPath**
   - What we know: `db.NormalizePath` exists for library_items. Books table uses `local_path` set during organize. Plan operations use paths from scan results or user input.
   - What's unclear: Whether `local_path` values in books table are consistently formatted with what plan operations use.
   - Recommendation: Normalize both the query input and ensure tests cover path variations (trailing slash, clean path).

2. **Should write_metadata fail or degrade on missing metadata?**
   - What we know: Current behavior writes empty skeleton. Phase 11 decision was "publishedYear as string not int for ABS JSON compatibility; empty arrays never nil".
   - What's unclear: Whether a sidecar with only ASIN and no title/author is useful to Audiobookshelf.
   - Recommendation: Degrade gracefully. Write whatever metadata is available. An ASIN-only sidecar is still useful for ABS library matching.

## Sources

### Primary (HIGH confidence)
- `internal/planengine/engine.go` -- current write_metadata implementation (lines 311-323)
- `internal/fileops/sidecar.go` -- BuildABSMetadata and WriteMetadataSidecar (lines 46-87)
- `internal/db/books.go` -- Book struct and existing queries
- `internal/db/library_items.go` -- LibraryItem struct with path-keyed lookup
- `internal/metadata/metadata.go` -- ExtractMetadata fallback chain
- `internal/scanner/asin.go` -- ExtractASIN utility
- `.planning/v1.1-MILESTONE-AUDIT.md` -- gap identification and fix requirements

### Secondary (MEDIUM confidence)
- `.planning/ROADMAP.md` -- stale checkbox/progress identification
- SUMMARY frontmatter format derived from working examples (13-01, 14-02)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all packages already exist in project
- Architecture: HIGH - clear integration point, existing patterns to follow
- Pitfalls: HIGH - well-understood Go package patterns, simple DB query

**Research date:** 2026-04-12
**Valid until:** 2026-05-12 (stable -- internal project changes only)
