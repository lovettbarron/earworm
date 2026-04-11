# Phase 14: Multi-Book Split & Claude Skill - Research

**Researched:** 2026-04-11
**Domain:** File operations (multi-book splitting) + Claude Code skill authoring
**Confidence:** HIGH

## Summary

This phase delivers two distinct capabilities: (1) splitting multi-book folders into individual book directories, and (2) a Claude Code skill for conversational plan creation. Both build heavily on existing infrastructure.

The multi-book split is well-supported by the codebase. The scanner already detects `IssueMultiBook` via filename heuristics, the plan system already recognizes "split" as a valid operation type, `fileops.VerifiedMove` provides SHA-256-verified file moves, and `organize.BuildBookPath` generates Libation-compatible output paths. The primary new work is: (a) an enhanced grouping algorithm that uses actual audio metadata (title/author/narrator) not just filenames, (b) a `SplitPlanner` that converts detected groups into plan operations, and (c) adding "split" handling to the plan engine's `executeOp` dispatcher.

The Claude Code skill is a `.claude/skills/earworm/SKILL.md` file with YAML frontmatter. It instructs Claude to orchestrate read-only earworm CLI commands (`scan --deep`, `plan list`, `plan review`, `status`) and plan creation commands, with an explicit deny-list preventing execution of `plan apply`, `cleanup`, `download`, and `organize`. The skill format is well-documented and straightforward.

**Primary recommendation:** Implement split as plan operations using existing fileops primitives, and implement the Claude skill as a single SKILL.md with `disable-model-invocation: true` for safety-critical commands.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Hybrid detection -- use metadata (title/author/narrator via dhowden/tag + ffprobe) combined with filename pattern analysis to propose file groupings
- **D-02:** Always require user confirmation of the proposed grouping before creating a split plan. Present via existing `earworm plan show` dry-run view
- **D-03:** When detection confidence is low (sparse/ambiguous metadata), skip the folder and flag it as "needs manual review" in scan results. User can create a manual plan via CSV import
- **D-04:** Split directories use Libation naming convention (Author/Title [ASIN]/ or Author/Title/ if no ASIN)
- **D-05:** Shared files (covers, metadata) are copied to ALL split directories so each book is self-contained
- **D-06:** Original parent directory is NOT auto-removed after split -- left for the `earworm cleanup` command. Conservative approach
- **D-07:** Skill can orchestrate: `scan --deep`, `plan create`, `plan show` (dry-run), `status`/`list`
- **D-08:** Skill format is SKILL.md in `.claude/skills/` -- standard Claude Code skill, auto-discovered
- **D-09:** Explicit deny-list guardrails in SKILL.md: NEVER run `plan apply`, `cleanup`, `download`, `organize`
- **D-10:** Both slash command and natural language triggers -- slash command for direct actions, natural language for exploratory conversations
- **D-11:** When creating a plan, skill runs `plan show` (JSON mode), formats output conversationally, and asks user for approval before saving. User can request adjustments

### Claude's Discretion
- Internal implementation of the split detection algorithm (grouping heuristics, confidence thresholds)
- SKILL.md trigger pattern specifics
- JSON output parsing and conversational formatting in the skill

### Deferred Ideas (OUT OF SCOPE)
None

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FOPS-04 | User can split multi-book folders into separate directories with content-based detection | Hybrid metadata+filename grouping algorithm, SplitPlanner creating plan operations, split execution via VerifiedMove with SHA-256, audit trail via existing LogAudit |
| INTG-02 | Claude Code skill enables conversational plan creation (not execution) via Claude Code | SKILL.md with frontmatter, allowed-tools for Bash(earworm *), deny-list in instructions, $ARGUMENTS for flexible invocation |

</phase_requirements>

## Architecture Patterns

### Recommended Project Structure

New files for this phase:

```
internal/
├── split/                    # NEW package
│   ├── grouper.go            # File grouping algorithm (metadata + filename)
│   ├── grouper_test.go
│   ├── planner.go            # Converts groups into plan operations
│   └── planner_test.go
├── planengine/
│   └── engine.go             # ADD "split" case to executeOp (move files + copy shared)
├── cli/
│   └── split.go              # NEW: `earworm split` command (detect, preview, create plan)
│   └── split_test.go
.claude/
└── skills/
    └── earworm/
        └── SKILL.md           # Claude Code skill
```

### Pattern 1: Split Grouping Algorithm

**What:** Hybrid metadata + filename analysis to group audio files into distinct books.
**When to use:** When `detectMultiBook` has already flagged a directory.

The existing `detectMultiBook` in `internal/scanner/issues.go` uses only filename patterns. The new grouper enhances this with actual audio metadata:

