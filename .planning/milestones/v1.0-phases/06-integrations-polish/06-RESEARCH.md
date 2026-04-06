# Phase 6: Integrations & Polish - Research

**Researched:** 2026-04-04
**Domain:** HTTP API integration, CSV export, daemon/polling loop, Go CLI patterns
**Confidence:** HIGH

## Summary

Phase 6 adds four capabilities to complete the Earworm v1 workflow: (1) Audiobookshelf library scan triggers via its REST API, (2) Goodreads CSV export from the local book database, (3) a daemon/polling mode for unattended operation, and (4) comprehensive README documentation. All four are well within Go's standard library capabilities with no new dependencies required.

The Audiobookshelf integration is a thin HTTP client making one POST request with Bearer auth -- roughly 30 lines of code. Goodreads CSV export uses Go's `encoding/csv` stdlib to produce a file matching Goodreads' import format. The daemon mode wraps the existing sync/download/organize pipeline in a `time.Ticker` loop with signal handling borrowed from the download command. None of these features require new third-party dependencies.

**Primary recommendation:** Build three new internal packages (`audiobookshelf`, `goodreads`, `daemon`) with corresponding CLI commands (`notify`, `goodreads`, `daemon`), plus a `daemon.polling_interval` config key. All use stdlib only. Add a `daemon.polling_interval` config default and wire ABS notification into the download pipeline as a post-completion hook.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Trigger ABS library scan once after full batch completes (end of download/organize pipeline). No per-book scans.
- **D-02:** If ABS is unreachable, warn and continue. Downloads/organization are not blocked by ABS availability. Books are already in the library -- user can trigger scan manually later.
- **D-03:** Silent skip if `audiobookshelf.url` is unconfigured. No nagging or hints. Users who don't use ABS never see it mentioned.
- **D-04:** Add standalone `earworm notify` command for manual ABS scan trigger. Useful when ABS was down during download, or user organized files outside Earworm.
- **D-05:** Use `net/http` stdlib for ABS API calls (`POST /api/libraries/{id}/scan` with Bearer token auth). No third-party HTTP client needed.
- **D-06:** CSV export approach -- `earworm goodreads` generates a Goodreads-compatible CSV from the Audible library. User uploads it to Goodreads manually. No web scraping, no external tool wrapping.
- **D-07:** One-way sync: Audible -> Goodreads only. Export library as CSV for Goodreads import.
- **D-08:** Exported books land on the "read" shelf by default. No shelf configuration needed for v1.
- **D-09:** Dedicated `earworm daemon` subcommand. Runs in foreground, polls on interval, runs full sync->download->organize->notify cycle each poll. Ctrl+C to stop. Easy to wrap with systemd/launchd.
- **D-10:** Default polling interval: 6 hours. Configurable via `daemon.polling_interval` config key. Conservative default suitable for NAS setups.
- **D-11:** Quiet by default between polls -- only log when something happens (new books found, downloads started, errors). `--verbose` flag for heartbeat/cycle logs.
- **D-12:** Full pipeline cycle always -- each poll runs sync, download, organize, notify. No partial step configuration. Simple mental model.
- **D-13:** Reuse Phase 4's two-stage Ctrl+C pattern: first SIGINT finishes current book then stops daemon. Second SIGINT kills immediately.
- **D-14:** README covers: installation (Go binary + audible-cli setup), quickstart guide (auth->sync->download), full command reference with flags.
- **D-15:** Include step-by-step audible-cli setup instructions directly in README (install Python, pip install audible-cli, earworm auth). One place for users to follow.

