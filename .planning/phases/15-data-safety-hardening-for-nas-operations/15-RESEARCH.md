# Phase 15: Data Safety Hardening for NAS Operations - Research

**Researched:** 2026-04-11
**Domain:** File I/O safety, NFS/SMB durability, idempotent resume
**Confidence:** HIGH

## Summary

This phase hardens five specific file operation paths to prevent data loss when operating on irreplaceable audiobook files over NAS mounts (NFS, SMB/CIFS). The issues are well-defined, localized bugs -- not architectural rework. Each fix touches one or two functions and has a clear test strategy.

The five fixes are: (1) adding `Sync()` calls before `Close()` in two copy functions, (2) upgrading the cross-filesystem move from size-only to SHA-256 verification, (3) guarding `FlattenDir` against cleanup when errors occurred, (4) adding audit logging to the `--permanent` delete path, and (5) making the plan engine resume detect already-completed moves by checking destination hash.

**Primary recommendation:** Fix each issue as a surgical edit to the existing function, add targeted tests for each, and verify the full test suite stays green.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SAFE-01 | All file write operations call fsync before close | `copyFile` in mover.go and `VerifiedCopy` in copy.go both lack `Sync()` before `Close()`. Add `dstFile.Sync()` after `io.Copy` and before `dstFile.Close()`. |
| SAFE-02 | Cross-filesystem moves use SHA-256 verification before delete | `copyVerifyDelete` in mover.go uses size-only check. Replace with `fileops.HashFile` for both source (before copy) and destination (after copy+sync). |
| SAFE-03 | FlattenDir skips cleanup on errors | `FlattenDir` in flatten.go unconditionally calls `removeEmptyDirs` at line 87. Add `if len(result.Errors) == 0` guard. |
| SAFE-04 | Permanent delete writes audit log entries | `executePermanentDelete` in cli/cleanup.go has no `db.LogAudit` calls. Add before/after audit entries matching the pattern in `CleanupExecutor.Execute`. |
| SAFE-05 | Plan engine resume is idempotent for completed moves | `executeOp` in engine.go fails when source is missing on resume. Check if destination exists with correct hash before attempting move; if so, mark success. |
</phase_requirements>

## Architecture Patterns

### Fix Locations and Current Code

#### SAFE-01: fsync in write paths

**File 1:** `internal/organize/mover.go` -- `copyFile()` (line 71-94)
- Currently does `io.Copy` then `dstFile.Close()` with no `Sync()`.
- Fix: Insert `dstFile.Sync()` after `io.Copy` succeeds, before the final `dstFile.Close()`.
- Also affects `copyVerifyDelete` in `internal/planengine/cleanup.go` (line 60-107) which has its own `copyVerifyDelete` -- same fix needed there.

**File 2:** `internal/fileops/copy.go` -- `VerifiedCopy()` (line 14-62)
- Currently does `io.Copy` then `dstFile.Close()` at line 45 with no `Sync()`.
- Fix: Insert `dstFile.Sync()` after `io.Copy` succeeds, before `dstFile.Close()`.

**Why fsync matters on NAS:** Without fsync, the OS write cache may report success while data is still in a local buffer. If the NAS connection drops or the machine crashes, the destination file can be zero-length or corrupt. On NFS especially, data may appear written (`stat` shows correct size) but not actually be flushed to the server. `Sync()` forces a flush to the remote filesystem before returning.

**Go API:** `*os.File.Sync()` calls `fsync(2)` on the file descriptor. It returns an error if the flush fails.

#### SAFE-02: SHA-256 for cross-filesystem moves

**File:** `internal/organize/mover.go` -- `copyVerifyDelete()` (line 39-67)
- Currently checks `srcInfo.Size() != dstInfo.Size()` (size-only).
- Fix: Hash source before copy, hash destination after copy+sync, compare hashes. Only delete source if hashes match.
- Import `fileops.HashFile` (already exists in `internal/fileops/hash.go`).
- Same fix needed in `internal/planengine/cleanup.go` `copyVerifyDelete()`.