```go
// internal/split/grouper.go
package split

import "github.com/lovettbarron/earworm/internal/metadata"

// BookGroup represents a cluster of audio files belonging to the same book.
type BookGroup struct {
    Title      string
    Author     string
    Narrator   string
    ASIN       string            // if extractable from metadata
    AudioFiles []string          // absolute paths to M4A/M4B files
    Confidence float64           // 0.0-1.0 confidence in grouping
}

// GroupResult holds the output of analyzing a multi-book directory.
type GroupResult struct {
    SourceDir   string
    Groups      []BookGroup
    SharedFiles []string  // covers, metadata.json, etc. -- copied to all groups
    Skipped     bool      // true if confidence too low
    SkipReason  string
}

// GroupFiles analyzes audio files in a directory and returns proposed groupings.
// Uses metadata.ExtractMetadata on each file individually (not just the first),
// then clusters by (title, author) tuple.
func GroupFiles(dirPath string) (*GroupResult, error) {
    // 1. Find all audio files
    // 2. Extract metadata from EACH file (not just first)
    // 3. Group by (title, author) -- primary key
    // 4. Fall back to filename pattern grouping if metadata is sparse
    // 5. Identify shared files (non-audio: covers, JSON, etc.)
    // 6. Calculate confidence per group
    // 7. If any group has confidence < threshold, mark as skipped
}
```

Key insight: The existing `metadata.ExtractMetadata` only reads the FIRST audio file in a directory. For split detection, we need to read metadata from EVERY audio file individually. This is the core difference -- per-file metadata extraction, not per-directory.

### Pattern 2: Split Plan Generation

**What:** Convert BookGroups into plan operations.
**When to use:** After grouping is confirmed by user.

```go
// internal/split/planner.go
package split

import (
    "database/sql"
    "github.com/lovettbarron/earworm/internal/db"
    "github.com/lovettbarron/earworm/internal/organize"
)

// CreateSplitPlan generates a plan with "split" operations for each file move.
func CreateSplitPlan(database *sql.DB, result *GroupResult, libraryRoot string) (int64, error) {
    // 1. Create plan with name "split: <source_dir_name>"
    // 2. For each BookGroup:
    //    a. BuildBookPath(group.Author, group.Title, group.ASIN) for dest dir
    //    b. For each audio file: AddOperation with op_type="split", source=file, dest=new_path
    //    c. For each shared file: AddOperation with op_type="split", source=file, dest=copy_to_new_path
    // 3. Return plan ID for review
}
```

Split operations are "move" operations semantically but use the "split" op_type for audit trail clarity. The plan engine's `executeOp` must handle "split" by delegating to `fileops.VerifiedMove` for audio files and a copy operation for shared files.

### Pattern 3: Split Execution in Plan Engine

**What:** Add "split" case to `planengine.Executor.executeOp`.
**When to use:** When plan apply processes split operations.

```go
// In internal/planengine/engine.go executeOp switch:
case "split":
    // Check if source is a shared file (cover, metadata.json)
    // If shared: copy (not move) with SHA-256 verification
    // If audio: move with SHA-256 verification (same as "move" case)
    
    if isSharedFile(op.SourcePath) {
        // Copy + hash verify
        if err := fileops.VerifiedCopy(op.SourcePath, op.DestPath); err != nil {
            result.Error = err.Error()
            return result
        }
    } else {
        // Move + hash verify (reuse existing VerifiedMove)
        if err := fileops.VerifiedMove(op.SourcePath, op.DestPath); err != nil {
            result.Error = err.Error()
            return result
        }
    }
    hash, err := fileops.HashFile(op.DestPath)
    if err != nil {
        result.Error = fmt.Sprintf("hash after split: %v", err)
        return result
    }
    result.Success = true
    result.SHA256 = hash
```

Note: `fileops.VerifiedCopy` does not exist yet -- it needs to be created. It is similar to `VerifiedMove` but copies instead of moves, since shared files (covers) must remain in the source directory for other groups.

### Pattern 4: Claude Code Skill

**What:** SKILL.md for conversational plan creation.
**When to use:** User invokes `/earworm` or asks about their audiobook library in Claude Code.

