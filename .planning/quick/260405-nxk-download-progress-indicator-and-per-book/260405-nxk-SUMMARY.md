---
phase: quick
plan: 260405-nxk
subsystem: download
tags: [download, progress, timeout, ux]
dependency_graph:
  requires: []
  provides: [per-book-timeout, elapsed-ticker]
  affects: [internal/download, internal/config, internal/cli]
tech_stack:
  added: []
  patterns: [context.WithTimeout-per-book, goroutine-ticker-with-stop-channel]
key_files:
  created:
    - internal/download/progress_test.go
  modified:
    - internal/download/pipeline.go
    - internal/download/progress.go
    - internal/download/pipeline_test.go
    - internal/config/config.go
    - internal/cli/download.go
decisions:
  - Per-book timeout wraps context.WithTimeout on the download ctx; errors are unwrapped to prevent batch abort
  - Ticker uses a stop channel (not context) to ensure clean shutdown even without timeout
  - tickInterval is package-level var for test override (50ms in tests, 10s production)
metrics:
  duration: 4min
  completed: 2026-04-05
  tasks: 1
  files: 6
---

# Quick Task 260405-nxk: Download Progress Indicator and Per-Book Timeout Summary

Per-book context.WithTimeout (default 30min, configurable) and elapsed-time ticker goroutine printing every 10s during audible-cli downloads.

## What Was Done

### Task 1: Add per-book timeout and elapsed time ticker to download pipeline

**TDD approach: RED then GREEN.**

1. **config.go**: Added `download.timeout_minutes` default (30), validation (>= 0), and ValidKeys entry.

2. **PipelineConfig**: Added `TimeoutMinutes int` field, read from viper in cli/download.go.

3. **progress.go**: Added `FormatElapsed()` and `PrintElapsed()` methods that format/print `[N/M] Downloading: Author - Title [ASIN]... Xm Ys` with `\r` for in-place updates. Respects quiet mode.

4. **pipeline.go downloadWithRetry()**: Before `p.client.Download()`:
   - If `TimeoutMinutes > 0`, wraps ctx with `context.WithTimeout`
   - Starts goroutine with `time.NewTicker(tickInterval)` calling `PrintElapsed` on each tick
   - Goroutine exits via `tickStop` channel when download completes
   - After download, per-book timeout errors are rewrapped WITHOUT `%w` so `errors.Is(err, context.DeadlineExceeded)` does not match in the Run loop -- this ensures the batch continues to the next book instead of treating it as an interrupt

5. **cli/download.go**: Added `TimeoutMinutes: viper.GetInt("download.timeout_minutes")` to PipelineConfig.

6. **Tests**: 6 new tests covering timeout exceeded, per-book timeout vs batch continuation, ticker writes, timeout=0 disabled, FormatElapsed formatting, and quiet mode.

## Commits

| # | Hash | Message |
|---|------|---------|
| 1 | 8a63f6e | test(quick-260405-nxk): add failing tests for download timeout and elapsed ticker |
| 2 | 0cfff94 | feat(quick-260405-nxk): add per-book timeout and elapsed time ticker |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Per-book timeout errors aborting entire batch**
- **Found during:** Task 1 GREEN phase
- **Issue:** `context.DeadlineExceeded` from per-book timeout was caught by Run loop's interrupt check, aborting the entire batch instead of continuing to next book
- **Fix:** Rewrap per-book timeout errors with `fmt.Errorf` (no `%w`) so `errors.Is` does not match `context.DeadlineExceeded` in the batch loop. Parent context cancellation still correctly triggers interrupt.
- **Files modified:** internal/download/pipeline.go
- **Commit:** 0cfff94

**2. [Rule 2 - Missing functionality] Ticker goroutine leak without timeout**
- **Found during:** Task 1 implementation
- **Issue:** When `TimeoutMinutes=0`, no `dlCancel` exists to stop the ticker goroutine via context
- **Fix:** Used a separate `tickStop` channel (closed after Download returns) instead of relying on context cancellation. Goroutine always exits cleanly regardless of timeout configuration.
- **Files modified:** internal/download/pipeline.go
- **Commit:** 0cfff94

## Verification

```
go test ./... -count=1  -- ALL PASS (13 packages)
go test ./internal/download/... -run "Timeout|Elapsed|Tick" -v  -- 5/5 PASS
go test ./internal/download/... -run "FormatElapsed|PrintElapsed" -v  -- 4/4 PASS
```

## Known Stubs

None.

## Self-Check: PASSED

All 6 key files verified present. Both commit hashes (8a63f6e, 0cfff94) confirmed in git log.
