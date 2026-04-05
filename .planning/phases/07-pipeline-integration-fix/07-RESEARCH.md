# Phase 7: Fix Download->Organize Pipeline Integration - Research

**Researched:** 2026-04-05
**Domain:** Go pipeline integration, file staging/organization, status state machine
**Confidence:** HIGH

## Summary

The download pipeline and organize module have a **double-move conflict**: after `earworm download` completes, `download.verifyAndMove` calls `MoveToLibrary` which moves files from `staging/ASIN/` to `library/Title [ASIN]/` (flat, no author directory, original filenames). Then `earworm organize` tries to read from `staging/ASIN/` which no longer exists, failing with directory-not-found errors.

The fix is straightforward and well-scoped: remove the `MoveToLibrary` call from `download.verifyAndMove`, keeping files in staging after download verification. The organize module already correctly implements the Libation-compatible `Author/Title [ASIN]/` structure with proper file renaming. The download pipeline should set status to `downloaded` (leaving files in staging), and `earworm organize` becomes the sole staging-to-library move step.

**Primary recommendation:** Remove the library move from `download.verifyAndMove` so it only decrypts and verifies, then let `organize.OrganizeAll` be the single code path for staging-to-library moves.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ORG-01 | Downloaded books organized in Libation-compatible Author/Title [ASIN]/ structure | organize.BuildBookPath already produces correct paths; fix is removing the competing MoveToLibrary call so OrganizeBook actually runs on populated staging dirs |
| ORG-02 | Cover art, chapter metadata, and audio files placed in correct locations | organize.destinationFilename routing already correct (.m4a->Title.m4a, .jpg->cover.jpg, .json->chapters.json, .m4b->Title.m4b); fix enables this code path to execute |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go, single binary distribution
- **Database:** modernc.org/sqlite with driver name "sqlite", WAL mode
- **CLI:** Cobra commands in internal/cli/, one file per command
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests
- **Error handling:** Cobra RunE pattern, wrap errors with fmt.Errorf("context: %w", err)
- **File format:** M4A and M4B (after AAXC decryption) for v1
- **Cross-filesystem:** Copy-then-delete with size verification (already implemented in organize.MoveFile)

## Architecture Patterns

### Current (Broken) Flow

```
earworm download:
  1. download.Pipeline.Run() downloads each book
  2. download.verifyAndMove() decrypts, verifies, then calls MoveToLibrary
  3. MoveToLibrary moves staging/ASIN/* -> library/Title [ASIN]/ (FLAT, original filenames)
  4. DB updated to status='downloaded', local_path=library/Title [ASIN]
  5. Staging dir removed

earworm organize:
  1. organize.OrganizeAll() queries books with status='downloaded'
  2. OrganizeBook tries to read staging/ASIN/ -> FAILS (dir already removed by download)
  3. DB updated to status='error'
```

### Target (Fixed) Flow

```
earworm download:
  1. download.Pipeline.Run() downloads each book
  2. verifyAndMove() decrypts AAXC if needed, verifies audio files
  3. Files REMAIN in staging/ASIN/
  4. DB updated to status='downloaded' (no local_path yet)

earworm organize:
  1. organize.OrganizeAll() queries books with status='downloaded'
  2. OrganizeBook reads staging/ASIN/, moves to library/Author/Title [ASIN]/
  3. Files renamed: *.m4a->Title.m4a, *.m4b->Title.m4b, *.jpg->cover.jpg, *.json->chapters.json
  4. DB updated to status='organized', local_path=library/Author/Title [ASIN]

earworm daemon:
  sync -> download -> organize -> ABS scan (all steps succeed)
```

### Specific Code Changes Required

**1. `internal/download/pipeline.go` - `verifyAndMove` method (lines 401-442)**

Remove the MoveToLibrary call (Step 4, line 437-439). Keep decrypt and verify steps.

Before:
```go
func (p *Pipeline) verifyAndMove(ctx context.Context, asin string, title string, stagingDir string) error {
    // Step 1: Decrypt AAXC to M4B if applicable.
    // Step 2: Glob for audio files
    // Step 3: Verify each audio file
    // Step 4: Move to library  <-- REMOVE THIS
}
```

After: rename to `verifyStaged` (or similar), remove Step 4 entirely.