**Why size-only is insufficient:** A corrupted copy (bit flip, truncated NFS write) can produce a file of the same size but different content. SHA-256 catches this. The performance cost is one read pass over source + one read pass over destination, which is acceptable for audiobook files (typically 50-500MB, one-time operation per book).

#### SAFE-03: FlattenDir safety guard

**File:** `internal/fileops/flatten.go` -- `FlattenDir()` (line 33-90)
- Line 87: `result.DirsRemoved = removeEmptyDirs(bookDir)` is called unconditionally.
- Fix: Wrap in `if len(result.Errors) == 0 { ... }`.
- When errors exist, directories may still contain source files that failed to move. Removing those directories would orphan or lose the files.

#### SAFE-04: Permanent delete audit logging

**File:** `internal/cli/cleanup.go` -- `executePermanentDelete()` (line 143-165)
- Currently calls `os.Remove` and `db.UpdateOperationStatus` but never `db.LogAudit`.
- Fix: Add `db.LogAudit` calls for success and failure paths, matching the pattern in `CleanupExecutor.Execute` (cleanup.go lines 151-183).
- Before state: `{"source_path": op.SourcePath, "action": "permanent_delete"}`.
- After state (success): `{"deleted": true}`.
- After state (failure): `{"error": err.Error()}`.

**Note:** The function currently takes `*sql.DB` as parameter, which is sufficient since `db.LogAudit` also takes `*sql.DB`.

#### SAFE-05: Idempotent resume for moves

**File:** `internal/planengine/engine.go` -- `executeOp()` (line 163-245)
- Currently, a "move" op calls `fileops.VerifiedMove(src, dst)`. If source doesn't exist (because a prior run already moved it), this fails.
- Fix: In the "move" case, before calling `VerifiedMove`, check if source exists. If source is missing AND destination exists with a valid hash, treat as already-completed (return success with the destination hash).
- Same logic for "split" case with `VerifiedMove`.
- This is safe because: if source is gone and destination doesn't exist, the operation truly failed (return error). If source is gone but destination exists, verify destination hash is non-empty to confirm it's a valid file.

**Pattern:**
```go
case "move":
    // Idempotent resume: if source is gone but dest exists with valid hash, skip
    if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
        if hash, hashErr := fileops.HashFile(op.DestPath); hashErr == nil && hash != "" {
            result.Success = true
            result.SHA256 = hash
            return result
        }
        // Source missing, dest also missing or invalid -- real failure
        result.Error = fmt.Sprintf("source missing and dest invalid: %s", op.SourcePath)
        return result
    }
    // Normal move path...
```

### Anti-Patterns to Avoid

- **Calling Sync() after Close():** `Close()` may flush but is not guaranteed to call `fsync`. Always `Sync()` first, then `Close()`. If `Sync()` fails, do NOT delete the source.
- **Removing source before verifying destination:** The hash must be computed on the closed, synced destination file. Never delete source optimistically.
- **Sharing `copyVerifyDelete` between packages:** `organize/mover.go` and `planengine/cleanup.go` each have their own `copyVerifyDelete`. Consider whether to deduplicate into `fileops` or fix both independently. Recommendation: fix both independently in this phase (minimal change), and leave dedup as future refactoring.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| File hashing | Custom hash function | `fileops.HashFile` | Already exists, tested, uses SHA-256 |
| Audit logging | Custom log format | `db.LogAudit` | Already exists with entity_type, entity_id, before/after state |
| File sync | Manual syscall | `*os.File.Sync()` | Go stdlib wraps fsync(2) correctly |

## Common Pitfalls

### Pitfall 1: Sync() error ignored
**What goes wrong:** Calling `dstFile.Sync()` but ignoring the error. On NFS, sync can fail (EIO, ESTALE) indicating the data didn't reach the server.
**Why it happens:** Copy-paste coding where error returns are forgotten.
**How to avoid:** Every `Sync()` call must check and propagate the error. If sync fails, do NOT delete the source file.
**Warning signs:** `_ = dstFile.Sync()` in code review.