```yaml
---
name: earworm
description: Manage your Audible audiobook library. Scan for issues, create cleanup plans, and review library status. Use when discussing audiobooks, library organization, or earworm commands.
allowed-tools: Bash(earworm *)
---

You are an audiobook library assistant using the earworm CLI tool.

## What you CAN do
- Run `earworm scan --deep` to detect library issues
- Run `earworm plan list --json` to see existing plans
- Run `earworm plan review <id> --json` to preview a plan
- Run `earworm split detect <path>` to analyze multi-book folders
- Run `earworm split plan <path>` to create a split plan
- Run `earworm status` to check library status

## What you MUST NEVER do
- NEVER run `earworm plan apply` -- humans must explicitly apply plans
- NEVER run `earworm cleanup` -- destructive operation requiring human confirmation
- NEVER run `earworm download` -- interacts with external services
- NEVER run `earworm organize` -- moves files without plan review

## Workflow
1. When asked about library issues, run `earworm scan --deep` first
2. Present scan results conversationally, highlighting actionable items
3. When creating plans, always show the dry-run output and ask for confirmation
4. Use `--json` flag for machine-parseable output, format conversationally for the user
5. If the user wants to apply a plan, tell them the exact command to run themselves

$ARGUMENTS
```

### Anti-Patterns to Avoid
- **Per-file metadata extraction in the scanner itself:** The scanner should stay lightweight (filename-only). Heavy per-file metadata extraction belongs in the split package, triggered only for confirmed multi-book directories.
- **Auto-removing source directories after split:** D-06 explicitly prohibits this. Use the existing cleanup command path.
- **Skill executing plans:** D-09 explicitly prohibits this. The skill must NEVER run `plan apply`.
- **Monolithic grouping:** Do not try to handle all edge cases in one function. Separate metadata grouping from filename-pattern grouping and combine results.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SHA-256 file verification | Custom hash logic | `fileops.HashFile` + `fileops.VerifiedMove` | Already battle-tested in plan engine |
| Libation-compatible paths | Custom path builder | `organize.BuildBookPath` | Handles sanitization, ASIN formatting, author splitting |
| Audio file discovery | Custom file walker | `metadata.FindAudioFiles` | Handles case-insensitive extensions, sorting |
| Plan CRUD | Custom DB operations | `db.CreatePlan`, `db.AddOperation` | Consistent with existing plan infrastructure |
| Audit logging | Custom logging | `db.LogAudit` | Consistent audit trail format |
| M4A metadata reading | Custom parser | `metadata.ExtractMetadata` fallback chain | tag -> ffprobe -> folder already handles edge cases |

**Key insight:** Nearly every infrastructure piece exists. The new code is primarily the grouping algorithm and the glue between scanner detection and plan creation.

## Common Pitfalls

### Pitfall 1: Reading Metadata from Only the First File
**What goes wrong:** `metadata.ExtractMetadata` reads the first audio file and returns. For split detection, every file needs individual metadata.
**Why it happens:** The existing API is designed for per-directory metadata, not per-file.
**How to avoid:** Call `extractWithTag` (or the full fallback chain) on each audio file individually in the grouper. Do NOT use `ExtractMetadata` -- it only reads the first file.
**Warning signs:** All files in a multi-book folder report the same title.

### Pitfall 2: Shared File Copy vs Move
**What goes wrong:** Using `VerifiedMove` for shared files (covers) means only the first group gets the cover, and subsequent moves fail because the source is gone.
**Why it happens:** D-05 requires shared files be COPIED to all groups. Move deletes the source.
**How to avoid:** Implement `fileops.VerifiedCopy` (hash source, copy, hash dest, compare). Mark shared-file operations distinctly in the plan so the engine knows to copy not move.
**Warning signs:** "file not found" errors on shared files after the first group is processed.

### Pitfall 3: Distinguishing Shared vs Audio Files in Plan Operations
**What goes wrong:** The `PlanOperation` struct has `SourcePath` and `DestPath` but no flag to indicate copy vs move.
**Why it happens:** All existing operations are either move or delete -- no copy semantic existed before.
**How to avoid:** Two options: (a) add a metadata/notes field to PlanOperation, or (b) use naming convention in the plan (e.g., operations with source that is a non-audio file are copies). Option (b) is simpler -- check file extension in the split executor to decide copy vs move.
**Warning signs:** Shared files disappearing from source directory.

### Pitfall 4: Confidence Threshold Too Low
**What goes wrong:** Algorithm groups files incorrectly when metadata is sparse (e.g., files with no title tag).
**Why it happens:** M4A files from different sources have wildly varying metadata quality.
**How to avoid:** Per D-03, when confidence is below threshold, skip the folder entirely and flag as "needs manual review." Start with a conservative threshold (e.g., require title+author match for at least 80% of files in each group). Users can always use CSV import for ambiguous cases.
**Warning signs:** Groups with single files, or groups where metadata fields are empty strings.