**2. `internal/download/pipeline.go` - `downloadWithRetry` (lines 345-351)**

After successful verify, update DB to `downloaded` status but do NOT set `local_path` to a library path (files are still in staging). The `local_path` should either be left empty or set to the staging path.

Current code at line 347-351:
```go
folderName := book.ASIN
if book.Title != "" {
    folderName = fmt.Sprintf("%s [%s]", sanitizeFolderName(book.Title), book.ASIN)
}
localPath := filepath.Join(p.config.LibraryDir, folderName)
```

This constructs a library path that doesn't match the Libation-compatible structure anyway. It should be removed or changed to leave `local_path` empty until organize sets it.

**3. `internal/download/staging.go` - `MoveToLibrary` function**

Can be removed entirely (or kept but unexported and unused). It is the competing implementation. The `sanitizeFolderName` helper may also become unused. The `moveFile` and `copyAndDelete` helpers are superseded by `organize.MoveFile` which has better size verification.

**4. `internal/download/pipeline.go` - `cleanOrphans` (line 460)**

The orphan cleanup includes status `"downloaded"` in the keep set. This is correct -- staging dirs for downloaded (but not yet organized) books must NOT be cleaned.

**5. `internal/cli/download.go` - ABS scan trigger (lines 148-163)**

Currently fires after successful downloads. After the fix, downloads leave files in staging, so ABS scan should NOT fire here. It should only fire after organize. The daemon already handles this correctly (sync->download->organize->ABS scan), but the standalone `earworm download` command triggers ABS early.

Options:
- Remove ABS scan from download command (daemon handles it)
- Move ABS scan to organize command
- Keep both (download triggers scan for any pre-existing organized books, organize triggers for newly organized) -- this is least disruptive

**6. Tests that need updating**

- `internal/download/pipeline_test.go`: Tests that verify files end up in library dir after download will need to verify files remain in staging instead.
- `internal/download/staging_test.go`: Tests for `MoveToLibrary` can be removed or kept as dead code tests.
- New integration test: download -> organize handoff with real staging state.

### DB Status State Machine

```
new/synced -> downloading -> downloaded -> organized
                  |              |
                  v              v
                error          error
```

Key statuses:
- `downloaded`: files in staging, verified, ready for organize
- `organized`: files in library, correctly structured
- `error`: something failed (download or organize)

### File Location at Each Stage

| Stage | Audio File | Cover | Chapters | Location |
|-------|-----------|-------|----------|----------|
| Downloading | *.aaxc or *.m4a | cover(500).jpg | chapters.json | staging/ASIN/ |
| Downloaded (after decrypt+verify) | *.m4b or *.m4a | cover(500).jpg | chapters.json | staging/ASIN/ |
| Organized | Title.m4b or Title.m4a | cover.jpg | chapters.json | library/Author/Title [ASIN]/ |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Cross-filesystem file move | Custom copy logic | organize.MoveFile | Already handles EXDEV fallback with size verification |
| Path sanitization | New sanitizer | organize.SanitizeName | Already handles all 9 illegal chars, UTF-8 truncation |
| Libation path construction | Inline string concat | organize.BuildBookPath | Validates author/title, handles multi-author parsing |

## Common Pitfalls

### Pitfall 1: Staging Dir Cleaned Before Organize
**What goes wrong:** `cleanOrphans` runs at start of download pipeline and removes staging dirs that OrganizeAll hasn't processed yet.
**Why it happens:** `cleanOrphans` only keeps dirs for books with status `downloaded`, `organized`, or `downloading`. If a second download run starts before organize, it's fine. But if `cleanOrphans` logic changes, it could delete unorganized staging dirs.
**How to avoid:** Verify `cleanOrphans` preserves `downloaded` status dirs. Add a test for this specific case.
**Warning signs:** Books download successfully but organize reports "staging directory not found."

### Pitfall 2: local_path Set to Wrong Value Before Organize
**What goes wrong:** `UpdateDownloadComplete` sets `local_path` to a library path that doesn't exist yet (files are in staging).
**Why it happens:** Current code constructs `local_path` as `library/Title [ASIN]` during download, which is wrong both because files aren't there yet AND because it's not the Libation-compatible path.
**How to avoid:** Either leave `local_path` empty on `UpdateDownloadComplete` or set it to the staging path. Let `UpdateOrganizeResult` set the final library path.
**Warning signs:** `earworm status` shows a `local_path` that doesn't exist on disk.

