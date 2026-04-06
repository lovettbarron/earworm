# Phase 6: Integrations & Polish - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-04
**Phase:** 06-integrations-polish
**Areas discussed:** Audiobookshelf integration, Goodreads sync approach, Daemon/polling mode, Documentation scope

---

## Audiobookshelf Integration

### When to trigger ABS scan

| Option | Description | Selected |
|--------|-------------|----------|
| After full batch completes | One scan at end of download/organize. Avoids per-book API calls. | ✓ |
| After each book organized | Per-book scan trigger. Books appear sooner but many API calls. | |
| Manual only via earworm notify | Separate command, no automatic integration. | |

**User's choice:** After full batch completes
**Notes:** None

### Error handling for unreachable ABS

| Option | Description | Selected |
|--------|-------------|----------|
| Warn and continue | Print warning, don't fail the operation. Books are already in library. | ✓ |
| Retry with backoff | Retry 2-3 times before giving up. Delays pipeline. | |
| Fail the command | Treat as error. Strict notification guarantee. | |

**User's choice:** Warn and continue
**Notes:** None

### ABS config behavior when unconfigured

| Option | Description | Selected |
|--------|-------------|----------|
| Silent skip if unconfigured | Skip scan silently when url is empty. No nagging. | ✓ |
| One-time hint | Print hint on first run, then never again. | |
| Always warn if unconfigured | Print warning each time. | |

**User's choice:** Silent skip if unconfigured
**Notes:** None

### Standalone notify command

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, add earworm notify | Standalone command for manual scan trigger. | ✓ |
| No, batch-end only | Only trigger as part of pipeline. | |

**User's choice:** Yes, add earworm notify
**Notes:** None

---

## Goodreads Sync Approach

### Integration mechanism

| Option | Description | Selected |
|--------|-------------|----------|
| CSV export for manual import | Generate Goodreads-compatible CSV. User uploads manually. | ✓ |
| Wrap an external tool | Wrap audible-cli's goodreads plugin or similar. | |
| Defer to v2 | Skip Goodreads for v1. | |

**User's choice:** CSV export for manual import
**Notes:** None

### Sync direction

| Option | Description | Selected |
|--------|-------------|----------|
| Audible → Goodreads only | One-way push. Mark Audible books on Goodreads. | ✓ |
| Bidirectional | Also pull Goodreads shelves back. | |

**User's choice:** Audible → Goodreads only
**Notes:** None

### Goodreads shelf

| Option | Description | Selected |
|--------|-------------|----------|
| "read" shelf | All exported books marked as read. | ✓ |
| User-chosen shelf | Configurable via --shelf flag. | |
| You decide | Claude picks sensible default. | |

**User's choice:** "read" shelf
**Notes:** None

---

## Daemon/Polling Mode

### Invocation style

| Option | Description | Selected |
|--------|-------------|----------|
| earworm daemon | Dedicated subcommand, foreground, polls on interval. | ✓ |
| earworm download --watch | Flag on existing command. | |
| Cron-friendly (no daemon) | No daemon, users chain commands via cron. | |

**User's choice:** earworm daemon
**Notes:** None

### Polling interval

| Option | Description | Selected |
|--------|-------------|----------|
| 1 hour | Hourly checks. | |
| 6 hours | Conservative, good for NAS. | |
| Configurable, default 1h | Config key with sensible default. | ✓ |

**User's choice:** Configurable, default 6hr
**Notes:** User specified configurable with 6-hour default (combining options)

### Logging behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Quiet by default | Only log when something happens. --verbose for heartbeats. | ✓ |
| Heartbeat each cycle | Log 'No new books found' after each poll. | |
| You decide | Claude picks. | |

**User's choice:** Quiet by default
**Notes:** None

### Pipeline steps per cycle

| Option | Description | Selected |
|--------|-------------|----------|
| Full cycle always | sync→download→organize→notify every poll. | ✓ |
| Configurable steps | Flags to skip steps. | |
| You decide | Claude picks. | |

**User's choice:** Full cycle always
**Notes:** None

---

## Documentation Scope

### README coverage

| Option | Description | Selected |
|--------|-------------|----------|
| Install + quickstart + command ref | Installation, quickstart guide, full command reference. | ✓ |
| Comprehensive with architecture | Above plus architecture, config reference, troubleshooting. | |
| Minimal with --help | Brief README, rely on CLI self-docs. | |

**User's choice:** Install + quickstart + command ref
**Notes:** None

### audible-cli setup instructions

| Option | Description | Selected |
|--------|-------------|----------|
| Include setup steps | Step-by-step in README. One place for users. | ✓ |
| Link to audible-cli docs | Just link, avoid stale duplicate docs. | |
| You decide | Claude picks. | |

**User's choice:** Include setup steps
**Notes:** None

---

## Claude's Discretion

- Goodreads CSV format details (column names, date formats)
- ABS API error parsing details within "warn and continue" constraint
- Daemon tick/poll implementation approach
- README structure and section ordering
- Whether `earworm notify` gets `--quiet`/`--json` flags
- Whether daemon mode gets a `--once` flag for testing

## Deferred Ideas

None — discussion stayed within phase scope