### Pitfall 5: Skill Allowed-Tools Too Broad
**What goes wrong:** `allowed-tools: Bash(earworm *)` would also match `earworm plan apply` since it starts with `earworm`.
**Why it happens:** The `allowed-tools` field grants permission without prompting, and wildcard matching is prefix-based.
**How to avoid:** The deny-list in the skill instructions is the primary guardrail (Claude reads and follows instructions). The `allowed-tools` field is about bypassing per-use permission prompts, not about blocking commands. Claude's instruction-following is the safety mechanism, combined with `disable-model-invocation: true` for the skill itself to prevent autonomous triggering.
**Warning signs:** Claude running `plan apply` or `cleanup` without being asked.

### Pitfall 6: Cobra Flag State in Tests
**What goes wrong:** Test flag contamination when running multiple CLI tests.
**Why it happens:** Cobra flags are package-level vars that persist across tests (Phase 08 and 12 decisions).
**How to avoid:** Reset cobra help flag value and Changed state in `executeCommand` test helper. Reset any new split-related flags (e.g., `splitConfirm`, `splitJSON`) between tests.
**Warning signs:** Tests passing individually but failing when run together.

## Code Examples

### Per-File Metadata Extraction (Core of Grouping Algorithm)

```go
// Read metadata from each audio file individually for grouping
func extractPerFileMetadata(audioFiles []string) (map[string]*metadata.BookMetadata, error) {
    results := make(map[string]*metadata.BookMetadata)
    for _, filePath := range audioFiles {
        // Try tag first, then ffprobe -- skip folder fallback (meaningless per-file)
        meta, err := extractWithTag(filePath)
        if err != nil || meta.Title == "" {
            meta, err = extractWithFFprobe(filePath)
        }
        if err != nil {
            // File has no extractable metadata -- will use filename grouping
            results[filePath] = nil
            continue
        }
        results[filePath] = meta
    }
    return results, nil
}
```

Note: `extractWithTag` and `extractWithFFprobe` are currently unexported in the metadata package. They need to be exported (or a new exported function created) for the split package to use them per-file.

### Grouping by Metadata Tuple

```go
// Group files by (title, author) metadata key
func groupByMetadata(perFile map[string]*metadata.BookMetadata) map[string][]string {
    groups := make(map[string][]string)
    for path, meta := range perFile {
        if meta == nil || meta.Title == "" {
            groups["_unknown"] = append(groups["_unknown"], path)
            continue
        }
        key := strings.ToLower(meta.Title) + "|" + strings.ToLower(meta.Author)
        groups[key] = append(groups[key], path)
    }
    return groups
}
```

### VerifiedCopy (New fileops function)

```go
// VerifiedCopy copies src to dst, creating parent dirs, then verifies SHA-256 match.
func VerifiedCopy(src, dst string) error {
    srcHash, err := HashFile(src)
    if err != nil {
        return fmt.Errorf("verified copy: %w", err)
    }

    if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
        return fmt.Errorf("verified copy mkdir: %w", err)
    }

    srcFile, err := os.Open(src)
    if err != nil {
        return fmt.Errorf("verified copy open: %w", err)
    }
    defer srcFile.Close()

    dstFile, err := os.Create(dst)
    if err != nil {
        return fmt.Errorf("verified copy create: %w", err)
    }
    defer dstFile.Close()

    if _, err := io.Copy(dstFile, srcFile); err != nil {
        os.Remove(dst)
        return fmt.Errorf("verified copy write: %w", err)
    }

    dstHash, err := HashFile(dst)
    if err != nil {
        return fmt.Errorf("verified copy hash dest: %w", err)
    }

    if srcHash != dstHash {
        os.Remove(dst)
        return fmt.Errorf("hash mismatch after copy: src=%s dst=%s", srcHash, dstHash)
    }
    return nil
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None -- `go test ./...` |
| Quick run command | `go test ./internal/split/... ./internal/fileops/... ./internal/planengine/... ./internal/cli/... -count=1 -short` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FOPS-04a | GroupFiles clusters audio by metadata (title+author) | unit | `go test ./internal/split/... -run TestGroupFiles -count=1` | No -- Wave 0 |
| FOPS-04b | GroupFiles falls back to filename patterns when metadata sparse | unit | `go test ./internal/split/... -run TestGroupFiles_FilenameFallback -count=1` | No -- Wave 0 |
| FOPS-04c | GroupFiles skips folder when confidence below threshold | unit | `go test ./internal/split/... -run TestGroupFiles_LowConfidence -count=1` | No -- Wave 0 |
| FOPS-04d | CreateSplitPlan generates correct plan operations | unit | `go test ./internal/split/... -run TestCreateSplitPlan -count=1` | No -- Wave 0 |
| FOPS-04e | Split execution moves audio files with SHA-256 verification | integration | `go test ./internal/planengine/... -run TestExecuteOp_Split -count=1` | No -- Wave 0 |
| FOPS-04f | Split execution copies shared files to all groups | integration | `go test ./internal/planengine/... -run TestExecuteOp_SplitSharedFiles -count=1` | No -- Wave 0 |
| FOPS-04g | Split audit trail records before/after state | unit | `go test ./internal/planengine/... -run TestSplitAudit -count=1` | No -- Wave 0 |
| FOPS-04h | CLI split detect shows proposed groupings | unit | `go test ./internal/cli/... -run TestSplitDetect -count=1` | No -- Wave 0 |
| FOPS-04i | CLI split plan creates plan from groupings | unit | `go test ./internal/cli/... -run TestSplitPlan -count=1` | No -- Wave 0 |
| INTG-02a | SKILL.md exists with correct frontmatter | manual-only | Verify file exists at `.claude/skills/earworm/SKILL.md` | No -- Wave 0 |
| INTG-02b | Skill deny-list prevents execution commands | manual-only | Invoke `/earworm` in Claude Code, attempt `plan apply` -- should refuse | N/A |

### Sampling Rate
- **Per task commit:** `go test ./internal/split/... ./internal/fileops/... ./internal/planengine/... ./internal/cli/... -count=1 -short`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/split/grouper_test.go` -- covers FOPS-04a, FOPS-04b, FOPS-04c
- [ ] `internal/split/planner_test.go` -- covers FOPS-04d
- [ ] `internal/planengine/engine_test.go` -- add split cases for FOPS-04e, FOPS-04f, FOPS-04g (file exists but needs new test cases)
- [ ] `internal/cli/split_test.go` -- covers FOPS-04h, FOPS-04i
- [ ] `internal/fileops/copy.go` + `internal/fileops/copy_test.go` -- VerifiedCopy function