### Pitfall 3: Pipeline Test Mocking Hides Real Behavior
**What goes wrong:** Pipeline tests mock `verifyFunc`, `sleepFunc`, and `decryptFunc`, and never exercise the real file movement path.
**Why it happens:** Necessary for unit testing, but means integration between download and organize was never actually tested end-to-end.
**How to avoid:** Add an integration test that runs download (with mocked audible client) followed by organize, verifying the full staging->library flow.
**Warning signs:** All unit tests pass but real usage fails.

### Pitfall 4: ABS Scan Fires on Incomplete Library
**What goes wrong:** `earworm download` triggers ABS scan after downloads, but files are still in staging (not yet organized into library).
**Why it happens:** Current download command triggers ABS scan on success, assuming files are in library.
**How to avoid:** Move ABS scan trigger to after organize step, or to the daemon cycle only.
**Warning signs:** Audiobookshelf doesn't find newly downloaded books despite scan.

### Pitfall 5: Duplicate Files if Fix is Partial
**What goes wrong:** If MoveToLibrary is removed but the old `local_path` logic remains, or if organize runs on books that were already moved by a previous version, files could be duplicated or lost.
**Why it happens:** Mixed state from before/after the fix.
**How to avoid:** Handle the case where staging dir doesn't exist gracefully in OrganizeBook (skip, don't error). Consider that books already in `downloaded` status from old pipeline runs may have had their staging cleared.

## Code Examples

### Minimal Change to verifyAndMove (rename to verifyStaged)

```go
// verifyStaged decrypts AAXC files (if present) and verifies audio files
// in the staging directory. Files remain in staging for the organize step.
func (p *Pipeline) verifyStaged(ctx context.Context, asin string, stagingDir string) error {
    // Step 1: Decrypt AAXC to M4B if applicable.
    if !p.config.Quiet {
        fmt.Fprintf(p.w, "  Decrypting...\n")
    }
    if err := p.decryptFunc(ctx, stagingDir, nil); err != nil {
        return fmt.Errorf("decrypting AAXC for %s: %w", asin, err)
    }

    // Step 2: Glob for audio files in staging (.m4b first, then .m4a).
    var matches []string
    for _, ext := range []string{"*.m4b", "*.m4a"} {
        found, err := filepath.Glob(filepath.Join(stagingDir, ext))
        if err != nil {
            return fmt.Errorf("globbing staging dir for %s: %w", asin, err)
        }
        matches = append(matches, found...)
    }
    if len(matches) == 0 {
        return fmt.Errorf("no audio files (.m4b/.m4a) found in staging for %s", asin)
    }

    // Step 3: Verify each audio file.
    if !p.config.Quiet {
        fmt.Fprintf(p.w, "  Verifying...\n")
    }
    for _, f := range matches {
        if err := p.verifyFunc(f); err != nil {
            return fmt.Errorf("verifying %s: %w", filepath.Base(f), err)
        }
    }

    return nil
}
```

### Updated downloadWithRetry Success Path

```go
// After successful verify (no move):
if err := db.UpdateDownloadComplete(p.db, book.ASIN, ""); err != nil {
    slog.Warn("failed to mark download complete", "asin", book.ASIN, "error", err)
}
return nil
```

### Integration Test Pattern

