# Phase 2: Local Library Scanning - Research

**Researched:** 2026-04-03
**Domain:** Filesystem scanning, M4A metadata extraction, CLI output formatting
**Confidence:** HIGH

## Summary

Phase 2 builds on Phase 1's database layer, config system, and Cobra CLI to implement local library scanning. The core work involves: (1) walking a Libation-style directory tree to discover audiobook folders by ASIN, (2) extracting metadata from M4A files using dhowden/tag, (3) persisting discovered books to SQLite, and (4) displaying library status via `earworm scan` and `earworm status` commands with optional `--json` output.

The existing Phase 1 codebase provides a solid foundation: the `books` table with ASIN primary key, CRUD operations (`InsertBook`, `GetBook`, `ListBooks`, `UpdateBookStatus`), config system with `library_path` already defined, and the Cobra command structure. The main gaps to fill are the scanner package, metadata extraction, CLI commands, and schema evolution to store richer metadata (narrator, duration, genre, series, etc.).

**Primary recommendation:** Build a `scanner` package in `internal/scanner/` that handles directory walking and ASIN extraction, a `metadata` package in `internal/metadata/` for M4A tag reading with ffprobe fallback, and register `scan`/`status` commands in `internal/cli/`. Extend the books table via a new migration (002) to hold the additional metadata fields.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Use fuzzy matching for ASIN extraction -- recognize ASINs in square brackets `[B0...]`, parentheses `(B0...)`, and standalone B0-prefixed strings in folder names. Not limited to strict Libation `Author/Title [ASIN]/` pattern only.
- **D-02:** Folders with no recognizable ASIN are skipped with a warning. They are NOT indexed. User sees a summary of skipped folders at the end of the scan.
- **D-03:** Default scan depth is two levels (Author/Title) from the library root. A `--recursive` flag (or config option) enables full recursive tree walking to find ASIN-bearing folders at any depth. Default is two-level.
- **D-04:** Extract comprehensive metadata from M4A files: title, author, narrator, duration, genre, year, series, cover art presence, chapter count -- everything dhowden/tag can provide.
- **D-05:** Metadata fallback chain: dhowden/tag (primary) -> ffprobe if available (secondary) -> folder name parsing (tertiary). Each book's metadata source is tracked so the user knows the quality level.
- **D-06:** `earworm status` uses compact one-line-per-book format: `Author - Title [ASIN] (status_flag)`. Status flags indicate OK, partial metadata, missing files, etc.
- **D-07:** `--json` flag on status/list commands outputs machine-readable JSON (requirement LIB-06).
- **D-08:** Filtering support (e.g., --author, --status) at Claude's discretion based on what feels natural for the CLI design.
- **D-09:** Permission errors and inaccessible directories are skipped with a warning. Scan continues on accessible paths. Summary of skipped paths shown at the end.
- **D-10:** Scan progress shown via spinner with live counter ("Scanning... 47 books found"). Provides feedback on slow NAS mounts.
- **D-11:** Rescanning is incremental -- compare filesystem state against DB, add new books, update changed metadata, mark missing books as 'removed'. Preserves history and status tracking.

### Claude's Discretion
- Whether to include basic filtering flags (--author, --status) on `earworm status`, or keep it simple and let users pipe to grep.

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| LIB-01 | User can scan an existing local audiobook directory and index discovered books by ASIN | Scanner package with filepath.WalkDir, ASIN regex extraction, DB persistence via existing CRUD |
| LIB-02 | User can view the current state of their library (books, download status, metadata) | `earworm status` command using lipgloss for formatted table output |
| LIB-06 | User can get machine-readable JSON output from all list/status commands | `--json` flag on status command, `encoding/json` stdlib marshal |
| CLI-03 | Error messages clearly communicate what went wrong and how to recover | Structured error types with user-facing messages and recovery hints |
| TEST-03 | Unit tests for local library scanner (directory walking, ASIN extraction, metadata parsing) | Table-driven tests with temp directory fixtures, mock M4A files |
| TEST-04 | Integration tests for CLI commands (earworm scan, status, --json output correctness) | Execute commands via Cobra test harness (pattern established in cli_test.go) |
</phase_requirements>