## Open Questions

1. **How to distinguish copy vs move in PlanOperation for split**
   - What we know: PlanOperation has source_path, dest_path, op_type. No "copy vs move" flag.
   - What's unclear: Best way to encode the distinction without adding a DB column.
   - Recommendation: Use file extension in the split executor -- non-audio files (`.jpg`, `.jpeg`, `.png`, `.json`) are always copies; audio files (`.m4a`, `.m4b`) are always moves. This is deterministic and requires no schema change.

2. **Metadata function visibility**
   - What we know: `extractWithTag` and `extractWithFFprobe` are unexported in the metadata package.
   - What's unclear: Whether to export them or create a new exported wrapper.
   - Recommendation: Create `metadata.ExtractFileMetadata(filePath string)` that runs the tag->ffprobe chain on a single specified file (skipping folder fallback). Cleaner than exporting internals.

3. **Split CLI as subcommand or standalone**
   - What we know: D-07 mentions `plan create` and `plan show` but also implies `split detect` as a distinct command.
   - What's unclear: Whether split is `earworm split detect <path>` or `earworm plan create --split <path>`.
   - Recommendation: `earworm split detect <path>` for detection/preview and `earworm split plan <path>` for plan creation. Keeps split as its own command namespace since it has distinct detection logic.

## Sources

### Primary (HIGH confidence)
- Project codebase: `internal/scanner/issues.go` -- existing multi-book detection
- Project codebase: `internal/planengine/engine.go` -- plan execution patterns
- Project codebase: `internal/fileops/hash.go` -- SHA-256 verification patterns
- Project codebase: `internal/db/plans.go` -- ValidOpTypes includes "split"
- Project codebase: `internal/metadata/metadata.go` -- metadata extraction chain
- Project codebase: `internal/organize/path.go` -- BuildBookPath for Libation naming
- [Claude Code Skills Documentation](https://code.claude.com/docs/en/skills) -- SKILL.md format, frontmatter, allowed-tools

### Secondary (MEDIUM confidence)
- [Claude Code slash commands guide](https://dev.to/whoffagents/how-to-build-claude-code-skills-custom-slash-commands-that-actually-work-1nje) -- best practices for skill authoring

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all infrastructure exists in codebase, no new dependencies needed
- Architecture: HIGH -- follows established patterns (plan operations, fileops, Cobra CLI)
- Pitfalls: HIGH -- based on direct code reading and understanding of metadata extraction limitations
- Claude Skill: HIGH -- official documentation read, format is straightforward YAML + markdown

**Research date:** 2026-04-11
**Valid until:** 2026-05-11 (stable -- no external dependencies, all internal code)