### Claude's Discretion
- Goodreads CSV format details (column names, date formats) -- match Goodreads import expectations
- ABS API error parsing and retry logic details within the "warn and continue" constraint
- Daemon tick/poll implementation (time.Ticker vs time.After)
- README structure and ordering of sections
- Whether `earworm notify` needs `--quiet` and `--json` flags (probably yes, for consistency)
- Whether daemon mode needs a `--once` flag for testing (run one cycle then exit)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| INT-01 | User can trigger an Audiobookshelf library scan via its REST API after downloads complete | ABS API `POST /api/libraries/{id}/scan` with Bearer token; stdlib `net/http`; auto-trigger after organize step |
| INT-02 | User can configure Audiobookshelf connection (API URL, Bearer token, library ID) | Config keys already exist in `internal/config/config.go`: `audiobookshelf.url`, `.token`, `.library_id` |
| INT-03 | User can sync their Audible library to Goodreads via external CLI tooling | CSV export using `encoding/csv` stdlib; Goodreads import column format documented below |
| INT-04 | User can run Earworm in polling/daemon mode to periodically check for and download new books | `time.Ticker` + signal handling; new `daemon.polling_interval` config key; reuses existing pipeline |
| CLI-05 | README is updated with each phase to reflect current capabilities | Full README with install, quickstart, command reference, config reference |
| TEST-11 | Integration tests for external integrations (Audiobookshelf API mock, Goodreads sync, daemon mode lifecycle) | `net/http/httptest` for ABS mock; CSV output verification; daemon start/stop with context cancellation |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go, single binary distribution
- **CLI framework:** Cobra + Viper (already in use)
- **HTTP client:** `net/http` stdlib only (per stack decisions)
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests
- **Error handling:** Cobra RunE pattern, wrap errors with `fmt.Errorf("context: %w", err)`
- **CLI pattern:** One file per command in `internal/cli/`, `--quiet` and `--json` flags for consistency
- **Config:** Viper with YAML at `~/.config/earworm/config.yaml`
- **Output:** `cmd.OutOrStdout()` for testable output

## Standard Stack

### Core (already in go.mod -- no new dependencies)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| net/http (stdlib) | Go 1.26 | Audiobookshelf API client | 2 endpoints total, stdlib is sufficient per CLAUDE.md |
| encoding/csv (stdlib) | Go 1.26 | Goodreads CSV export | Standard CSV writer, no edge cases that need a library |
| time (stdlib) | Go 1.26 | Daemon polling ticker | time.Ticker for interval-based polling |
| net/http/httptest (stdlib) | Go 1.26 | ABS API test mocking | Standard HTTP test server for integration tests |
| os/signal (stdlib) | Go 1.26 | Daemon signal handling | Reuse Phase 4's two-stage pattern |

**No new dependencies required.** All features use Go stdlib.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| net/http | go-resty/resty | Overkill for 2 endpoints; adds dependency for no benefit |
| time.Ticker | cron library | Ticker is simpler for fixed-interval polling; cron is for complex schedules |
| encoding/csv | manual string formatting | CSV has quoting rules; stdlib handles edge cases correctly |

## Architecture Patterns

### New Package Structure
```
internal/
  audiobookshelf/       # NEW: ABS API client
    client.go           # ScanLibrary(), ListLibraries()
    client_test.go      # httptest-based tests
  goodreads/            # NEW: CSV export
    export.go           # ExportCSV() from []db.Book
    export_test.go      # CSV output verification
  daemon/               # NEW: Polling loop orchestration
    daemon.go           # Run() with ticker + signal handling
    daemon_test.go      # Lifecycle tests with context cancellation
  cli/
    notify.go           # NEW: earworm notify command
    goodreads.go        # NEW: earworm goodreads command
    daemon.go           # NEW: earworm daemon command
```

### Pattern 1: Audiobookshelf Client
**What:** Thin HTTP client wrapping ABS REST API
**When to use:** After organize completes, or via `earworm notify`

```go
// internal/audiobookshelf/client.go
package audiobookshelf

import (
    "fmt"
    "net/http"
    "time"
)

type Client struct {
    BaseURL    string
    Token      string
    LibraryID  string
    HTTPClient *http.Client // injectable for testing
}

func NewClient(baseURL, token, libraryID string) *Client {
    return &Client{
        BaseURL:    baseURL,
        Token:      token,
        LibraryID:  libraryID,
        HTTPClient: &http.Client{Timeout: 30 * time.Second},
    }
}

// ScanLibrary triggers a library scan. Returns nil on success.
// Non-2xx responses return an error but are not fatal to the caller.
func (c *Client) ScanLibrary() error {
    url := fmt.Sprintf("%s/api/libraries/%s/scan", c.BaseURL, c.LibraryID)
    req, err := http.NewRequest("POST", url, nil)
    if err != nil {
        return fmt.Errorf("creating scan request: %w", err)
    }
    req.Header.Set("Authorization", "Bearer "+c.Token)

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return fmt.Errorf("scan request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("scan returned status %d", resp.StatusCode)
    }
    return nil
}
```