## Standard Stack

### Core (already in go.mod)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| modernc.org/sqlite | v1.48.1 | Library state persistence | Already in project. Pure Go, no CGo. |
| spf13/cobra | v1.10.2 | CLI commands | Already in project. Subcommand model for scan/status. |
| spf13/viper | v1.21.0 | Config (library_path, scan settings) | Already in project. library_path already defined. |
| testify | v1.11.1 | Test assertions | Already in project. |

### New Dependencies Needed
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| dhowden/tag | v0.0.0-20240417053706 | M4A metadata extraction | Reading title, author, album, genre, year, artwork from M4A files |
| charmbracelet/lipgloss/v2 | v2.0.2 | Styled terminal output | Table formatting for `earworm status` display |
| charmbracelet/bubbles/v2 | v2.1.0 | Spinner component | Scan progress indicator ("Scanning... 47 books found") |

### Not Needed Yet
| Library | Why Not |
|---------|---------|
| charmbracelet/bubbletea | Full TUI framework overkill for scan/status commands. Bubbles spinner can work standalone. |

**Installation:**
```bash
cd /Users/albair/src/earworm
go get github.com/dhowden/tag@latest
go get github.com/charmbracelet/lipgloss/v2@v2.0.2
go get github.com/charmbracelet/bubbles/v2@v2.1.0
```

**Note on bubbles/v2 spinner:** The bubbles spinner component requires Bubble Tea's runtime to animate. For a non-TUI CLI, a simpler approach is to use a goroutine-based spinner that writes directly to stderr, or use lipgloss for static styled output and a basic custom spinner. This avoids pulling in the full Bubble Tea dependency. Research recommends evaluating whether a simple custom spinner (10-15 lines) is preferable to the bubbles dependency.

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── cli/           # Cobra commands (existing: root, config, version; new: scan, status)
├── config/        # Viper config (existing)
├── db/            # SQLite layer (existing: books CRUD, migrations)
│   └── migrations/
│       ├── 001_initial.sql
│       └── 002_add_metadata_fields.sql   # NEW
├── metadata/      # M4A tag reading + ffprobe fallback   # NEW
│   ├── metadata.go
│   ├── tag.go        # dhowden/tag wrapper
│   ├── ffprobe.go    # ffprobe subprocess fallback
│   └── folder.go     # folder name parsing fallback
└── scanner/       # Directory walking + ASIN extraction   # NEW
    ├── scanner.go
    ├── asin.go       # ASIN regex extraction
    └── scanner_test.go
```

### Pattern 1: ASIN Extraction with Fuzzy Matching (D-01)
**What:** Regex-based ASIN extraction from folder names supporting multiple formats.
**When to use:** Every discovered directory is tested for ASIN presence.
**Example:**
```go
// ASIN format: starts with B0, followed by 8 alphanumeric characters (10 chars total)
// Matches: [B08C6YJ1LS], (B08C6YJ1LS), B08C6YJ1LS standalone
var asinPattern = regexp.MustCompile(`(?:\[|\()?(?P<asin>B[0-9A-Z]{9})(?:\]|\))?`)