### Pitfall 2: Double-close panic
**What goes wrong:** Calling `dstFile.Sync()` + `dstFile.Close()` explicitly, but also having `defer dstFile.Close()` -- the second `Close()` on an already-closed file descriptor returns an error (usually harmless but sloppy).
**Why it happens:** Adding Sync()+Close() without removing the defer.
**How to avoid:** Use the pattern: `defer dstFile.Close()` for safety, then do explicit `Sync()` + `Close()`, and return. The deferred close on an already-closed file is a no-op error that can be safely ignored. Or: remove the defer and handle close explicitly.
**Recommendation:** Keep `defer dstFile.Close()` as safety net but do explicit `Sync()` before it. The explicit close return in `copyFile` already handles this correctly.

### Pitfall 3: Hash comparison on NFS with caching
**What goes wrong:** Reading the destination file for hashing immediately after writing, but NFS client cache returns the data from its own buffer rather than reading from the server.
**Why it happens:** NFS close-to-open consistency means the cache is only guaranteed consistent after close+reopen from a different file descriptor.
**How to avoid:** The existing pattern in `VerifiedCopy` already closes the file before hashing (reopens via `HashFile`). Ensure the mover.go fix also closes before hashing. The `Sync()` call additionally ensures data is flushed before close.

### Pitfall 4: Import cycle with fileops.HashFile
**What goes wrong:** `organize/mover.go` importing `fileops` when `fileops/hash.go` already imports `organize`.
**Why it happens:** `fileops.VerifiedMove` calls `organize.MoveFile`. Adding `fileops.HashFile` to `organize/mover.go` would create a cycle.
**How to avoid:** Either inline the SHA-256 hash in mover.go (duplicate ~10 lines) or restructure. Check the import graph first. The hash function is simple enough to inline.

### Pitfall 5: Resume check races with concurrent execution
**What goes wrong:** The idempotent resume check (source missing + dest exists) could false-positive if another process moved the file.
**Why it happens:** Not a real risk here -- earworm is single-process CLI, not a concurrent server.
**How to avoid:** No mitigation needed for single-process CLI.

## Code Examples

### fsync pattern for copyFile
```go
// After io.Copy succeeds:
if err := dstFile.Sync(); err != nil {
    return fmt.Errorf("syncing destination: %w", err)
}
return dstFile.Close()
```

### SHA-256 verification for cross-filesystem move
```go
func copyVerifyDelete(src, dst string) error {
    // Hash source BEFORE copy
    srcHash, err := hashFileSHA256(src) // inline or import
    if err != nil {
        return fmt.Errorf("hash source: %w", err)
    }

    if err := copyFile(src, dst); err != nil {
        os.Remove(dst)
        return fmt.Errorf("copy failed: %w", err)
    }

    // Hash destination AFTER copy+sync
    dstHash, err := hashFileSHA256(dst)
    if err != nil {
        os.Remove(dst)
        return fmt.Errorf("hash destination: %w", err)
    }

    if srcHash != dstHash {
        os.Remove(dst)
        return fmt.Errorf("hash mismatch: src=%s dst=%s", srcHash, dstHash)
    }

    if err := os.Remove(src); err != nil {
        return fmt.Errorf("removing source after verified copy: %w", err)
    }
    return nil
}
```

