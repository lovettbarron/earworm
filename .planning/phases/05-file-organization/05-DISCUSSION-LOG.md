# Phase 5: File Organization - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md -- this log preserves the alternatives considered.

**Date:** 2026-04-04
**Phase:** 05-file-organization
**Areas discussed:** Folder naming, File placement, Cross-filesystem moves, Organization trigger

---

## Folder Naming

### Missing Metadata Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Require both | Refuse to organize if author or title is missing -- mark as error | :white_check_mark: |
| Fallback to ASIN | Use Unknown Author/Unknown Title [ASIN] as fallback | |
| You decide | Claude picks | |

**User's choice:** Require both
**Notes:** Safest for library consistency.

### Multi-Author Books

| Option | Description | Selected |
|--------|-------------|----------|
| First author only | Use first listed author for folder path | :white_check_mark: |
| Joined authors | Combine all authors with & separator | |
| You decide | Claude picks based on Libation compatibility | |

**User's choice:** First author only
**Notes:** Full author list stays in DB metadata.

### Special Character Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Strip unsafe chars | Remove characters illegal on Windows/macOS/Linux | :white_check_mark: |
| Replace with underscore | Replace unsafe characters with underscores | |
| You decide | Claude picks | |

**User's choice:** Strip unsafe chars
**Notes:** Matches Libation convention.

### Path Length Limit

| Option | Description | Selected |
|--------|-------------|----------|
| Truncate at 255 chars | Truncate individual folder name components at 255 chars | :white_check_mark: |
| No limit | Trust filesystem to reject if too long | |
| You decide | Claude picks | |

**User's choice:** Truncate at 255 chars
**Notes:** Handles NAS/SMB compatibility.

---

## File Placement

### M4A Audio Filename

| Option | Description | Selected |
|--------|-------------|----------|
| Title.m4a | Name matches book title | |
| Original filename | Keep audible-cli output filename | |
| ASIN.m4a | Use ASIN as filename | |
| You decide | Claude picks based on compatibility | :white_check_mark: |

**User's choice:** You decide
**Notes:** Claude has discretion on audio filename convention.

### Cover Art Filename

| Option | Description | Selected |
|--------|-------------|----------|
| cover.jpg | Standard name auto-detected by Audiobookshelf | :white_check_mark: |
| folder.jpg | Alternative media server convention | |
| You decide | Claude picks | |

**User's choice:** cover.jpg
**Notes:** Standard convention.

### Chapter Metadata

| Option | Description | Selected |
|--------|-------------|----------|
| chapters.json | JSON sidecar file alongside M4A | :white_check_mark: |
| Embedded only | Rely on chapters embedded in M4A | |
| You decide | Claude picks | |

**User's choice:** chapters.json
**Notes:** Useful for debugging and tooling.

---

## Cross-Filesystem Moves

### Move Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Try rename, fall back to copy+delete | Attempt os.Rename first, EXDEV fallback | :white_check_mark: |
| Always copy+delete | Skip rename, always copy then delete | |
| You decide | Claude picks | |

**User's choice:** Try rename, fall back to copy+delete
**Notes:** Handles both local and NAS seamlessly.

### Copy Failure Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Clean up partial + mark error | Delete partial on destination, keep staging, mark error | :white_check_mark: |
| Leave partial + mark error | Leave partial for manual recovery | |
| You decide | Claude picks | |

**User's choice:** Clean up partial + mark error
**Notes:** Prevents Audiobookshelf from picking up corrupt files.

### Copy Verification

| Option | Description | Selected |
|--------|-------------|----------|
| Size check | Compare source and destination file sizes | :white_check_mark: |
| Checksum (SHA-256) | Hash both files and compare | |
| You decide | Claude picks | |

**User's choice:** Size check
**Notes:** Fast, consistent with Phase 4's lightweight verification philosophy.

---

## Organization Trigger

### Command Model

| Option | Description | Selected |
|--------|-------------|----------|
| Auto only | Organization only via download pipeline | |
| Auto + standalone command | Auto after download, plus `earworm organize` for recovery | :white_check_mark: |
| You decide | Claude picks | |

**User's choice:** Auto + standalone command
**Notes:** More flexible recovery scenario.

### Standalone Command Scope

| Option | Description | Selected |
|--------|-------------|----------|
| All staged books | Find everything with 'downloaded' status | :white_check_mark: |
| Specific ASINs | Require --asin flag | |
| You decide | Claude picks | |

**User's choice:** All staged books
**Notes:** Simple recovery -- just run the command.

### Conflict Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Overwrite | Replace existing files | :white_check_mark: |
| Skip with warning | Leave existing folder, print warning | |
| You decide | Claude picks | |

**User's choice:** Overwrite
**Notes:** Re-downloads should update.

---

## Claude's Discretion

- M4A audio filename convention (user deferred to Claude)
- Internal package structure for the organizer
- Progress reporting during organize operations
- Whether `earworm organize` needs `--quiet` and `--json` flags

## Deferred Ideas

None -- discussion stayed within phase scope
