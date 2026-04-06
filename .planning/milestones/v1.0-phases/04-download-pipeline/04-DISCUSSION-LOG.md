# Phase 4: Download Pipeline - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md -- this log preserves the alternatives considered.

**Date:** 2026-04-04
**Phase:** 04-download-pipeline
**Areas discussed:** Progress reporting, Interrupt & recovery, Retry & failure tracking, Staging workflow

---

## Progress Reporting

| Option | Description | Selected |
|--------|-------------|----------|
| Compact status line | One updating line per book: "[3/12] Downloading: Author - Title [ASIN]... 45%" | ✓ |
| Verbose streaming | Stream audible-cli's raw output plus earworm's own status | |
| Summary only | Just show "Downloading 12 books..." then a completion summary | |

**User's choice:** Compact status line
**Notes:** Keeps terminal clean, consistent with scan spinner pattern from Phase 2

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, speed + ETA | Show download speed and estimated time remaining | ✓ |
| No, just percentage | Keep it simple -- percentage per book and book count | |
| You decide | Claude picks based on what audible-cli's output provides | |

**User's choice:** Yes, speed + ETA
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| Silent until done | No output until complete, then print summary | ✓ |
| Errors only | Suppress progress but still print failures in real-time | |

**User's choice:** Silent until done
**Notes:** Consistent with --quiet on scan/status

| Option | Description | Selected |
|--------|-------------|----------|
| Always show summary | Summary always printed, even in --quiet mode | ✓ |
| Only on failure | Skip summary if everything succeeded | |
| Respect --quiet fully | No summary in --quiet mode at all | |

**User's choice:** Always show summary
**Notes:** None

---

## Interrupt & Recovery

| Option | Description | Selected |
|--------|-------------|----------|
| Finish current book | First Ctrl+C finishes current book, second kills immediately | ✓ |
| Kill immediately | Single Ctrl+C kills audible-cli subprocess right away | |
| Prompt the user | On Ctrl+C, ask "Finish current book? (y/n)" | |

**User's choice:** Finish current book (two-stage Ctrl+C)
**Notes:** Prevents partial files on graceful shutdown

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-detect and report | Detect incomplete state, print resume message, no special flag | ✓ |
| Require --resume flag | User must explicitly pass --resume to continue | |
| Always fresh start | Each invocation finds all undownloaded books naturally | |

**User's choice:** Auto-detect and report
**Notes:** No special --resume flag needed

| Option | Description | Selected |
|--------|-------------|----------|
| Clean up on startup | Delete orphaned staging files, re-download from scratch | ✓ |
| Keep and attempt resume | Try to resume partial files if audible-cli supports it | |
| Leave for user | Print warning about orphaned files, let user decide | |

**User's choice:** Clean up on startup
**Notes:** Simple, avoids corrupt files

---

## Retry & Failure Tracking

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, auto-retry in batch | Each book gets up to max_retries attempts with exponential backoff | ✓ |
| No, single attempt per run | One try per invocation, user re-runs to retry | |
| Retry at end of batch | Download everything once, then retry failures in second pass | |

**User's choice:** Yes, auto-retry in batch
**Notes:** Uses existing config: max_retries (3), backoff_multiplier (2.0)

| Option | Description | Selected |
|--------|-------------|----------|
| Included automatically | Next earworm download includes failed books, retry count resets | ✓ |
| Separate retry command | earworm download --retry-failed targets only failed books | |
| Manual reset required | Failed books stay failed until user explicitly resets | |

**User's choice:** Included automatically
**Notes:** Just re-run the command

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, categorize errors | Parse audible-cli output: network (retry), auth (abort), rate limit (longer backoff) | ✓ |
| Treat all errors the same | Any failure = retry with backoff, no parsing | |

**User's choice:** Yes, categorize errors
**Notes:** Different error types get different handling strategies

---

## Staging Workflow

| Option | Description | Selected |
|--------|-------------|----------|
| Local temp dir | ~/.config/earworm/staging/, always local filesystem | ✓ |
| Next to library root | .staging/ folder next to library root path | |
| System temp directory | os.TempDir(), may be cleaned by OS on reboot | |

**User's choice:** Local temp dir (~/.config/earworm/staging/)
**Notes:** Fast writes, avoids NAS latency, staging_path config allows override

| Option | Description | Selected |
|--------|-------------|----------|
| After each book | Move immediately after download + verification | ✓ |
| After entire batch | Keep all in staging until batch completes | |
| User triggers move | Downloads stay in staging until explicit command | |

**User's choice:** After each book
**Notes:** Frees staging space, makes progress visible in library sooner

| Option | Description | Selected |
|--------|-------------|----------|
| Basic verification | Check file exists, non-zero size, M4A header readable via dhowden/tag | ✓ |
| No verification | Trust audible-cli's exit code | |
| Full verification | Verify integrity, duration, all expected files present | |

**User's choice:** Basic verification
**Notes:** Quick, catches corrupt downloads without heavy processing

---

## Claude's Discretion

- audible-cli output parsing patterns for progress, speed, and error categorization
- Rate limiter implementation (token bucket vs simple sleep)
- DB schema additions for download tracking
- Book selection logic for download command (filtering, --limit flag)

## Deferred Ideas

None -- discussion stayed within phase scope
