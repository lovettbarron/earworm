# Phase 5: File Organization - Context

**Gathered:** 2026-04-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Organize downloaded audiobooks from the local staging directory into Libation-compatible `Author/Title [ASIN]/` folder structure at the configured library path (typically a NAS mount). Includes path construction, file naming, cross-filesystem move handling, and a standalone `earworm organize` command for recovery. This phase makes downloaded books visible to Audiobookshelf.

</domain>

<decisions>
## Implementation Decisions

### Folder Naming
- **D-01:** Require both author and title metadata to organize a book. If either is missing, mark as error and refuse to organize. Library consistency over convenience.
- **D-02:** Multi-author books use first listed author only for folder path. Full author list stays in DB metadata.
- **D-03:** Strip characters illegal on Windows/macOS/Linux (: / \ * ? " < > |) from author and title names. Keep names readable.
- **D-04:** Truncate individual folder name components at 255 characters (standard filesystem limit) to handle NAS/SMB compatibility.

### File Placement
- **D-05:** Cover art named `cover.jpg` inside each book folder. Standard name auto-detected by Audiobookshelf.
- **D-06:** Chapter metadata stored as `chapters.json` sidecar file alongside the M4A. Useful for debugging and tooling.
- **D-07:** M4A audio filename -- Claude's discretion based on Libation/Audiobookshelf compatibility research.

### Cross-Filesystem Moves
- **D-08:** Try `os.Rename` first (fast for same filesystem). On EXDEV error, fall back to copy+delete. Handles both local and NAS seamlessly.
- **D-09:** On copy failure (network drop, disk full), clean up partial file on destination, keep staging copy intact, mark book as error. User re-runs to retry.
- **D-10:** Verify copy succeeded by comparing source and destination file sizes before deleting staging file. Fast, catches truncated copies.

### Organization Trigger
- **D-11:** Organization happens automatically as part of the download pipeline (per Phase 4 D-12), AND via standalone `earworm organize` command for recovery/manual use.
- **D-12:** `earworm organize` operates on all staged books with 'downloaded' status. No ASIN filtering needed -- just run it.
- **D-13:** If a book's folder already exists at the library path, overwrite existing files. Re-downloads should update.

### Claude's Discretion
- M4A audio filename convention (D-07) -- pick based on Libation/Audiobookshelf compatibility
- Internal package structure for the organizer (likely `internal/organize/`)
- How to report progress during organize operations (consistent with download pipeline patterns)
- Whether `earworm organize` needs `--quiet` and `--json` flags (probably yes, for consistency)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Specs
- `.planning/PROJECT.md` -- Core value, constraints (Go, M4A only, Libation-compatible structure)
- `.planning/REQUIREMENTS.md` -- ORG-01, ORG-02, ORG-03, TEST-09, TEST-10 are in scope
- `.planning/ROADMAP.md` SS Phase 5 -- Success criteria and dependency on Phase 4

### Prior Phase Context
- `.planning/phases/01-foundation-configuration/01-CONTEXT.md` -- DB schema, config paths (library_path, staging_path), CLI patterns
- `.planning/phases/02-local-library-scanning/02-CONTEXT.md` -- Scanner two-level structure (Author/Title), metadata extraction, folder name parsing
- `.planning/phases/04-download-pipeline/04-CONTEXT.md` -- Staging workflow (D-11, D-12, D-13), verification approach, download status transitions

### Existing Code
- `internal/metadata/folder.go` -- Already parses Author/Title [ASIN] folder convention (reverse of what Phase 5 constructs)
- `internal/scanner/scanner.go` -- Two-level scan (Author/Title) defines the expected structure
- `internal/config/config.go` -- library_path and staging_path config keys
- `internal/db/books.go` -- Book struct, "organized" status already defined in ValidStatuses

### Technology
- `CLAUDE.md` SS Technology Stack -- dhowden/tag for M4A reading, os/exec for subprocess
- `CLAUDE.md` SS Conventions -- Established patterns (project layout, DB driver, migrations, config, CLI, testing, error handling)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/metadata/folder.go` -- `extractFromFolderName` parses Author/Title [ASIN] -- the reverse logic informs path construction
- `internal/db/books.go` -- Book struct with Author, Title, ASIN fields; "organized" status already valid
- `internal/config/config.go` -- `library_path` and `staging_path` config keys with validation
- `internal/cli/root.go` -- Cobra root with `--quiet` flag pattern

### Established Patterns
- Cobra commands in `internal/cli/`, one file per command, RunE for error propagation
- `cmd.OutOrStdout()` for testable output
- `--quiet` and `--json` flags on commands
- Book status transitions validated in Go code (not DB constraints)
- testify/assert + testify/require for tests
- In-memory SQLite for DB tests

### Integration Points
- Phase 4's download pipeline calls the organizer after each book's download + verification
- New `internal/organize/` package for path construction, file moves, and cross-filesystem handling
- `earworm organize` command wired in `internal/cli/organize.go`
- DB status transition: "downloaded" -> "organized" after successful move

</code_context>

<specifics>
## Specific Ideas

- Path construction is the inverse of `metadata/folder.go`'s parsing -- construct `Author/Title [ASIN]/` from Book struct fields
- The EXDEV error check pattern: `if errors.Is(err, syscall.EXDEV)` triggers copy+delete fallback
- Size verification is consistent with Phase 4's lightweight verification philosophy (D-13: "Quick, catches corrupt downloads without heavy processing")
- `earworm organize` is the recovery path when Phase 4's auto-organize fails (e.g., NAS was unmounted during download)

</specifics>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 05-file-organization*
*Context gathered: 2026-04-04*
