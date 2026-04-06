# Phase 2: Local Library Scanning - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-03
**Phase:** 02-local-library-scanning
**Areas discussed:** ASIN extraction, Metadata reading, Status display, Error handling

---

## ASIN Extraction

### Q1: How strict should ASIN extraction be from folder names?

| Option | Description | Selected |
|--------|-------------|----------|
| Strict Libation only | Only recognize Author/Title [ASIN]/ pattern. Skip non-matching folders. | |
| Fuzzy matching | Try multiple patterns — brackets, parentheses, standalone B0-prefix. | ✓ |
| Configurable | Default strict, allow custom regex via config. | |

**User's choice:** Fuzzy matching
**Notes:** None

### Q2: When a folder has no recognizable ASIN, what should happen?

| Option | Description | Selected |
|--------|-------------|----------|
| Skip with warning | Log warning, continue scanning. Summary of skipped items at end. | ✓ |
| Index without ASIN | Add to DB as "discovered" with no ASIN. Won't match Audible later. | |
| You decide | Claude picks best approach. | |

**User's choice:** Skip with warning
**Notes:** None

### Q3: Should the scanner handle nested author folders?

| Option | Description | Selected |
|--------|-------------|----------|
| Two levels only | Expect Author/Title [ASIN]/ at library root. | |
| Recursive walk | Walk entire tree, find ASIN pattern at any depth. | |
| You decide | Claude picks based on Libation output. | |

**User's choice:** Other — Enable recursive as a flag/config option, but default to two levels only.
**Notes:** User wants both options available via configuration.

---

## Metadata Reading

### Q1: What metadata should the scanner extract from M4A files?

| Option | Description | Selected |
|--------|-------------|----------|
| Basic | Title, author, duration, narrator. Fast scan. | |
| Comprehensive | All available: title, author, narrator, duration, genre, year, series, cover art, chapters. | ✓ |
| Minimal | Just confirm M4A exists. Rely on folder names. | |

**User's choice:** Comprehensive
**Notes:** None

### Q2: When dhowden/tag can't read an M4A file, how should the scanner handle it?

| Option | Description | Selected |
|--------|-------------|----------|
| Folder name fallback | Extract from folder name, mark as 'partial'. | |
| ffprobe fallback | Shell out to ffprobe for second attempt. | |
| Both in sequence | dhowden/tag → ffprobe → folder name. Most robust. | ✓ |

**User's choice:** Both in sequence
**Notes:** None

---

## Status Display

### Q1: What should the default `earworm status` output look like?

| Option | Description | Selected |
|--------|-------------|----------|
| Summary + table | Header with counts, then table of books. Lipgloss-styled. | |
| Compact list | One line per book. Dense, grep-friendly. | ✓ |
| Grouped by author | Books under author headers with indented details. | |

**User's choice:** Compact list
**Notes:** None

### Q2: What columns/info should appear in the compact output?

| Option | Description | Selected |
|--------|-------------|----------|
| Author - Title [ASIN] | Minimal identifying info. | |
| Author - Title [ASIN] (duration, narrator) | Add duration and narrator. | |
| Author - Title [ASIN] + status flag | Add status indicator (OK, partial, missing). | ✓ |

**User's choice:** Author - Title [ASIN] + status flag
**Notes:** None

### Q3: Should `earworm status` support filtering?

| Option | Description | Selected |
|--------|-------------|----------|
| No filtering for now | Show everything, pipe to grep. | |
| Basic --author flag | Single filter flag. | |
| You decide | Claude picks. | ✓ |

**User's choice:** You decide
**Notes:** None

---

## Error Handling

### Q1: When scanning hits permission errors, what should happen?

| Option | Description | Selected |
|--------|-------------|----------|
| Skip + warn | Log each error, continue. Summary of skipped paths at end. | ✓ |
| Fail fast | Stop immediately on first permission error. | |
| You decide | Claude picks for NAS context. | |

**User's choice:** Skip + warn
**Notes:** None

### Q2: How should `earworm scan` report progress?

| Option | Description | Selected |
|--------|-------------|----------|
| Counter with spinner | Spinner with "Scanning... 47 books found". | ✓ |
| Silent until done | No output during scan. | |
| Verbose logging | Log each directory entered. | |

**User's choice:** Counter with spinner
**Notes:** None

### Q3: Should rescanning update existing entries or wipe and rebuild?

| Option | Description | Selected |
|--------|-------------|----------|
| Incremental update | Compare filesystem vs DB. Add/update/mark removed. | ✓ |
| Full rebuild | Drop and rebuild from scratch. | |
| You decide | Claude picks based on data model. | |

**User's choice:** Incremental update
**Notes:** None

---

## Claude's Discretion

- Filtering flags on `earworm status` (--author, --status) — user deferred this to Claude's judgment.

## Deferred Ideas

None — discussion stayed within phase scope.