### Pattern 2: Goodreads CSV Export
**What:** Generate Goodreads-compatible CSV from book database
**When to use:** Via `earworm goodreads` command

The Goodreads CSV import format expects these columns (verified from Goodreads export files):

| Column | Required | Earworm Source | Notes |
|--------|----------|----------------|-------|
| Title | Yes | `book.Title` | Direct mapping |
| Author | Yes | `book.Author` | Direct mapping |
| ISBN | No | Empty | Audible books don't have ISBNs |
| ISBN13 | No | Empty | Same as above |
| My Rating | No | Empty | Not tracked |
| Average Rating | No | Empty | Not tracked |
| Publisher | No | Empty | Not tracked |
| Year Published | No | `book.Year` | If available from metadata |
| Date Read | No | `book.PurchaseDate` | Best approximation |
| Date Added | No | `book.CreatedAt` | When added to earworm DB |
| Bookshelves | Yes (for shelf) | `"read, audiobook"` | Per D-08 |
| Exclusive Shelf | Yes (for shelf) | `"read"` | Per D-08 |

**Key insight:** Both `Bookshelves` and `Exclusive Shelf` columns must be present for shelf assignment to work. The `Exclusive Shelf` takes values: "read", "currently-reading", "to-read".

```go
// internal/goodreads/export.go
func ExportCSV(w io.Writer, books []db.Book) error {
    cw := csv.NewWriter(w)
    defer cw.Flush()

    header := []string{
        "Title", "Author", "ISBN", "ISBN13", "My Rating",
        "Average Rating", "Publisher", "Year Published",
        "Date Read", "Date Added", "Bookshelves", "Exclusive Shelf",
    }
    if err := cw.Write(header); err != nil {
        return fmt.Errorf("writing CSV header: %w", err)
    }

    for _, b := range books {
        record := []string{
            b.Title,
            b.Author,
            "",  // ISBN -- Audible doesn't provide
            "",  // ISBN13
            "",  // My Rating
            "",  // Average Rating
            "",  // Publisher
            yearString(b.Year),
            b.PurchaseDate,
            b.CreatedAt.Format("2006/01/02"),
            "read, audiobook",
            "read",
        }
        if err := cw.Write(record); err != nil {
            return fmt.Errorf("writing CSV row for %s: %w", b.ASIN, err)
        }
    }
    return cw.Error()
}
```

### Pattern 3: Daemon Polling Loop
**What:** Foreground process that periodically runs the full pipeline
**When to use:** Via `earworm daemon` command

```go
// internal/daemon/daemon.go
func Run(ctx context.Context, interval time.Duration, cycle func(ctx context.Context) error, verbose bool) error {
    // Run first cycle immediately
    if err := cycle(ctx); err != nil {
        slog.Error("cycle failed", "error", err)
    }

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if verbose {
                slog.Info("starting poll cycle")
            }
            if err := cycle(ctx); err != nil {
                slog.Error("cycle failed", "error", err)
                // Continue polling -- don't exit on cycle errors
            }
        }
    }
}
```

**Recommendation: Use `time.Ticker` over `time.After`.** Ticker provides clean interval-based semantics and is naturally cancellable via select. `time.After` leaks timers if not consumed before context cancellation. Ticker is the idiomatic Go choice for polling loops.

### Pattern 4: Pipeline Hook for ABS Notification
**What:** After download+organize completes, optionally trigger ABS scan
**Where:** In the `earworm download` command flow, or in the daemon cycle

