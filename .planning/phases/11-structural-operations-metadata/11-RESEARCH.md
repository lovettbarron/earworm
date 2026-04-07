# Phase 11: Structural Operations & Metadata - Research

**Researched:** 2026-04-07
**Domain:** File operations (flatten, SHA-256 verification), Audiobookshelf metadata sidecar generation
**Confidence:** HIGH

## Summary

Phase 11 builds two file operation primitives that Phase 12's plan engine will execute: (1) a directory flattener that moves nested audio files up to the book folder level with SHA-256 hash verification, and (2) a metadata.json sidecar writer that produces Audiobookshelf-compatible JSON without touching audio files.

The existing codebase provides strong foundations. The `internal/organize/` package already has `MoveFile` with cross-filesystem fallback and size verification -- Phase 11 upgrades this to SHA-256 hash verification and adds recursive directory walking for the flatten operation. The `internal/metadata/` package already extracts book metadata from M4A files -- Phase 11 transforms this extracted metadata into Audiobookshelf's JSON sidecar format.

The Audiobookshelf metadata.json schema has been verified from the ABS source code (`server/models/LibraryItem.js` lines 632-655). The format is a flat JSON object with specific field names and types. SHA-256 hashing uses Go's stdlib `crypto/sha256` -- no external libraries needed.

**Primary recommendation:** Create a new `internal/fileops/` package containing `Flatten()` and `WriteMetadataSidecar()` functions. Both return detailed result structs suitable for audit logging. Reuse `organize.MoveFile` for the actual file moves but add SHA-256 pre/post hash comparison.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FOPS-01 | User can flatten nested audio directories, moving files up to the book folder level | Flatten function with recursive walk, SHA-256 verification per file, empty dir cleanup. Uses existing `metadata.FindAudioFiles` pattern extended to walk subdirs. |
| FOPS-02 | User can write Audiobookshelf-compatible metadata.json sidecars without modifying audio files | ABS metadata.json schema fully documented from source code. Write JSON with `encoding/json`, use existing `metadata.ExtractMetadata` + DB fields as data source. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go -- single binary, no CGo
- **File format:** M4A only for v1 (also M4B based on existing code patterns)
- **Audio tag writing explicitly out of scope** -- metadata.json sidecars are the safer approach
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests
- **Error handling:** Cobra RunE pattern, wrap errors with `fmt.Errorf("context: %w", err)`
- **Package layout:** `internal/` for private packages
- **Existing patterns:** `organize.MoveFile` for file moves, `metadata.ExtractMetadata` for reading metadata, `metadata.FindAudioFiles` for audio file discovery
- **DB patterns:** Phase 9 established plan_operations with op_type "flatten" and "write_metadata" already defined in `ValidOpTypes`

## Standard Stack

### Core (already in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| crypto/sha256 (stdlib) | Go 1.26 | SHA-256 file hashing | Stdlib, zero dependencies, well-suited for file integrity verification |
| encoding/json (stdlib) | Go 1.26 | metadata.json serialization | Stdlib JSON encoder with struct tags |
| io (stdlib) | Go 1.26 | Streaming hash computation | `io.TeeReader` for hash-while-copy pattern |
| path/filepath (stdlib) | Go 1.26 | Directory walking | `filepath.WalkDir` for recursive nested file discovery |

### Supporting (already in project)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| internal/organize | existing | MoveFile with cross-FS fallback | Reuse for actual file moves in flatten |
| internal/metadata | existing | ExtractMetadata, FindAudioFiles | Source data for metadata.json generation |
| internal/db | existing | Plan operations, audit logging | Recording operation results |
| testify/assert + require | v1.11.1 | Test assertions | All tests |

### No New Dependencies Required
This phase uses only stdlib packages and existing internal packages. No new `go get` needed.

## Architecture Patterns

### Recommended Package Structure
```
internal/
  fileops/
    flatten.go          # FlattenDir function
    flatten_test.go
    hash.go             # SHA-256 file hashing utility
    hash_test.go
    sidecar.go          # WriteMetadataSidecar function
    sidecar_test.go
```