func ExtractASIN(folderName string) (string, bool) {
    matches := asinPattern.FindStringSubmatch(folderName)
    if matches == nil {
        return "", false
    }
    idx := asinPattern.SubexpIndex("asin")
    return matches[idx], true
}
```

**Important ASIN facts:**
- Audible ASINs always start with "B0" followed by 8 alphanumeric chars (total 10 chars)
- Some older ASINs may start with just "B" followed by 9 alphanumeric chars
- The regex should match `B[0-9A-Z]{9}` to cover both patterns
- Libation's default structure: `Author/Title [ASIN]/` but user libraries vary

### Pattern 2: Two-Level vs Recursive Scan (D-03)
**What:** Default scanning walks exactly two directory levels; `--recursive` enables full tree walk.
**When to use:** Default mode for Libation-style Author/Title structure; recursive for non-standard layouts.
**Example:**
```go
func ScanLibrary(root string, recursive bool) ([]DiscoveredBook, []SkippedDir, error) {
    if recursive {
        return scanRecursive(root)
    }
    return scanTwoLevel(root)
}

func scanTwoLevel(root string) ([]DiscoveredBook, []SkippedDir, error) {
    // Level 1: author directories
    authors, err := os.ReadDir(root)
    // Level 2: title directories within each author
    for _, author := range authors {
        titles, err := os.ReadDir(filepath.Join(root, author.Name()))
        for _, title := range titles {
            asin, ok := ExtractASIN(title.Name())
            // ...
        }
    }
}
```

### Pattern 3: Metadata Fallback Chain (D-05)
**What:** Try dhowden/tag first, then ffprobe, then folder name parsing. Track which source provided metadata.
**When to use:** For every discovered book directory containing M4A files.
**Example:**
```go
type MetadataSource string
const (
    SourceTag     MetadataSource = "tag"      // dhowden/tag
    SourceFFprobe MetadataSource = "ffprobe"  // ffprobe subprocess
    SourceFolder  MetadataSource = "folder"   // folder name parsing
)

type BookMetadata struct {
    Title    string
    Author   string
    Narrator string   // from Raw() tags or ffprobe
    Genre    string
    Year     int
    Series   string   // from Raw() tags or ffprobe
    HasCover bool
    Duration int      // seconds, from ffprobe only
    Source   MetadataSource
}

func ExtractMetadata(bookDir string) (*BookMetadata, error) {
    m4aFiles := findM4AFiles(bookDir)
    if len(m4aFiles) == 0 {
        return extractFromFolderName(bookDir), nil
    }
    
    meta, err := extractWithTag(m4aFiles[0])
    if err == nil && meta.Title != "" {
        return meta, nil
    }
    
    meta, err = extractWithFFprobe(m4aFiles[0])
    if err == nil {
        return meta, nil
    }
    
    return extractFromFolderName(bookDir), nil
}
```

### Pattern 4: Incremental Scan (D-11)
**What:** Compare filesystem state against DB, add/update/mark-removed.
**When to use:** Every scan operation (not just first scan).
**Example:**
```go
func IncrementalSync(database *sql.DB, discovered []DiscoveredBook) (*ScanResult, error) {
    existing, _ := db.ListBooks(database)
    existingMap := make(map[string]*db.Book)
    for i := range existing {
        existingMap[existing[i].ASIN] = &existing[i]
    }
    
    var added, updated, removed int
    seenASINs := make(map[string]bool)
    
    for _, disc := range discovered {
        seenASINs[disc.ASIN] = true
        if book, exists := existingMap[disc.ASIN]; exists {
            // Update if metadata changed
            updated++
        } else {
            // Insert new
            added++
        }
    }
    
    // Mark books in DB but not on filesystem as 'removed'
    for asin, book := range existingMap {
        if !seenASINs[asin] && book.Status != "removed" {
            db.UpdateBookStatus(database, asin, "removed")
            removed++
        }
    }
    
    return &ScanResult{Added: added, Updated: updated, Removed: removed}, nil
}
```

### Pattern 5: JSON Output (D-07, LIB-06)
**What:** `--json` flag produces machine-readable JSON instead of styled terminal output.
**When to use:** On `earworm status` and any future list commands.
**Example:**
```go
var jsonOutput bool

