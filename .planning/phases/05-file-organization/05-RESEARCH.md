# Phase 5: File Organization - Research

**Researched:** 2026-04-04
**Domain:** Filesystem operations, path construction, cross-filesystem moves, Audiobookshelf compatibility
**Confidence:** HIGH

## Summary

Phase 5 constructs Libation-compatible `Author/Title [ASIN]/` folder paths from Book metadata and moves staged downloads into the library directory. The core work is: (1) a path construction module that sanitizes names and builds the folder hierarchy, (2) a file mover that handles cross-filesystem boundaries with size verification, and (3) an `earworm organize` CLI command for manual/recovery use.

The existing `internal/download/staging.go` already contains `MoveToLibrary` and `copyAndDelete` functions that handle cross-filesystem moves. However, these move to a flat `libraryDir/ASIN/` structure. Phase 5 needs to move to `libraryDir/Author/Title [ASIN]/` instead. The organizer package should build on the patterns from staging.go but with the correct destination path construction, size verification (D-10), and partial cleanup on failure (D-09).

**Primary recommendation:** Create `internal/organize/` package with path builder (sanitization + construction), file mover (leveraging staging.go patterns), and orchestrator. Wire into `earworm organize` command and expose for Phase 4 pipeline integration.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Require both author and title metadata to organize a book. If either is missing, mark as error and refuse to organize. Library consistency over convenience.
- **D-02:** Multi-author books use first listed author only for folder path. Full author list stays in DB metadata.
- **D-03:** Strip characters illegal on Windows/macOS/Linux (: / \ * ? " < > |) from author and title names. Keep names readable.
- **D-04:** Truncate individual folder name components at 255 characters (standard filesystem limit) to handle NAS/SMB compatibility.
- **D-05:** Cover art named `cover.jpg` inside each book folder. Standard name auto-detected by Audiobookshelf.
- **D-06:** Chapter metadata stored as `chapters.json` sidecar file alongside the M4A. Useful for debugging and tooling.
- **D-07:** M4A audio filename -- Claude's discretion based on Libation/Audiobookshelf compatibility research.
- **D-08:** Try `os.Rename` first (fast for same filesystem). On EXDEV error, fall back to copy+delete. Handles both local and NAS seamlessly.
- **D-09:** On copy failure (network drop, disk full), clean up partial file on destination, keep staging copy intact, mark book as error. User re-runs to retry.
- **D-10:** Verify copy succeeded by comparing source and destination file sizes before deleting staging file. Fast, catches truncated copies.
- **D-11:** Organization happens automatically as part of the download pipeline (per Phase 4 D-12), AND via standalone `earworm organize` command for recovery/manual use.
- **D-12:** `earworm organize` operates on all staged books with 'downloaded' status. No ASIN filtering needed -- just run it.
- **D-13:** If a book's folder already exists at the library path, overwrite existing files. Re-downloads should update.

### Claude's Discretion
- M4A audio filename convention (D-07) -- pick based on Libation/Audiobookshelf compatibility
- Internal package structure for the organizer (likely `internal/organize/`)
- How to report progress during organize operations (consistent with download pipeline patterns)
- Whether `earworm organize` needs `--quiet` and `--json` flags (probably yes, for consistency)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ORG-01 | Downloaded books organized in Libation-compatible Author/Title [ASIN]/ structure | Path construction module with sanitization, Audiobookshelf folder naming research confirms `[ASIN]` pattern is auto-detected |
| ORG-02 | Cover art, chapter metadata, and audio files placed in correct locations | `cover.jpg` (D-05), `chapters.json` (D-06), M4A named `Title.m4a` (research recommendation for D-07) |
| ORG-03 | File moves from staging to library handle cross-filesystem boundaries | Existing `staging.go` patterns for rename+copy fallback, enhanced with size verification (D-10) and partial cleanup (D-09) |
| TEST-09 | Unit tests for file organization logic (path construction, cross-filesystem move, naming conventions) | Table-driven tests for path sanitization, name construction, truncation edge cases |
| TEST-10 | Integration tests for end-to-end file organization (staging to library move, folder structure validation) | Temp directory based integration tests verifying full staging-to-library flow |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| os (stdlib) | Go 1.26 | File operations, directory creation, rename | All filesystem operations use stdlib |
| io (stdlib) | Go 1.26 | Cross-filesystem copy via io.Copy | Used in copy+delete fallback |
| path/filepath (stdlib) | Go 1.26 | Cross-platform path construction | Handles OS-specific separators |
| syscall (stdlib) | Go 1.26 | EXDEV error detection for cross-fs moves | Explicit error check before fallback |
| regexp (stdlib) | Go 1.26 | Character sanitization patterns | Strip illegal filename characters |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| testify/assert | v1.11.1 | Test assertions | All unit and integration tests |
| testify/require | v1.11.1 | Fatal test assertions | Setup steps in tests |
| spf13/cobra | v1.10.2 | CLI command registration | `earworm organize` command |
| spf13/viper | v1.21.0 | Config access (library_path, staging_path) | Reading paths in organize command |

**No new dependencies required.** All functionality uses existing project dependencies and Go stdlib.

## Architecture Patterns

### Recommended Project Structure
```
internal/organize/
    path.go           # Path construction: sanitize names, build Author/Title [ASIN]/ paths
    path_test.go      # Table-driven tests for sanitization, truncation, edge cases
    mover.go          # File move operations: rename, copy+delete fallback, size verification
    mover_test.go     # Tests for move, cross-fs fallback, partial cleanup, size check
    organizer.go      # Orchestrator: query DB for 'downloaded' books, organize each, update status
    organizer_test.go # Tests for orchestration logic with mock DB
internal/cli/
    organize.go       # Cobra command for `earworm organize`
    organize_test.go  # CLI integration test
```

### Pattern 1: Path Construction (inverse of metadata/folder.go parsing)

**What:** Build `Author/Title [ASIN]/` from Book struct fields -- the exact inverse of `extractFromFolderName` in `metadata/folder.go`.

**When to use:** Every time a book needs to be placed in the library.

**Example:**
```go
// internal/organize/path.go

// illegalChars matches characters not allowed on Windows/macOS/Linux filesystems.
// Per D-03: : / \ * ? " < > |
var illegalChars = regexp.MustCompile(`[:/\\*?"<>|]`)

// SanitizeName removes illegal filesystem characters and trims whitespace.
func SanitizeName(name string) string {
    sanitized := illegalChars.ReplaceAllString(name, "")
    sanitized = strings.TrimSpace(sanitized)
    // Truncate to 255 bytes (filesystem limit per D-04)
    if len(sanitized) > 255 {
        sanitized = sanitized[:255]
    }
    return sanitized
}

// FirstAuthor extracts the first author from a possibly multi-author string.
// Per D-02: comma, semicolon, or " & " separated.
func FirstAuthor(authors string) string {
    for _, sep := range []string{",", ";", " & "} {
        if idx := strings.Index(authors, sep); idx >= 0 {
            return strings.TrimSpace(authors[:idx])
        }
    }
    return strings.TrimSpace(authors)
}

// BuildBookPath constructs the library-relative path: Author/Title [ASIN]
func BuildBookPath(author, title, asin string) (string, error) {
    if author == "" || title == "" {
        return "", fmt.Errorf("both author and title required (got author=%q, title=%q)", author, title)
    }
    authorDir := SanitizeName(FirstAuthor(author))
    titleDir := SanitizeName(fmt.Sprintf("%s [%s]", title, asin))
    return filepath.Join(authorDir, titleDir), nil
}
```

### Pattern 2: Cross-Filesystem Move with Verification

**What:** Try rename, detect EXDEV, fall back to copy+verify+delete.

**When to use:** Moving files from local staging to potentially remote NAS library.

**Example:**
```go
// internal/organize/mover.go

// MoveFile moves a file from src to dst.
// Tries os.Rename first. On cross-filesystem error (EXDEV), falls back to copy+verify+delete.
// On copy failure, cleans up partial destination file per D-09.
func MoveFile(src, dst string) error {
    err := os.Rename(src, dst)
    if err == nil {
        return nil
    }
    // Check for cross-filesystem error
    if !errors.Is(err, syscall.EXDEV) {
        return fmt.Errorf("rename %q to %q: %w", src, dst, err)
    }
    return copyVerifyDelete(src, dst)
}

func copyVerifyDelete(src, dst string) error {
    // Copy file
    if err := copyFile(src, dst); err != nil {
        // D-09: Clean up partial destination on failure
        os.Remove(dst)
        return fmt.Errorf("copy %q to %q: %w", src, dst, err)
    }
    // D-10: Verify sizes match before deleting source
    srcInfo, err := os.Stat(src)
    if err != nil {
        return fmt.Errorf("stat source %q: %w", src, err)
    }
    dstInfo, err := os.Stat(dst)
    if err != nil {
        return fmt.Errorf("stat destination %q: %w", dst, err)
    }
    if srcInfo.Size() != dstInfo.Size() {
        os.Remove(dst) // Clean up bad copy
        return fmt.Errorf("size mismatch: src=%d dst=%d", srcInfo.Size(), dstInfo.Size())
    }
    // Safe to delete source
    return os.Remove(src)
}
```

### Pattern 3: Organizer Orchestration

**What:** Query DB for books with 'downloaded' status, organize each, update status.

**When to use:** Both from `earworm organize` command and from download pipeline callback.

**Example:**
```go
// OrganizeBook moves a single book from staging to library.
// Returns the final library path on success.
func OrganizeBook(book db.Book, stagingDir, libraryDir string) (string, error) {
    relPath, err := BuildBookPath(book.Author, book.Title, book.ASIN)
    if err != nil {
        return "", fmt.Errorf("build path for %s: %w", book.ASIN, err)
    }
    
    destDir := filepath.Join(libraryDir, relPath)
    srcDir := filepath.Join(stagingDir, book.ASIN)
    
    if err := os.MkdirAll(destDir, 0755); err != nil {
        return "", fmt.Errorf("create destination %q: %w", destDir, err)
    }
    
    // Move all files from staging ASIN dir to library book dir
    entries, err := os.ReadDir(srcDir)
    if err != nil {
        return "", fmt.Errorf("read staging dir %q: %w", srcDir, err)
    }
    
    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }
        srcFile := filepath.Join(srcDir, entry.Name())
        dstFile := filepath.Join(destDir, entry.Name())
        if err := MoveFile(srcFile, dstFile); err != nil {
            return "", fmt.Errorf("move %q: %w", entry.Name(), err)
        }
    }
    
    // Remove empty staging directory
    os.Remove(srcDir)
    
    return destDir, nil
}
```

### Anti-Patterns to Avoid
- **Building paths with string concatenation:** Always use `filepath.Join` for cross-platform safety.
- **Deleting source before verifying copy:** The size check (D-10) must happen before `os.Remove(src)`.
- **Ignoring empty author/title:** D-01 mandates error, not fallback to "Unknown Author".
- **Moving the entire staging directory with os.Rename:** This fails across filesystems. Move files individually.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Character sanitization | Custom character-by-character filter | regexp.MustCompile with character class | Maintainable, handles all illegal chars in one pass |
| Cross-platform paths | String concatenation with "/" | filepath.Join | Handles Windows backslash, trailing separators |
| File copy | Manual buffer management | io.Copy | Handles large files, uses efficient OS primitives |

**Key insight:** This phase is mostly stdlib filesystem operations. No external libraries needed. The complexity is in edge cases (truncation at rune boundaries, multi-author parsing, EXDEV detection, size verification), not in the technology.

## M4A Filename Convention (D-07 Recommendation)

**Recommendation: Name the M4A file `Title.m4a`** (sanitized title, no ASIN).

**Rationale:**
- Audiobookshelf extracts ASIN from the **folder name** `[B0123456789]` pattern -- confirmed in official docs. The ASIN does not need to be in the filename.
- Audiobookshelf reads title, author, and other metadata from ID3 tags in the audio file itself, not from the filename.
- Libation's default puts the title in the filename. The folder already carries the ASIN.
- Human readability: `Title.m4a` is cleaner than `B0123456789.m4a` when browsing files.
- If the title is very long, truncation to 255 chars (minus `.m4a` = 251 chars for name) handles it.

**Alternative considered:** Keep original audible-cli output filename. Rejected because audible-cli produces unpredictable filenames and the organizing step should normalize.

## `earworm organize` Command Flags

**Recommendation: Include `--quiet` and `--json` flags** for consistency with all other earworm commands (scan, status, download).

- `--quiet`: Suppress progress output, only print summary.
- `--json`: Output structured JSON result for scripting/automation.

This follows the established CLI pattern in `internal/cli/root.go` (`--quiet`) and every subcommand (`--json`).

## Common Pitfalls

### Pitfall 1: Unicode Truncation at Rune Boundary
**What goes wrong:** Truncating a UTF-8 string at byte 255 can split a multi-byte character, producing invalid UTF-8.
**Why it happens:** Go strings are byte slices; naive `s[:255]` cuts mid-rune.
**How to avoid:** Use `utf8.ValidString` check after truncation, or truncate by rune count/valid rune boundary.
**Warning signs:** Garbled folder names on NAS with non-ASCII titles.

```go
func truncateToBytes(s string, maxBytes int) string {
    if len(s) <= maxBytes {
        return s
    }
    // Find the last valid rune boundary at or before maxBytes
    for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
        maxBytes--
    }
    return s[:maxBytes]
}
```

### Pitfall 2: Existing staging.go MoveToLibrary Conflict
**What goes wrong:** The existing `staging.go` `MoveToLibrary` function moves to `libraryDir/ASIN/`. Phase 5 needs `libraryDir/Author/Title [ASIN]/`.
**Why it happens:** Phase 4 implemented a simple flat structure; Phase 5 adds the real folder hierarchy.
**How to avoid:** The new `internal/organize/` package provides the correct path-aware move. The existing `staging.go` `MoveToLibrary` may need to be deprecated or updated to call the organizer. Alternatively, the download pipeline skips `MoveToLibrary` and calls the organizer directly.
**Warning signs:** Books landing in wrong directory structure.

### Pitfall 3: NAS Permissions and SMB Edge Cases
**What goes wrong:** `os.MkdirAll` fails or `os.Rename` fails with unexpected errors on SMB/NFS mounts.
**Why it happens:** Network filesystems have different permission models, case sensitivity, and error semantics.
**How to avoid:** Use defensive error handling. Don't assume EXDEV is the only rename failure -- the current staging.go approach (fall back on ANY rename error) is actually more robust than checking only EXDEV. Consider keeping that approach.
**Warning signs:** Intermittent failures only on NAS paths.

### Pitfall 4: Empty or Whitespace-Only Names After Sanitization
**What goes wrong:** After stripping illegal characters, author or title becomes empty string.
**Why it happens:** Title like `???` becomes empty after sanitization.
**How to avoid:** Check for empty string AFTER sanitization, not just before. Return error per D-01.
**Warning signs:** Empty directory names created in library.

### Pitfall 5: Race Between Download Pipeline and Manual Organize
**What goes wrong:** `earworm organize` runs while download pipeline is also organizing, causing duplicate moves or file conflicts.
**Why it happens:** Both the auto-organize (D-11) and manual command operate on 'downloaded' status books.
**How to avoid:** Use DB status as the coordination mechanism. Set status to 'organizing' (or similar) before starting the move. Check-and-set atomically. If status is already not 'downloaded', skip.
**Warning signs:** File-not-found errors during organize.

### Pitfall 6: Rename Fallback Strategy -- EXDEV-Only vs Any-Error
**What goes wrong:** Checking only for `syscall.EXDEV` misses other rename failures on network filesystems.
**Why it happens:** NFS/SMB mounts can return different error codes for cross-device situations.
**How to avoid:** The existing `staging.go` falls back on ANY rename error. D-08 says check EXDEV specifically. **Recommendation:** Check EXDEV explicitly for the copy+delete path, but also handle other rename errors gracefully (return them rather than silently ignoring).
**Warning signs:** Move failures on certain NAS configurations.

## Code Examples

### Complete Path Construction with All Edge Cases
```go
// Source: Project decisions D-01 through D-04