```go
func TestDownloadOrganizeHandoff(t *testing.T) {
    database := setupTestDB(t)
    stagingDir := t.TempDir()
    libraryDir := t.TempDir()

    // Insert a book as 'downloaded' (simulating post-download state)
    require.NoError(t, db.InsertBook(database, db.Book{
        ASIN: "B000TEST01", Title: "Test Book", Author: "Test Author",
        Status: "downloaded", AudibleStatus: "finished",
    }))

    // Create staging files (as download pipeline would leave them)
    asinDir := filepath.Join(stagingDir, "B000TEST01")
    require.NoError(t, os.MkdirAll(asinDir, 0755))
    require.NoError(t, os.WriteFile(filepath.Join(asinDir, "audio.m4b"), []byte("audio"), 0644))
    require.NoError(t, os.WriteFile(filepath.Join(asinDir, "cover(500).jpg"), []byte("img"), 0644))
    require.NoError(t, os.WriteFile(filepath.Join(asinDir, "chapters.json"), []byte("{}"), 0644))

    // Run organize
    results, err := organize.OrganizeAll(database, stagingDir, libraryDir)
    require.NoError(t, err)
    require.Len(t, results, 1)
    assert.True(t, results[0].Success)

    // Verify Libation-compatible structure
    expectedDir := filepath.Join(libraryDir, "Test Author", "Test Book [B000TEST01]")
    assert.DirExists(t, expectedDir)
    assert.FileExists(t, filepath.Join(expectedDir, "Test Book.m4b"))
    assert.FileExists(t, filepath.Join(expectedDir, "cover.jpg"))
    assert.FileExists(t, filepath.Join(expectedDir, "chapters.json"))

    // Verify DB status
    book, err := db.GetBook(database, "B000TEST01")
    require.NoError(t, err)
    assert.Equal(t, "organized", book.Status)
    assert.Equal(t, expectedDir, book.LocalPath)

    // Verify staging cleaned
    _, err = os.Stat(asinDir)
    assert.True(t, os.IsNotExist(err))
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed (Go convention) |
| Quick run command | `go test ./internal/download/ ./internal/organize/ ./internal/cli/ -run "Organize\|Pipeline\|Handoff" -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ORG-01 | Files organized in Author/Title [ASIN]/ after download | integration | `go test ./internal/organize/ -run TestOrganizeAll_Integration -count=1` | Exists (needs staging-state update) |
| ORG-02 | Cover->cover.jpg, chapters->chapters.json, audio->Title.ext | unit | `go test ./internal/organize/ -run TestDestinationFilename -count=1` | Exists |
| ORG-01+02 | Full download->organize handoff | integration | `go test ./internal/cli/ -run TestDownloadOrganizeHandoff -count=1` | Wave 0 |
| ORG-01 | Daemon cycle completes with books reaching 'organized' | integration | `go test ./internal/cli/ -run TestDaemonOrganize -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/download/ ./internal/organize/ ./internal/cli/ -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before verify

### Wave 0 Gaps
- [ ] `internal/cli/pipeline_integration_test.go` -- download->organize handoff test (covers ORG-01, ORG-02 integration)
- [ ] Update `internal/download/pipeline_test.go` -- verify files remain in staging (not moved to library)

## Open Questions

1. **What to do with `download.MoveToLibrary` and helpers?**
   - What we know: Function becomes unused after fix. `sanitizeFolderName`, `moveFile`, `copyAndDelete` also become unused.
   - What's unclear: Whether to delete them entirely or leave them (dead code).
   - Recommendation: Delete them. The organize package has better implementations. Dead code noted as tech debt in audit.

2. **Should `earworm download` still trigger ABS scan?**
   - What we know: After fix, downloads leave files in staging, so ABS scan after download is premature.
   - What's unclear: Whether some users run `download` without `organize` (unlikely given daemon flow).
   - Recommendation: Remove ABS scan from download command. Keep it in daemon cycle after organize step. Optionally add it to organize command.

3. **Books already in `downloaded` status from old pipeline runs**
   - What we know: Any books previously downloaded have staging dirs already cleared.
   - What's unclear: Whether there are any such books in real usage.
   - Recommendation: OrganizeBook should handle missing staging dir gracefully (report as error with clear message suggesting re-download).

## Sources

### Primary (HIGH confidence)
- Direct code reading of `internal/download/pipeline.go`, `internal/organize/organizer.go`, `internal/cli/download.go`, `internal/cli/organize.go`, `internal/cli/daemon.go`
- `.planning/v1.0-MILESTONE-AUDIT.md` -- gap analysis identifying the double-move conflict
- `internal/db/books.go` -- status state machine (ListDownloadable, ListOrganizable, UpdateDownloadComplete, UpdateOrganizeResult)

### Secondary (MEDIUM confidence)
- Existing test suites (`organize_test.go`, `pipeline_test.go`, `cli/organize_test.go`) -- confirm test patterns and coverage

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new libraries needed, pure code restructuring
- Architecture: HIGH - both implementations (download and organize) are well-understood from source
- Pitfalls: HIGH - root cause identified precisely via milestone audit, confirmed by code reading

**Research date:** 2026-04-05
**Valid until:** Indefinite (internal codebase, no external dependencies changing)