var statusCmd = &cobra.Command{
    Use:   "status",
    Short: "Show library contents and status",
    RunE: func(cmd *cobra.Command, args []string) error {
        books, err := db.ListBooks(database)
        if err != nil {
            return fmt.Errorf("failed to read library: %w\n\nTry running 'earworm scan' first", err)
        }
        
        if jsonOutput {
            enc := json.NewEncoder(cmd.OutOrStdout())
            enc.SetIndent("", "  ")
            return enc.Encode(books)
        }
        
        // Styled terminal output
        for _, b := range books {
            fmt.Fprintf(cmd.OutOrStdout(), "%s - %s [%s] (%s)\n",
                b.Author, b.Title, b.ASIN, b.Status)
        }
        return nil
    },
}

func init() {
    statusCmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
}
```

### Anti-Patterns to Avoid
- **Opening M4A files without closing them:** Always defer `f.Close()` immediately after open. NAS mounts can have limited file handle counts.
- **Stat-ing every file during walk:** Use `d.Type()` from `fs.DirEntry` instead of `d.Info()` during directory traversal -- avoids an extra syscall per file on NAS.
- **Blocking on spinner:** Spinner must run in a separate goroutine; scan logic runs on main goroutine and sends updates via channel.
- **Global viper state in tests:** Tests must call `viper.Reset()` in cleanup (pattern already established in cli_test.go).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| M4A metadata parsing | Custom MP4 atom parser | dhowden/tag | MP4 container format is complex; tag handles AAC, ALAC, M4B variants |
| ASIN validation | Custom character-by-character parser | `regexp.MustCompile` | Regex is clearer and covers all bracket/paren variants |
| Styled terminal tables | Manual fmt.Sprintf with padding | lipgloss/v2 table | Handles terminal width, Unicode, color gracefully |
| JSON serialization | Manual string building | encoding/json stdlib | Standard, handles escaping, struct tags |
| Config binding | Manual ENV/flag/file merging | Viper (already in project) | Already handles library_path, just add scan_depth config key |

## Common Pitfalls

### Pitfall 1: NAS Mount Latency
**What goes wrong:** `filepath.WalkDir` on a NAS mount with thousands of books takes minutes with no feedback; user thinks it's hung.
**Why it happens:** Each directory read is a network round trip. Stat calls multiply the problem.
**How to avoid:** (1) Use two-level scan by default (only 2 ReadDir calls per author). (2) Show spinner with live counter. (3) Avoid unnecessary Info() calls -- use DirEntry.Type() instead.
**Warning signs:** Scan takes >30 seconds with no output.

### Pitfall 2: dhowden/tag Missing Fields for Audiobooks
**What goes wrong:** D-04 requests narrator, duration, series, chapter count. dhowden/tag's Metadata interface only exposes: Title, Album, Artist, AlbumArtist, Composer, Genre, Year, Track, Disc, Picture, Lyrics, Comment.
**Why it happens:** dhowden/tag provides standard MP4 tags, not audiobook-specific atoms. Duration is not part of the tag metadata (it's in the audio stream header). Narrator and series are non-standard tags.
**How to avoid:** (1) Use `Raw()` map to check for audiobook-specific keys like `nrt` (narrator) and freeform iTunes atoms `----:com.apple.iTunes:SERIES`. (2) ffprobe fallback is essential for duration and chapter count. (3) Track metadata source per D-05 so users know which fields came from which source.
**Warning signs:** Metadata struct has many empty fields when only dhowden/tag is used.

### Pitfall 3: Schema Migration Breakage
**What goes wrong:** Adding columns to the books table without a proper migration breaks existing databases.
**Why it happens:** Phase 1 created `001_initial.sql` with just asin, title, author, status, local_path. Phase 2 needs narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source.
**How to avoid:** Create `002_add_metadata_fields.sql` using `ALTER TABLE books ADD COLUMN` statements. SQLite's ALTER TABLE only supports ADD COLUMN (no modify, no drop), which is fine here.
**Warning signs:** Test database opens fine but production DB fails on upgrade.

### Pitfall 4: Duplicate ASIN Handling During Rescan
**What goes wrong:** InsertBook returns error on duplicate ASIN (existing behavior). Rescan needs upsert semantics.
**Why it happens:** Phase 1's InsertBook does a plain INSERT which fails on primary key conflict.
**How to avoid:** Add an `UpsertBook` function that uses `INSERT OR REPLACE` or a separate update path. The incremental scan (D-11) should check existence first and route to insert vs update.
**Warning signs:** Second scan of the same library fails with "UNIQUE constraint failed" errors.

### Pitfall 5: Permission Errors Killing the Scan
**What goes wrong:** One permission-denied error aborts the entire directory walk.
**Why it happens:** Default WalkDir behavior propagates errors upward.
**How to avoid:** In the WalkDir callback, check for permission errors and return `nil` (continue) after logging the skipped path. Collect skipped paths for the summary per D-09.
**Warning signs:** Scan reports 0 books found but library directory is not empty.

## Code Examples

### dhowden/tag Metadata Extraction
```go
// Source: https://pkg.go.dev/github.com/dhowden/tag
import "github.com/dhowden/tag"