package organize

import (
    "fmt"
    "path/filepath"
    "regexp"
    "strings"
    "unicode/utf8"
)

var illegalChars = regexp.MustCompile(`[:/\\*?"<>|]`)

// SanitizeName removes filesystem-illegal characters, trims whitespace,
// and truncates to maxBytes (255 by default) on a valid UTF-8 boundary.
func SanitizeName(name string) string {
    s := illegalChars.ReplaceAllString(name, "")
    s = strings.TrimSpace(s)
    return truncateToBytes(s, 255)
}

func truncateToBytes(s string, maxBytes int) string {
    if len(s) <= maxBytes {
        return s
    }
    for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
        maxBytes--
    }
    return s[:maxBytes]
}

// FirstAuthor returns the first author from a multi-author string.
// Splits on comma, semicolon, or " & ".
func FirstAuthor(authors string) string {
    for _, sep := range []string{",", ";", " & "} {
        if idx := strings.Index(authors, sep); idx >= 0 {
            return strings.TrimSpace(authors[:idx])
        }
    }
    return strings.TrimSpace(authors)
}

// BuildBookPath returns the relative library path: Author/Title [ASIN]
func BuildBookPath(author, title, asin string) (string, error) {
    if strings.TrimSpace(author) == "" || strings.TrimSpace(title) == "" {
        return "", fmt.Errorf("both author and title required (author=%q, title=%q)", author, title)
    }
    authorDir := SanitizeName(FirstAuthor(author))
    if authorDir == "" {
        return "", fmt.Errorf("author name empty after sanitization (original=%q)", author)
    }
    titleWithASIN := fmt.Sprintf("%s [%s]", title, asin)
    titleDir := SanitizeName(titleWithASIN)
    if titleDir == "" {
        return "", fmt.Errorf("title empty after sanitization (original=%q)", title)
    }
    return filepath.Join(authorDir, titleDir), nil
}
```

### M4A File Renaming
```go
// RenameM4AFile returns the destination filename for the M4A audio file.
// Uses sanitized title + .m4a extension per D-07 recommendation.
func RenameM4AFile(title string) string {
    name := SanitizeName(title)
    if name == "" {
        name = "audiobook" // ultimate fallback
    }
    // Ensure total filename (including .m4a) fits in 255 bytes
    maxNameLen := 255 - len(".m4a")
    name = truncateToBytes(name, maxNameLen)
    return name + ".m4a"
}
```

### DB Query for Organizable Books
```go
// ListOrganizable returns books with 'downloaded' status.
// Used by `earworm organize` command per D-12.
func ListOrganizable(db *sql.DB) ([]Book, error) {
    rows, err := db.Query(
        `SELECT `+allColumns+` FROM books WHERE status = 'downloaded' ORDER BY updated_at ASC`,
    )
    // ... standard scan pattern from existing ListBooks/ListDownloadable
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Flat ASIN dirs (`staging.go`) | Author/Title [ASIN] hierarchy | Phase 5 | Books visible to Audiobookshelf scanner |
| No size verification on copy | Size comparison before source delete | Phase 5 | Catches truncated NAS copies |
| No auto-organize | Pipeline auto-organize + manual command | Phase 5 | Zero-intervention workflow |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.11.1 |
| Config file | None needed (Go convention) |
| Quick run command | `go test ./internal/organize/... -v` |
| Full suite command | `go test ./... -v` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ORG-01 | Path construction: Author/Title [ASIN]/ | unit | `go test ./internal/organize/... -run TestBuildBookPath -v` | Wave 0 |
| ORG-01 | Character sanitization | unit | `go test ./internal/organize/... -run TestSanitize -v` | Wave 0 |
| ORG-01 | First author extraction | unit | `go test ./internal/organize/... -run TestFirstAuthor -v` | Wave 0 |
| ORG-02 | File placement (cover.jpg, chapters.json, Title.m4a) | integration | `go test ./internal/organize/... -run TestOrganizeBook -v` | Wave 0 |
| ORG-03 | Cross-filesystem move with fallback | unit | `go test ./internal/organize/... -run TestMoveFile -v` | Wave 0 |
| ORG-03 | Size verification after copy | unit | `go test ./internal/organize/... -run TestCopyVerify -v` | Wave 0 |
| ORG-03 | Partial cleanup on failure | unit | `go test ./internal/organize/... -run TestCleanupOnFailure -v` | Wave 0 |
| TEST-09 | Path construction + naming + cross-fs tests | unit | `go test ./internal/organize/... -v` | Wave 0 |
| TEST-10 | End-to-end staging to library | integration | `go test ./internal/organize/... -run TestEndToEnd -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/organize/... -v`
- **Per wave merge:** `go test ./... -v`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/organize/path_test.go` -- covers ORG-01, TEST-09 (path construction, sanitization)
- [ ] `internal/organize/mover_test.go` -- covers ORG-03, TEST-09 (cross-fs move, size verify)
- [ ] `internal/organize/organizer_test.go` -- covers ORG-02, TEST-10 (end-to-end organization)
- [ ] `internal/cli/organize_test.go` -- covers TEST-10 (CLI integration)

## Integration with Existing Code

### Relationship to staging.go
The existing `internal/download/staging.go` has `MoveToLibrary(stagingDir, libraryDir, asin)` which moves to a flat `libraryDir/ASIN/` structure. Phase 5 introduces proper folder hierarchy. Two approaches:

**Recommended approach:** The download pipeline (Phase 4) calls the organizer from `internal/organize/` instead of `staging.MoveToLibrary`. The `staging.go` `MoveToLibrary` function remains for backward compatibility but the pipeline integration point shifts to the organizer.

**DB functions needed:** A `ListOrganizable` function (books with status='downloaded') for the `earworm organize` command. Also potentially an `UpdateOrganizeComplete` function that sets status='organized' and updates local_path to the library path.

### DB Status Transitions
```
downloaded -> organized  (successful organize)
downloaded -> error      (organize failed: missing metadata, copy failure, etc.)
```

The "organized" status already exists in `ValidStatuses` in `books.go`.

## Open Questions

1. **Should staging.go MoveToLibrary be refactored or deprecated?**
   - What we know: It moves to flat ASIN dirs, which is not the final structure
   - What's unclear: Whether Phase 4 pipeline code directly calls it, or if it's only used in tests
   - Recommendation: Keep it but have the pipeline call the organizer instead. Update Phase 4 integration in a later task.

2. **Chapter metadata format for chapters.json**
   - What we know: D-06 says store as sidecar JSON
   - What's unclear: What format audible-cli's `--chapter` flag outputs. It may already produce a chapter file in staging.
   - Recommendation: If audible-cli produces chapter data, rename/move it as-is. If not, create a minimal JSON from available metadata. This is a discovery task during implementation.

## Sources

### Primary (HIGH confidence)
- Audiobookshelf official docs (https://www.audiobookshelf.org/docs) -- directory structure, author/title folder naming, ASIN extraction from `[ASIN]` pattern
- Audiobookshelf book scanner guide (https://www.audiobookshelf.org/guides/book-scanner/) -- metadata extraction hierarchy, cover.jpg priority
- DeepWiki Audiobookshelf metadata (https://deepwiki.com/advplyr/audiobookshelf/8.3-metadata-extraction-and-matching) -- ASIN `[B0123456789]` pattern confirmation
- Existing codebase: `internal/metadata/folder.go`, `internal/download/staging.go`, `internal/db/books.go`

### Secondary (MEDIUM confidence)
- Libation naming templates (https://getlibation.com/docs/features/naming-templates) -- customizable, default not documented but ASIN-in-folder-name is standard
- Go stdlib documentation for os, io, syscall packages -- filesystem operations

## Project Constraints (from CLAUDE.md)

- **Language:** Go with single binary distribution
- **CLI framework:** Cobra with RunE pattern, one file per command in `internal/cli/`
- **Config:** Viper with `library_path` and `staging_path` keys
- **DB:** modernc.org/sqlite, driver name "sqlite", WAL mode
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests
- **Error handling:** Cobra RunE, wrap errors with `fmt.Errorf("context: %w", err)`
- **M4A only** for v1
- **No GPL code:** Only reference Libation's file structure conventions, don't copy code

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All stdlib, no new dependencies
- Architecture: HIGH - Clear inverse of existing folder.go parsing, well-defined decisions from CONTEXT.md
- Pitfalls: HIGH - Filesystem edge cases are well-known, existing staging.go provides working patterns

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (stable domain, filesystem operations don't change)