### Pattern 1: SHA-256 Verified File Move
**What:** Hash source before move, hash destination after move, compare. Only delete source if hashes match.
**When to use:** Every file move in the flatten operation.
**Example:**
```go
// HashFile computes SHA-256 of a file by streaming through the hasher.
func HashFile(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", fmt.Errorf("open for hash: %w", err)
    }
    defer f.Close()

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", fmt.Errorf("hash %s: %w", path, err)
    }
    return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifiedMove hashes src, moves file, hashes dest, compares.
// On mismatch: removes dest, returns error, source is intact.
func VerifiedMove(src, dst string) error {
    srcHash, err := HashFile(src)
    if err != nil {
        return fmt.Errorf("hash source: %w", err)
    }

    // Use organize.MoveFile for the actual move (handles cross-FS)
    if err := organize.MoveFile(src, dst); err != nil {
        return fmt.Errorf("move: %w", err)
    }

    dstHash, err := HashFile(dst)
    if err != nil {
        return fmt.Errorf("hash destination: %w", err)
    }

    if srcHash != dstHash {
        // Source was already deleted by MoveFile -- this is the failure case
        return fmt.Errorf("hash mismatch: src=%s dst=%s", srcHash, dstHash)
    }
    return nil
}
```

**IMPORTANT DESIGN NOTE:** The existing `organize.MoveFile` uses `os.Rename` as fast path, which is atomic -- the file moves without copy. For same-filesystem renames, SHA-256 verification before+after is redundant (bits don't change). For cross-filesystem moves (EXDEV fallback), the current `copyVerifyDelete` does size verification but not hash verification. The upgrade path is:

1. Hash source BEFORE any move
2. Attempt `os.Rename` -- if succeeds, hash destination and compare (should always match for rename)
3. If EXDEV: copy to destination, hash destination, compare with source hash, THEN delete source only on match

This means we need a new `VerifiedMoveFile` that replaces the size-only verification in `copyVerifyDelete` with SHA-256 verification. Rather than modifying the existing `organize.MoveFile` (which other code depends on), create a new function in `fileops/` that implements the full verified flow.

### Pattern 2: Flatten with Recursive Walk
**What:** Walk all subdirectories of a book folder, find audio files at any depth, move them to the book folder root.
**When to use:** FOPS-01 flatten operation.
**Example:**
```go
type FlattenResult struct {
    BookDir    string
    FilesMoved []FileMoveResult
    DirsRemoved []string
    Errors     []error
}

type FileMoveResult struct {
    SourcePath string
    DestPath   string
    SHA256     string
    Success    bool
    Error      string
}

func FlattenDir(bookDir string) (*FlattenResult, error) {
    result := &FlattenResult{BookDir: bookDir}

    // Walk subdirectories only (skip root-level files)
    err := filepath.WalkDir(bookDir, func(path string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        if d.IsDir() { return nil }

        // Skip files already at root level
        if filepath.Dir(path) == bookDir { return nil }

        ext := strings.ToLower(filepath.Ext(path))
        if ext != ".m4a" && ext != ".m4b" { return nil }

        destPath := filepath.Join(bookDir, filepath.Base(path))
        // Handle name collisions
        destPath = uniquePath(destPath)

        moveResult := verifiedMove(path, destPath)
        result.FilesMoved = append(result.FilesMoved, moveResult)
        return nil
    })

    // Clean up empty subdirectories (bottom-up)
    removeEmptyDirs(bookDir)

    return result, err
}
```

### Pattern 3: Audiobookshelf metadata.json Sidecar
**What:** Write a JSON file matching ABS's expected schema.
**When to use:** FOPS-02 metadata sidecar generation.
**Schema (verified from ABS source `server/models/LibraryItem.js` lines 632-655):**
```go
type ABSMetadata struct {
    Tags          []string     `json:"tags"`
    Chapters      []ABSChapter `json:"chapters"`
    Title         string       `json:"title"`
    Subtitle      string       `json:"subtitle"`
    Authors       []string     `json:"authors"`
    Narrators     []string     `json:"narrators"`
    Series        []string     `json:"series"`
    Genres        []string     `json:"genres"`
    PublishedYear  string      `json:"publishedYear"`
    PublishedDate  string      `json:"publishedDate"`
    Publisher      string      `json:"publisher"`
    Description    string      `json:"description"`
    ISBN           string      `json:"isbn"`
    ASIN           string      `json:"asin"`
    Language       string      `json:"language"`
    Explicit       bool        `json:"explicit"`
    Abridged       bool        `json:"abridged"`
}

type ABSChapter struct {
    ID    int     `json:"id"`
    Start float64 `json:"start"`
    End   float64 `json:"end"`
    Title string  `json:"title"`
}
```

**Key observations from ABS source:**
- `authors` is `[]string` (array of name strings)
- `narrators` is `[]string`
- `series` is `[]string` with format `"Series Name #1"` (name + sequence concatenated)
- `chapters` has numeric `start`/`end` (seconds as float) and string `title`
- `publishedYear` is a string, not int
- `tags` and `genres` are `[]string` -- deduplicated, trimmed
- File is written with `JSON.stringify(jsonObject, null, 2)` (pretty-printed, 2-space indent)

### Anti-Patterns to Avoid
- **Modifying audio files:** FOPS-02 explicitly writes a sidecar, never touches M4A/M4B content. This is also an explicit project out-of-scope constraint.
- **Deleting source before verification:** The SHA-256 comparison MUST happen before source deletion. If hash fails, source stays intact.
- **Ignoring name collisions in flatten:** Two subdirectories might contain files with the same name. Must handle with unique suffixing.
- **Hardcoding audio extensions:** Use the existing `metadata.FindAudioFiles` pattern that handles `.m4a` and `.m4b` case-insensitively.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SHA-256 hashing | Custom hash implementation | `crypto/sha256` + `io.Copy` streaming | Stdlib is correct, tested, handles large files via streaming |
| File moves | Custom copy logic | Extend `organize.MoveFile` pattern | Already handles cross-FS, parent dir creation, permissions |
| Audio file discovery | Custom extension matching | `metadata.FindAudioFiles` pattern | Already handles case-insensitive .m4a/.m4b matching |
| JSON serialization | Manual JSON string building | `encoding/json.MarshalIndent` | Handles escaping, unicode, pretty-printing correctly |

**Key insight:** This phase is mostly glue -- composing existing primitives (file moves, metadata extraction, DB operations) with two new capabilities (SHA-256 verification, ABS JSON format). Keep the new code focused on the composition logic.

## Common Pitfalls

### Pitfall 1: os.Rename Deletes Source Atomically
**What goes wrong:** With `os.Rename`, the source is gone after the call -- you can't hash it after the move.
**Why it happens:** Rename is atomic; there's no "moved but source still exists" state.
**How to avoid:** Hash the source BEFORE calling any move function. Store the hash, then verify the destination.
**Warning signs:** Tests that hash source after MoveFile will fail or read from the wrong path.

### Pitfall 2: Name Collisions During Flatten
**What goes wrong:** Two nested subdirectories both contain `Part01.m4a`. Moving both to the root directory overwrites one.
**Why it happens:** `filepath.Base` strips directory info, creating duplicates.
**How to avoid:** Check if destination exists before moving. If collision, append a numeric suffix (e.g., `Part01_2.m4a`).
**Warning signs:** Fewer files at destination than source after flatten.

### Pitfall 3: Empty Directory Cleanup Order
**What goes wrong:** Trying to remove parent directories before children fails because they're not empty.
**Why it happens:** `filepath.WalkDir` visits directories top-down, but removal must be bottom-up.
**How to avoid:** Collect subdirectories during walk, sort by depth (deepest first), then remove empty ones.
**Warning signs:** Leftover empty subdirectories after flatten.

### Pitfall 4: Metadata Fields Mismatch with ABS
**What goes wrong:** ABS ignores or misparses the metadata.json because field names or types don't match.
**Why it happens:** ABS expects `publishedYear` as string, `series` as `["Name #Seq"]` format, `authors` as `[]string` not objects.
**How to avoid:** Use exact field names from ABS source. Write tests that verify JSON output matches expected format. Especially: `publishedYear` is string, not int; `series` includes sequence in the string.
**Warning signs:** ABS scan doesn't pick up metadata from written files.

### Pitfall 5: Large File Hashing Performance
**What goes wrong:** Hashing a 500MB audiobook takes noticeable time, and hashing twice (before + after) doubles it.
**Why it happens:** SHA-256 processes every byte of the file.
**How to avoid:** For same-filesystem renames (`os.Rename` success), skip the post-move hash since the operation is atomic and the bits don't change. Only do full pre+post hashing for cross-filesystem copy operations.
**Warning signs:** Flatten operations taking unexpectedly long on large files.

### Pitfall 6: Partial Flatten State on Error
**What goes wrong:** Some files moved successfully, then one fails, leaving the book in a mixed state.
**Why it happens:** Flatten processes files sequentially; a mid-operation failure leaves some at root, some in subdirs.
**How to avoid:** Continue processing all files (like `OrganizeAll` pattern), collect all errors, report in result. The caller (plan engine) decides whether to retry or roll back.
**Warning signs:** FlattenResult with both successful moves and errors.

## Code Examples

### SHA-256 File Hashing
```go
// Source: Go stdlib crypto/sha256
func HashFile(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", fmt.Errorf("open for hash: %w", err)
    }
    defer f.Close()

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", fmt.Errorf("hash %s: %w", path, err)
    }
    return hex.EncodeToString(h.Sum(nil)), nil
}
```

### Audiobookshelf metadata.json Output
```json
{
  "tags": [],
  "chapters": [
    {
      "id": 0,
      "start": 0,
      "end": 6004.6675,
      "title": "Chapter 1"
    }
  ],
  "title": "The Great Book",
  "subtitle": "",
  "authors": ["Author Name"],
  "narrators": ["Narrator Name"],
  "series": ["Series Name #1"],
  "genres": ["Fiction"],
  "publishedYear": "2023",
  "publishedDate": "",
  "publisher": "",
  "description": "",
  "isbn": "",
  "asin": "B0XXXXXXXX",
  "language": "",
  "explicit": false,
  "abridged": false
}
```

### Writing metadata.json Sidecar
```go
func WriteMetadataSidecar(bookDir string, meta ABSMetadata) error {
    data, err := json.MarshalIndent(meta, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal metadata: %w", err)
    }

    path := filepath.Join(bookDir, "metadata.json")
    if err := os.WriteFile(path, data, 0644); err != nil {
        return fmt.Errorf("write metadata.json: %w", err)
    }
    return nil
}
```

### Building ABSMetadata from Existing Sources
```go
func BuildABSMetadata(bookMeta *metadata.BookMetadata, asin string) ABSMetadata {
    authors := []string{}
    if bookMeta.Author != "" {
        authors = []string{bookMeta.Author}
    }

    narrators := []string{}
    if bookMeta.Narrator != "" {
        narrators = []string{bookMeta.Narrator}
    }

    series := []string{}
    if bookMeta.Series != "" {
        series = []string{bookMeta.Series}
    }

    genres := []string{}
    if bookMeta.Genre != "" {
        genres = []string{bookMeta.Genre}
    }

    publishedYear := ""
    if bookMeta.Year > 0 {
        publishedYear = strconv.Itoa(bookMeta.Year)
    }

    return ABSMetadata{
        Tags:          []string{},
        Chapters:      []ABSChapter{},
        Title:         bookMeta.Title,
        Subtitle:      "",
        Authors:       authors,
        Narrators:     narrators,
        Series:        series,
        Genres:        genres,
        PublishedYear:  publishedYear,
        PublishedDate:  "",
        Publisher:      "",
        Description:    "",
        ISBN:           "",
        ASIN:           asin,
        Language:       "",
        Explicit:       false,
        Abridged:       false,
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ABS used `.abs` sidecar format | JSON is now default for new installs | ~2023-2024 | Use `metadata.json`, not `metadata.abs` |
| Size-only verification (current mover.go) | SHA-256 hash verification | Phase 11 | Stronger integrity guarantee for cross-FS moves |
| ABS metadata had nested `metadata` key | Flat JSON object (no nesting) | ABS v2+ | Use flat structure, but parser handles legacy nested format |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.11.1 |
| Config file | None (Go convention) |
| Quick run command | `go test ./internal/fileops/...` |
| Full suite command | `go test ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FOPS-01a | HashFile produces correct SHA-256 | unit | `go test ./internal/fileops/ -run TestHashFile -x` | Wave 0 |
| FOPS-01b | FlattenDir moves nested audio to root | unit | `go test ./internal/fileops/ -run TestFlattenDir -x` | Wave 0 |
| FOPS-01c | Flatten handles name collisions | unit | `go test ./internal/fileops/ -run TestFlattenNameCollision -x` | Wave 0 |
| FOPS-01d | Flatten leaves source intact on hash mismatch | unit | `go test ./internal/fileops/ -run TestFlattenHashMismatch -x` | Wave 0 |
| FOPS-01e | Flatten cleans empty subdirectories | unit | `go test ./internal/fileops/ -run TestFlattenCleansDirs -x` | Wave 0 |
| FOPS-02a | WriteMetadataSidecar produces valid ABS JSON | unit | `go test ./internal/fileops/ -run TestWriteMetadataSidecar -x` | Wave 0 |
| FOPS-02b | BuildABSMetadata converts from BookMetadata | unit | `go test ./internal/fileops/ -run TestBuildABSMetadata -x` | Wave 0 |
| FOPS-02c | Sidecar does not modify audio files | unit | `go test ./internal/fileops/ -run TestSidecarNoAudioModification -x` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/fileops/...`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/fileops/hash_test.go` -- covers FOPS-01a
- [ ] `internal/fileops/flatten_test.go` -- covers FOPS-01b through FOPS-01e
- [ ] `internal/fileops/sidecar_test.go` -- covers FOPS-02a through FOPS-02c

## Open Questions

1. **Chapter data availability for metadata.json**
   - What we know: `metadata.ExtractMetadata` returns `ChapterCount` (int) but not chapter start/end/title data. ffprobe returns chapter count. The ABS schema expects full chapter objects with start, end, title.
   - What's unclear: Whether we can extract full chapter data from ffprobe (yes, with `-show_chapters`) or if we should leave chapters empty when not available.
   - Recommendation: Write chapters as empty array `[]` when full chapter data is unavailable. Add a note that chapter extraction can be enhanced later via ffprobe `-show_chapters` with full JSON parsing. This keeps the sidecar valid without blocking on chapter extraction complexity.

2. **Should VerifiedMove live in fileops or extend organize?**
   - What we know: `organize.MoveFile` is used by existing `OrganizeBook` code. Changing it affects existing functionality.
   - What's unclear: Whether to modify `organize.MoveFile` to add SHA-256 or create a separate function.
   - Recommendation: Create `fileops.VerifiedMove` as a new function that wraps `organize.MoveFile` semantics but adds SHA-256. Don't modify existing organize package. Phase 12's plan engine can choose which move function to use based on operation type.

## Sources

### Primary (HIGH confidence)
- Audiobookshelf source code `server/models/LibraryItem.js` lines 632-655 -- metadata.json schema verified via GitHub API
- Audiobookshelf source code `server/utils/generators/abmetadataGenerator.js` -- JSON parsing/validation logic
- Go stdlib `crypto/sha256`, `io`, `path/filepath` -- standard patterns
- Existing earworm codebase: `internal/organize/mover.go`, `internal/metadata/metadata.go`, `internal/db/plans.go`

### Secondary (MEDIUM confidence)
- [Audiobookshelf Book Scanner Guide](https://www.audiobookshelf.org/guides/book-scanner/) -- metadata storage behavior
- [Audiobookshelf GitHub Discussion #59](https://github.com/advplyr/audiobookshelf/discussions/59) -- metadata format history
- [ab_mover utility](https://github.com/austinsr1/ab_mover) -- third-party tool confirming metadata.json field usage

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all stdlib, no new dependencies
- Architecture: HIGH - clear patterns from existing codebase, verified ABS schema from source
- Pitfalls: HIGH - based on direct code analysis of existing mover.go and ABS parser

**Research date:** 2026-04-07
**Valid until:** 2026-05-07 (stable domain, ABS schema unlikely to change)