The ABS notification should be called at the CLI command level (not deep in the pipeline), because:
1. It depends on config (URL/token) which lives in Viper
2. It's a side effect, not part of the download/organize core logic
3. It should be silent-skippable (D-03)

```go
// In download.go runDownload(), after pipeline.Run() completes successfully:
if absURL := viper.GetString("audiobookshelf.url"); absURL != "" && summary.Succeeded > 0 {
    abs := audiobookshelf.NewClient(absURL,
        viper.GetString("audiobookshelf.token"),
        viper.GetString("audiobookshelf.library_id"))
    if err := abs.ScanLibrary(); err != nil {
        slog.Warn("Audiobookshelf scan failed", "error", err)
        // D-02: warn and continue, don't fail the command
    } else if !quiet {
        fmt.Fprintln(cmd.OutOrStdout(), "Audiobookshelf library scan triggered.")
    }
}
```

### Anti-Patterns to Avoid
- **Per-book ABS scans:** D-01 explicitly says one scan after full batch. Scanning per-book would hammer the server.
- **Blocking on ABS failure:** D-02 says warn and continue. Never `return err` from an ABS call.
- **Mentioning ABS when unconfigured:** D-03 says silent skip. No "configure Audiobookshelf at..." hints.
- **Custom CSV formatting:** Use `encoding/csv` -- it handles quoting, escaping, commas in titles correctly.
- **Deep pipeline coupling for ABS:** Keep ABS notification at the CLI/command level, not inside download or organize packages.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CSV generation | Manual string concat with commas | `encoding/csv` stdlib | Titles with commas, quotes, or newlines need proper escaping |
| HTTP test mocking | Custom mock server | `net/http/httptest` stdlib | Purpose-built, handles port allocation, cleanup |
| Interval polling | Manual goroutine + sleep | `time.Ticker` | Handles interval drift, clean Stop() semantics |
| Signal handling | Raw os.Signal | `signal.NotifyContext` | Already proven in Phase 4's download.go |

## Common Pitfalls

### Pitfall 1: Goodreads CSV Column Names Must Be Exact
**What goes wrong:** CSV imports to Goodreads fail silently or create wrong shelves
**Why it happens:** Goodreads is very sensitive to exact column naming. `Bookshelves` and `Exclusive Shelf` must both be present for shelf assignment. Using just one or the other causes books to land on the wrong shelf.
**How to avoid:** Use the exact column names from a Goodreads export file. Include both `Bookshelves` and `Exclusive Shelf` columns.
**Warning signs:** Imported books have no shelf assignment, or all land on "to-read"

### Pitfall 2: Goodreads Date Format
**What goes wrong:** Dates don't parse correctly during Goodreads import
**Why it happens:** Goodreads exports dates in `yyyy/MM/dd` format (e.g., `2024/01/15`). Import expects the same format.
**How to avoid:** Format dates as `yyyy/MM/dd`. The `PurchaseDate` from Audible is ISO format (`2024-01-15`) which needs reformatting.
**Warning signs:** Date Read shows up blank after import

### Pitfall 3: ABS Scan Returns Immediately (Async)
**What goes wrong:** Code assumes scan is complete when POST returns
**Why it happens:** `POST /api/libraries/{id}/scan` starts an async scan and returns 200 immediately. The scan continues in the background.
**How to avoid:** Don't poll for scan completion. Just fire and forget. The 200 response means "scan started", not "scan complete".
**Warning signs:** None -- this is actually the desired behavior. Just document it.

### Pitfall 4: Daemon Signal Handling Race
**What goes wrong:** Signal arrives during pipeline execution, daemon exits uncleanly
**Why it happens:** Two levels of signal handling: daemon loop and inner pipeline (download has its own signal handling from Phase 4).
**How to avoid:** Use a shared context. The daemon creates a context via `signal.NotifyContext`. This same context is passed to the pipeline cycle. When SIGINT arrives, the context cancels, the inner pipeline finishes its current book, and the daemon loop's `ctx.Done()` case fires.
**Warning signs:** Second SIGINT during download doesn't force-kill