func extractWithTag(filePath string) (*BookMetadata, error) {
    f, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    m, err := tag.ReadFrom(f)
    if err != nil {
        return nil, err
    }

    meta := &BookMetadata{
        Title:  m.Title(),
        Author: m.AlbumArtist(), // AlbumArtist is typically the book author
        Genre:  m.Genre(),
        Year:   m.Year(),
        Source: SourceTag,
    }
    
    // Check for cover art presence
    if m.Picture() != nil {
        meta.HasCover = true
    }
    
    // Attempt to read audiobook-specific tags from Raw()
    raw := m.Raw()
    if nrt, ok := raw["©nrt"]; ok {  // narrator
        meta.Narrator = fmt.Sprintf("%v", nrt)
    }
    // Artist field may contain narrator for Audible M4A
    if meta.Narrator == "" {
        meta.Narrator = m.Artist() // Audible often puts narrator in artist field
    }
    
    return meta, nil
}
```

### ffprobe Fallback for Duration and Chapters
```go
// Source: ffprobe standard usage
func extractWithFFprobe(filePath string) (*BookMetadata, error) {
    // Check if ffprobe is available
    ffprobePath, err := exec.LookPath("ffprobe")
    if err != nil {
        return nil, fmt.Errorf("ffprobe not found: %w", err)
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, ffprobePath,
        "-v", "quiet",
        "-print_format", "json",
        "-show_format",
        "-show_chapters",
        filePath,
    )
    
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }
    
    var probe struct {
        Format struct {
            Duration string            `json:"duration"`
            Tags     map[string]string `json:"tags"`
        } `json:"format"`
        Chapters []struct {
            ID int `json:"id"`
        } `json:"chapters"`
    }
    
    if err := json.Unmarshal(output, &probe); err != nil {
        return nil, err
    }
    
    meta := &BookMetadata{Source: SourceFFprobe}
    if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
        meta.Duration = int(d)
    }
    meta.ChapterCount = len(probe.Chapters)
    
    // ffprobe tags are case-insensitive
    for k, v := range probe.Format.Tags {
        switch strings.ToLower(k) {
        case "title":
            meta.Title = v
        case "artist":
            meta.Narrator = v  // Audible puts narrator in artist
        case "album_artist":
            meta.Author = v
        case "genre":
            meta.Genre = v
        }
    }
    
    return meta, nil
}
```

### Folder Name Parsing (Tertiary Fallback)
```go
// Parse "Author Name/Book Title [B08C6YJ1LS]" from directory structure
func extractFromFolderName(bookDir string) *BookMetadata {
    meta := &BookMetadata{Source: SourceFolder}
    
    base := filepath.Base(bookDir)        // "Book Title [B08C6YJ1LS]"
    parent := filepath.Base(filepath.Dir(bookDir))  // "Author Name"
    
    // Strip ASIN from title
    title := asinPattern.ReplaceAllString(base, "")
    title = strings.TrimSpace(strings.Trim(title, "[]()- "))
    
    meta.Title = title
    meta.Author = parent
    return meta
}
```

### Database Migration 002
```sql
-- 002_add_metadata_fields.sql
ALTER TABLE books ADD COLUMN narrator TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN genre TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN year INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN series TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN has_cover INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN duration INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN chapter_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN metadata_source TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN file_count INTEGER NOT NULL DEFAULT 0;
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| filepath.Walk | filepath.WalkDir | Go 1.16 (2021) | Better performance -- DirEntry avoids extra Stat calls |
| lipgloss v1 | lipgloss v2 | March 2025 | Deterministic styles, I/O control; v2 import path |
| bubbles v1 | bubbles v2 | Feb 2026 | Updated spinner/progress; requires bubbletea v2 runtime |

