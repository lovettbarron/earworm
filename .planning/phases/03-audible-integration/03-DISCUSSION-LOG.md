# Phase 3: Audible Integration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-03
**Phase:** 03-audible-integration
**Areas discussed:** audible-cli wrapping strategy, Sync & data model, New book detection & dry-run, Auth UX & error recovery

---

## audible-cli Wrapping Strategy

### Auth Setup

| Option | Description | Selected |
|--------|-------------|----------|
| Earworm manages auth | `earworm auth` wraps `audible quickstart` — guides user through login interactively | ✓ |
| User pre-configures | Assume audible-cli already authenticated, just validate | |
| Hybrid | Check for existing profile, launch quickstart if missing | |

**User's choice:** Earworm manages auth
**Notes:** None

### Invocation Style

| Option | Description | Selected |
|--------|-------------|----------|
| Structured subprocess | Build commands via os/exec, parse stdout/stderr, map exit codes to typed errors | ✓ |
| Shell passthrough | Thin wrapper, relay output | |
| JSON output mode | Use audible-cli's --output-format json where available | |

**User's choice:** Structured subprocess
**Notes:** None

### Commands to Wrap

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal: quickstart + library list | Just the two needed for Phase 3 requirements | |
| Extended: + library export | Add richer metadata export | |
| Full inventory | Wrap quickstart, library list, library export, and download (stubbed) | ✓ |

**User's choice:** Full inventory
**Notes:** Build complete wrapper interface now even though download isn't used until Phase 4

---

## Sync & Data Model

### Data Mapping

| Option | Description | Selected |
|--------|-------------|----------|
| Extend books table | Add columns for remote metadata, book can be local-only/remote-only/both | ✓ |
| Separate audible_books table | Join tables on ASIN | |
| Embed JSON blob | Store raw JSON in a column | |

**User's choice:** Extend books table
**Notes:** None

### Conflict Resolution

| Option | Description | Selected |
|--------|-------------|----------|
| Remote wins | Remote metadata overwrites local on sync, local-only fields preserved | ✓ |
| Local wins | Local edits never overwritten | |
| Merge with conflict detection | Compare timestamps, flag conflicts | |

**User's choice:** Remote wins
**Notes:** None

### Sync Mode

| Option | Description | Selected |
|--------|-------------|----------|
| Always full sync | Pull complete library every time, upsert all | ✓ |
| Incremental with timestamp | Only fetch changes since last sync | |
| Full with change detection | Pull everything, diff, only update changed rows | |

**User's choice:** Always full sync
**Notes:** audible-cli doesn't support incremental fetching anyway

---

## New Book Detection & Dry-Run

### Definition of "New"

| Option | Description | Selected |
|--------|-------------|----------|
| Remote minus local | ASIN in Audible but no local_path or not downloaded/organized | ✓ |
| Status-based | Explicit 'needs_download' status | |
| Timestamp-based | Books added after last sync | |

**User's choice:** Remote minus local
**Notes:** None

### Dry-Run Output

| Option | Description | Selected |
|--------|-------------|----------|
| Title list with metadata | Author - Title [ASIN] (runtime), total count, --json support | ✓ |
| Minimal titles only | Just titles and count | |
| Rich preview with sizes | Titles, estimated sizes, download time | |

**User's choice:** Title list with metadata
**Notes:** None

### Dry-Run Command

| Option | Description | Selected |
|--------|-------------|----------|
| `earworm download --dry-run` | Flag on download command, natural UX | ✓ |
| `earworm sync --preview` | Part of sync command | |
| `earworm new` | Dedicated command | |

**User's choice:** `earworm download --dry-run`
**Notes:** None

---

## Auth UX & Error Recovery

### Auth Flow

| Option | Description | Selected |
|--------|-------------|----------|
| Pass-through interactive | Connect stdin/stdout to terminal, user interacts with audible-cli directly | ✓ |
| Guided wrapper | Earworm asks questions first, pre-fills args | |
| Browser-based | Open URL, capture token via browser | |

**User's choice:** Pass-through interactive
**Notes:** None

### Auth Failure Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Detect and guide | Parse error output, show clear message with recovery command | ✓ |
| Auto-retry auth | Automatically launch auth flow on failure | |
| Fail fast | Abort with raw error | |

**User's choice:** Detect and guide
**Notes:** None

### Auth Validation Timing

| Option | Description | Selected |
|--------|-------------|----------|
| Pre-flight check | Verify auth before every sync with lightweight command | ✓ |
| Only on error | Don't pre-check, handle errors as they come | |
| Cached check | Skip if authed within last 24h | |

**User's choice:** Pre-flight check
**Notes:** None

---

## Claude's Discretion

- Specific audible-cli output parsing patterns
- Which lightweight command for pre-flight auth check
- Schema migration numbering and column types
- Error message wording
- Test fixture design for fake subprocess testing

## Deferred Ideas

None — discussion stayed within phase scope