### Pitfall 5: Ticker Drift with Long Cycles
**What goes wrong:** If a sync+download+organize cycle takes 2 hours and interval is 6 hours, next tick fires 4 hours after cycle ends, not 6
**Why it happens:** `time.Ticker` fires at fixed intervals from creation, not from cycle completion
**How to avoid:** This is actually acceptable behavior for a 6-hour interval. The alternative (reset ticker after each cycle) adds complexity for minimal benefit. Document this behavior.
**Warning signs:** None -- this is fine for the use case

### Pitfall 6: executeCommand Test Helper Needs New Flag Resets
**What goes wrong:** Tests bleed state between runs
**Why it happens:** The `executeCommand` helper in `cli_test.go` resets all package-level flag variables. New commands add new flag variables that must be added to this reset list.
**How to avoid:** When adding `notify.go`, `goodreads.go`, `daemon.go` to `internal/cli/`, ensure any new package-level flag vars (e.g., `notifyJSON`, `goodreadsOutput`, `daemonVerbose`, `daemonOnce`, `daemonInterval`) are added to the `executeCommand` reset block in `cli_test.go`.
**Warning signs:** Tests pass individually but fail when run together

## Code Examples

### Audiobookshelf Integration Test with httptest
```go
func TestScanLibrary_Success(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "POST", r.Method)
        assert.Equal(t, "/api/libraries/lib123/scan", r.URL.Path)
        assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    client := audiobookshelf.NewClient(server.URL, "test-token", "lib123")
    err := client.ScanLibrary()
    require.NoError(t, err)
}

func TestScanLibrary_Unreachable(t *testing.T) {
    client := audiobookshelf.NewClient("http://127.0.0.1:1", "token", "lib")
    err := client.ScanLibrary()
    require.Error(t, err) // Connection refused
}
```

### Goodreads CSV Test
```go
func TestExportCSV(t *testing.T) {
    books := []db.Book{
        {
            Title:        "Project Hail Mary",
            Author:       "Andy Weir",
            Year:         2021,
            PurchaseDate: "2024-01-15",
            CreatedAt:    time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
        },
    }

    var buf bytes.Buffer
    err := goodreads.ExportCSV(&buf, books)
    require.NoError(t, err)

    reader := csv.NewReader(&buf)
    records, err := reader.ReadAll()
    require.NoError(t, err)
    require.Len(t, records, 2) // header + 1 row

    // Verify header
    assert.Equal(t, "Title", records[0][0])
    assert.Equal(t, "Exclusive Shelf", records[0][11])

    // Verify data
    assert.Equal(t, "Project Hail Mary", records[1][0])
    assert.Equal(t, "Andy Weir", records[1][1])
    assert.Equal(t, "read", records[1][11])
}
```

### Daemon Lifecycle Test
```go
func TestDaemon_StopsOnCancel(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cycleCount := 0

    go func() {
        // Let one cycle complete, then cancel
        time.Sleep(100 * time.Millisecond)
        cancel()
    }()

    err := daemon.Run(ctx, 1*time.Hour, func(ctx context.Context) error {
        cycleCount++
        return nil
    }, false)

    assert.ErrorIs(t, err, context.Canceled)
    assert.Equal(t, 1, cycleCount) // Only the initial immediate cycle
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Goodreads API | CSV import only | ~2020 (API deprecated) | No programmatic access; CSV is the only import method |
| ABS undocumented API | Partial official docs | 2024-2025 | `/api/libraries/{id}/scan` works but response format not fully documented |

**Deprecated/outdated:**
- Goodreads developer API was shut down in 2020. CSV import is now the only way to bulk-add books.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed (go test built-in) |
| Quick run command | `go test ./internal/audiobookshelf/... ./internal/goodreads/... ./internal/daemon/... ./internal/cli/... -run "Test(Notify\|Goodreads\|Daemon)" -v` |
| Full suite command | `go test ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| INT-01 | ABS scan trigger after downloads | integration | `go test ./internal/audiobookshelf/... -v` | No (Wave 0) |
| INT-02 | ABS config (url, token, library_id) | unit | `go test ./internal/config/... -v` | Partially (config.go has keys, no validation test) |
| INT-03 | Goodreads CSV export | unit | `go test ./internal/goodreads/... -v` | No (Wave 0) |
| INT-04 | Daemon polling mode | integration | `go test ./internal/daemon/... -v` | No (Wave 0) |
| CLI-05 | README documentation | manual | N/A (review README content) | No |
| TEST-11 | Integration tests for all integrations | integration | `go test ./internal/audiobookshelf/... ./internal/goodreads/... ./internal/daemon/... -v` | No (Wave 0) |