**Note on bubbles v2:** The spinner and progress components in bubbles v2 are designed to work within Bubble Tea's v2 runtime. For a non-TUI CLI that just needs a simple spinner, consider a lightweight custom implementation using goroutines and `\r` carriage return to stderr. This avoids pulling in bubbletea v2 as a transitive dependency.

## Open Questions

1. **Bubbles v2 spinner without Bubble Tea runtime**
   - What we know: Bubbles v2 spinner requires bubbletea v2 for its Update/View cycle.
   - What's unclear: Whether spinner can be used standalone outside bubbletea.
   - Recommendation: Implement a simple custom spinner (~15 lines). A goroutine that writes `\rScanning... N books found` to stderr is sufficient and avoids dependency complexity. If richer UI is needed later, bubbletea can be added.

2. **Raw tag keys for narrator in Audible M4A files**
   - What we know: dhowden/tag exposes `Raw()` map. Standard MP4 atom for narrator is not well-defined. Audible sometimes puts narrator in the Artist field.
   - What's unclear: Exact key names in Raw() for Audible-downloaded M4A files.
   - Recommendation: Try both `Raw()["©nrt"]` and fall back to `Artist()`. ffprobe provides more reliable narrator extraction. Flag metadata source per D-05.

3. **UpdateBook vs UpsertBook for incremental scan**
   - What we know: Current `InsertBook` fails on duplicate ASIN. Incremental scan (D-11) needs upsert.
   - What's unclear: Whether to modify InsertBook or add separate UpsertBook.
   - Recommendation: Add `UpsertBook` function using `INSERT ... ON CONFLICT(asin) DO UPDATE` (SQLite supports this). Keep InsertBook for strict insert scenarios.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | All code | yes | 1.26.1 | -- |
| ffprobe | Metadata fallback (D-05) | needs check at runtime | -- | dhowden/tag primary, folder name tertiary |

**Missing dependencies with no fallback:**
- None. ffprobe is optional per D-05.