### Idempotent resume check
```go
case "move":
    if _, statErr := os.Stat(op.SourcePath); os.IsNotExist(statErr) {
        hash, hashErr := fileops.HashFile(op.DestPath)
        if hashErr == nil && hash != "" {
            result.Success = true
            result.SHA256 = hash
            return result
        }
        result.Error = fmt.Sprintf("source missing, dest not valid: %s -> %s", op.SourcePath, op.DestPath)
        return result
    }
    // ... normal VerifiedMove path
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None (Go convention) |
| Quick run command | `go test ./internal/organize/ ./internal/fileops/ ./internal/planengine/ ./internal/cli/ -v -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SAFE-01 | copyFile calls Sync() before Close() | unit | `go test ./internal/organize/ -run TestCopyFile_Fsync -v` | No -- Wave 0 |
| SAFE-01 | VerifiedCopy calls Sync() before Close() | unit | `go test ./internal/fileops/ -run TestVerifiedCopy_Fsync -v` | No -- Wave 0 |
| SAFE-02 | Cross-fs move uses SHA-256 not size-only | unit | `go test ./internal/organize/ -run TestCopyVerifyDelete_SHA256 -v` | No -- Wave 0 |
| SAFE-03 | FlattenDir skips cleanup on errors | unit | `go test ./internal/fileops/ -run TestFlattenDir_SkipsCleanupOnError -v` | No -- Wave 0 |
| SAFE-04 | executePermanentDelete writes audit logs | unit | `go test ./internal/cli/ -run TestCleanup_PermanentDeleteAudit -v` | No -- Wave 0 |
| SAFE-05 | Resume detects already-moved files | unit | `go test ./internal/planengine/ -run TestApplyPlan_ResumeAlreadyMoved -v` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/organize/ ./internal/fileops/ ./internal/planengine/ ./internal/cli/ -v -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/organize/mover_test.go` -- TestCopyFile_Fsync, TestCopyVerifyDelete_SHA256
- [ ] `internal/fileops/flatten_test.go` -- TestFlattenDir_SkipsCleanupOnError
- [ ] `internal/fileops/copy_test.go` -- TestVerifiedCopy_Fsync (file may not exist yet)
- [ ] `internal/cli/cleanup_test.go` -- TestCleanup_PermanentDeleteAudit
- [ ] `internal/planengine/engine_test.go` -- TestApplyPlan_ResumeAlreadyMoved

## Import Dependency Analysis

Critical finding for SAFE-02: `internal/fileops/hash.go` imports `internal/organize` (for `organize.MoveFile`). Therefore `internal/organize/mover.go` CANNOT import `internal/fileops` -- this would create a circular dependency.

**Solution:** Inline a small `hashFileSHA256` helper in `internal/organize/mover.go` (or a local unexported function). The function is ~10 lines:
```go
func hashFileSHA256(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil { return "", err }
    defer f.Close()
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil { return "", err }
    return hex.EncodeToString(h.Sum(nil)), nil
}
```

This is acceptable duplication given the import constraint.

## Project Constraints (from CLAUDE.md)

- **Language:** Go -- all changes in Go
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests
- **Error handling:** Cobra RunE pattern, wrap errors with `fmt.Errorf("context: %w", err)`
- **DB:** modernc.org/sqlite with driver name "sqlite"
- **Conventions:** One file per command in internal/cli/, viper.Reset() between config tests
- **GSD workflow:** Changes go through GSD workflow

## Sources

### Primary (HIGH confidence)
- Direct code inspection of `internal/organize/mover.go`, `internal/fileops/copy.go`, `internal/fileops/flatten.go`, `internal/planengine/cleanup.go`, `internal/planengine/engine.go`, `internal/cli/cleanup.go`
- Go stdlib documentation for `*os.File.Sync()` -- wraps `fsync(2)`
- Existing test files in each package

### Secondary (MEDIUM confidence)
- NFS close-to-open consistency semantics (well-documented in Linux NFS FAQ)
- SHA-256 as standard integrity verification (used throughout project already via `fileops.HashFile`)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all fixes use existing Go stdlib and project code
- Architecture: HIGH -- each fix is localized, import graph verified, patterns follow existing code
- Pitfalls: HIGH -- import cycle confirmed by code inspection, fsync semantics well-understood

**Research date:** 2026-04-11
**Valid until:** 2026-05-11 (stable -- no external dependency changes)