### Sampling Rate
- **Per task commit:** `go test ./internal/audiobookshelf/... ./internal/goodreads/... ./internal/daemon/... ./internal/cli/... -v`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/audiobookshelf/client_test.go` -- covers INT-01, INT-02 (httptest mock)
- [ ] `internal/goodreads/export_test.go` -- covers INT-03 (CSV format verification)
- [ ] `internal/daemon/daemon_test.go` -- covers INT-04 (lifecycle, context cancellation)
- [ ] CLI test flag resets in `internal/cli/cli_test.go` for new commands

## Open Questions

1. **Goodreads date format edge case**
   - What we know: Goodreads exports use `yyyy/MM/dd`. Audible's `PurchaseDate` is ISO `yyyy-MM-dd`.
   - What's unclear: Whether Goodreads import also accepts ISO dates.
   - Recommendation: Reformat to `yyyy/MM/dd` to match the export format. Safe choice.

2. **ABS scan response body**
   - What we know: POST returns 200 on success. The scan runs asynchronously.
   - What's unclear: What the response body contains (if anything).
   - Recommendation: Only check status code. Don't parse the body. Per D-02, any non-success is a warning.

3. **Books with no author/title for Goodreads export**
   - What we know: Some books in the DB might have empty author or title (from scanning local files with missing metadata).
   - What's unclear: How Goodreads handles rows with empty Title or Author.
   - Recommendation: Skip books with empty Title or Author in the export. Log a warning for skipped books.

4. **Daemon `--once` flag**
   - What we know: A `--once` flag (run one cycle then exit) would be useful for testing and cron-based setups.
   - What's unclear: Whether to include in v1.
   - Recommendation: Include it. Trivial to implement (check flag, skip ticker loop), very useful for CI and debugging.

## Sources

### Primary (HIGH confidence)
- Existing codebase: `internal/config/config.go` -- ABS config keys already defined
- Existing codebase: `internal/cli/download.go` -- Two-stage signal handling pattern (lines 84-98)
- Existing codebase: `internal/cli/cli_test.go` -- executeCommand helper with flag reset pattern
- Existing codebase: `internal/db/books.go` -- Book struct with all fields needed for Goodreads export
- Go stdlib docs: `encoding/csv`, `net/http`, `net/http/httptest`, `time.Ticker`
- [Goodreads export CSV gist](https://gist.github.com/tmcw/f077b2f174a0194f62b94bec4e88f4d0) -- exact column names verified
- [Audiobookshelf GitHub Discussion #1012](https://github.com/advplyr/audiobookshelf/discussions/1012) -- scan endpoint confirmed
- [Audiobookshelf API Reference](https://api.audiobookshelf.org/) -- Bearer token auth, library endpoints

### Secondary (MEDIUM confidence)
- [Goodreads Help: How do I import or export my books?](https://help.goodreads.com/s/article/How-do-I-import-or-export-my-books-1553870934590) -- import/export instructions
- [Goodreads CSV import forum](https://www.goodreads.com/topic/show/419981-csv-import-export-format) -- Both Bookshelves AND Exclusive Shelf needed

### Tertiary (LOW confidence)
- Goodreads date format (`yyyy/MM/dd`) -- inferred from export files, not officially documented for imports

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all stdlib, no new dependencies, patterns proven in prior phases
- Architecture: HIGH -- three small packages with clear boundaries, well-established Go patterns
- Pitfalls: HIGH -- most are from direct codebase analysis; Goodreads format is MEDIUM (inferred from exports)

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (stable -- ABS API and Goodreads CSV format change rarely)