**Missing dependencies with fallback:**
- ffprobe: If not available, metadata extraction falls back to dhowden/tag + folder name parsing. Duration and chapter count will be unavailable without ffprobe.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed -- `go test ./...` |
| Quick run command | `go test ./internal/scanner/... ./internal/metadata/... -v -count=1` |
| Full suite command | `go test ./... -v -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| LIB-01 | Scan directory, extract ASINs, index to DB | unit + integration | `go test ./internal/scanner/... -v -run TestScan -count=1` | Wave 0 |
| LIB-02 | View library contents and metadata | integration | `go test ./internal/cli/... -v -run TestStatus -count=1` | Wave 0 |
| LIB-06 | JSON output from status command | integration | `go test ./internal/cli/... -v -run TestStatusJSON -count=1` | Wave 0 |
| CLI-03 | Clear error messages with recovery hints | unit + integration | `go test ./internal/cli/... -v -run TestError -count=1` | Wave 0 |
| TEST-03 | Unit tests for scanner logic | unit | `go test ./internal/scanner/... -v -count=1` | Wave 0 |
| TEST-04 | Integration tests for CLI commands | integration | `go test ./internal/cli/... -v -run "TestScan\|TestStatus" -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/scanner/... ./internal/metadata/... ./internal/cli/... -count=1`
- **Per wave merge:** `go test ./... -v -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/scanner/scanner_test.go` -- covers LIB-01, TEST-03 (ASIN extraction, directory walking, incremental sync)
- [ ] `internal/scanner/asin_test.go` -- covers TEST-03 (ASIN regex table-driven tests)
- [ ] `internal/metadata/metadata_test.go` -- covers TEST-03 (tag extraction, fallback chain)
- [ ] `internal/cli/scan_test.go` -- covers TEST-04 (scan command integration)
- [ ] `internal/cli/status_test.go` -- covers LIB-02, LIB-06, TEST-04 (status command, JSON output)

## Project Constraints (from CLAUDE.md)

- **Language:** Go -- single binary distribution
- **File format:** M4A only for v1
- **SQLite driver:** modernc.org/sqlite (pure Go, no CGo) -- already in project
- **CLI framework:** Cobra + Viper -- already in project
- **Audio metadata:** dhowden/tag (read-only, pure Go)
- **Terminal output:** lipgloss v2 + bubbles for styled output
- **Logging:** log/slog stdlib
- **Testing:** stdlib + testify/assert
- **Existing patterns:** Table-driven tests, viper.Reset() in test cleanup, executeCommand helper in cli_test.go, setupTestDB helper in db_test.go, embedded SQL migrations

## Sources

### Primary (HIGH confidence)
- [dhowden/tag pkg.go.dev](https://pkg.go.dev/github.com/dhowden/tag) -- Metadata interface methods, MP4 support confirmed
- [dhowden/tag GitHub](https://github.com/dhowden/tag) -- M4A/M4B/M4P file type support confirmed
- [filepath.WalkDir Go docs](https://pkg.go.dev/path/filepath) -- WalkDir vs Walk performance, symlink behavior
- [lipgloss v2 table package](https://pkg.go.dev/github.com/charmbracelet/lipgloss/table) -- Table rendering API
- [bubbles spinner package](https://pkg.go.dev/github.com/charmbracelet/bubbles/spinner) -- v2 published Feb 2026
- [Libation naming templates](https://getlibation.com/docs/features/naming-templates) -- ASIN is `<id>` tag, folder structure flexible
- Existing codebase: `internal/db/books.go`, `internal/db/db.go`, `internal/cli/root.go`, `internal/config/config.go`

### Secondary (MEDIUM confidence)
- [MP4 metadata tags (Mutagen docs)](https://mutagen.readthedocs.io/en/latest/api/mp4.html) -- MP4 atom key names for audiobook tags
- [Audiobookshelf M4B tag support discussion](https://github.com/advplyr/audiobookshelf/issues/787) -- Audiobook-specific MP4 metadata fields
- [Lipgloss v2 discussion](https://github.com/charmbracelet/lipgloss/discussions/506) -- v2 changes and migration

### Tertiary (LOW confidence)
- Raw tag key names for Audible-specific M4A files (narrator as `©nrt`) -- needs validation with actual Audible files

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries verified in go.mod or via `go list -m`, versions confirmed
- Architecture: HIGH -- patterns follow established Phase 1 conventions, Go stdlib filepath.WalkDir well-documented
- Pitfalls: HIGH -- dhowden/tag limitations verified against official pkg.go.dev docs; NAS latency is well-known
- Metadata extraction: MEDIUM -- audiobook-specific raw tag keys need validation with actual Audible M4A files

**Research date:** 2026-04-03
**Valid until:** 2026-05-03 (stable domain, 30 days)
