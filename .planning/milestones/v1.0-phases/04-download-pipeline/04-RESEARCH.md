# Phase 4: Download Pipeline - Research

**Researched:** 2026-04-04
**Domain:** Fault-tolerant batch download orchestration, subprocess output streaming, rate limiting, exponential backoff, signal handling, Go concurrency patterns
**Confidence:** HIGH

## Summary

Phase 4 implements the core download pipeline that makes `earworm download` actually download audiobooks. The `internal/audible/` package already has a stubbed `Download` method returning `ErrNotImplemented` -- this phase un-stubs it and builds the orchestration layer around it: rate limiting between requests, exponential backoff on errors, two-stage Ctrl+C handling, progress reporting, staging-then-move workflow, and failure tracking.

The main technical challenges are: (1) implementing the `Download` method to invoke `audible download --asin <ASIN> --aaxc --cover --chapter --output-dir <staging> --no-confirm --quality best` and stream its output for progress parsing, (2) building a download pipeline orchestrator in `internal/download/` that manages the batch queue, rate limiting, retry logic, and signal handling, (3) extending the DB schema with download-tracking columns (retry_count, last_error, download_started_at, download_completed_at), and (4) wiring it all into the existing `earworm download` CLI command (which Phase 3 Plan 03 creates with --dry-run only).

No new external Go dependencies are needed. All patterns use stdlib (os/signal, time.Ticker, context, sync, bufio) plus the existing project stack. The audible-cli `download` command supports `--asin` for single-book downloads, `--aaxc` for M4A format, `--cover` and `--chapter` for supplementary files, `--no-confirm` to skip prompts, and `--output-dir` for target directory.

**Primary recommendation:** Build `internal/download/` as a new package containing Pipeline (orchestrator), RateLimiter, BackoffCalculator, and ProgressTracker. The Pipeline accepts an `audible.AudibleClient` interface for testability. Keep the download loop sequential (one book at a time) with configurable delays -- audible-cli already supports `--jobs` for parallelism but rate limiting concerns make sequential safer for v1.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Compact status line per book: `[3/12] Downloading: Author - Title [ASIN]... 45%` -- single updating line, keeps terminal clean. Consistent with scan spinner pattern from Phase 2.
- **D-02:** Include download speed and ETA estimates when audible-cli output provides enough data to calculate them.
- **D-03:** `--quiet` mode is fully silent until complete -- no progress output at all. Only the completion summary prints.
- **D-04:** Completion summary always prints, even in `--quiet` mode: "Downloaded 10/12 books (2 failed, 47m 23s elapsed)".
- **D-05:** Graceful two-stage Ctrl+C: first SIGINT finishes the current book then stops. Second SIGINT kills immediately, marking current book as incomplete. Prevents partial files on graceful shutdown.
- **D-06:** Auto-detect and report on restart: `earworm download` detects incomplete state and prints "Resuming: 8 of 12 remaining (4 completed previously)". No special --resume flag needed.
- **D-07:** Clean up orphaned staging files on startup: check staging dir for files with no matching 'downloaded' status in DB. Delete them and re-download from scratch. Simple, avoids corrupt files.
- **D-08:** Auto-retry within batch: each book gets up to max_retries (default 3, configurable) attempts with exponential backoff (backoff_multiplier default 2.0) before being marked 'error'. Batch continues to next book after exhausting retries.
- **D-09:** Previously failed books are included automatically in the next `earworm download` alongside new books. Retry count resets on each new invocation. Just re-run the command.
- **D-10:** Categorize errors by parsing audible-cli output: network errors (retry with backoff), auth failures (abort entire batch + print "Run `earworm auth` to re-authenticate"), rate limits (longer backoff delay). Different error types get different handling.
- **D-11:** Default staging directory: `~/.config/earworm/staging/`. Always local filesystem, fast writes, avoids NAS latency. `staging_path` config key (already defined in Phase 1) allows override.
- **D-12:** Move each book to library immediately after download + verification completes. Frees staging space and makes progress visible in library sooner.
- **D-13:** Basic verification before moving: check file exists, non-zero size, and M4A header is readable via dhowden/tag. Quick, catches corrupt downloads without heavy processing.

### Claude's Discretion
- audible-cli output parsing patterns for progress percentage, speed, and error categorization
- Rate limiter implementation details (token bucket vs simple sleep between requests)
- DB schema additions for tracking retry counts, error messages, and download timestamps
- How the download command selects which books to download (all undownloaded, or allow filtering by ASIN/author)
- Whether to support `--limit N` flag to cap batch size

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DL-01 | User can download audiobooks from Audible in M4A format via audible-cli | audible-cli `--aaxc` flag produces M4A files. Un-stub `Download` method with correct flags. |
| DL-02 | Downloads include cover art saved alongside audio files | audible-cli `--cover` flag downloads cover JPG alongside audio. `--cover-size 500` for reasonable quality. |
| DL-03 | Downloads include chapter metadata JSON alongside audio files | audible-cli `--chapter` flag saves `{basename}-chapters.json` alongside audio. |
| DL-04 | Downloads are rate-limited with configurable delays between requests | Simple sleep-based rate limiter using `download.rate_limit_seconds` config (default 5). |
| DL-05 | Downloads use exponential backoff on errors to avoid Audible throttling | BackoffCalculator with `download.backoff_multiplier` (default 2.0) and `download.max_retries` (default 3). |
| DL-06 | User sees per-book and overall progress during batch downloads | D-01/D-02: compact status line with `\r` overwrite. Parse audible-cli output for percentage if available. |
| DL-07 | Batch downloads survive process interruptions and resume from last incomplete book | D-05/D-06: two-stage SIGINT + DB-tracked status. Resume auto-detects on restart. |
| DL-08 | Failed downloads are tracked and can be retried without re-downloading successful books | D-08/D-09: error status in DB, auto-included in next invocation, retry count resets. |
| DL-09 | Downloads go to a local staging directory first, then move to library location | D-11/D-12/D-13: staging dir, immediate move after verification, dhowden/tag M4A check. |
| TEST-07 | Unit tests for download pipeline logic (rate limiting, backoff calculation, retry state machine, progress tracking) | Table-driven tests for RateLimiter, BackoffCalculator, RetryStateMachine, ProgressTracker. |
| TEST-08 | Integration tests for download fault tolerance (interrupt recovery, partial download resume, failure tracking) | In-memory SQLite + fake AudibleClient via interface. Simulate interrupts and failures. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go, single binary distribution
- **CLI framework:** Cobra with RunE, one file per command in `internal/cli/`
- **Config:** Viper with YAML, config at `~/.config/earworm/config.yaml`
- **Database:** modernc.org/sqlite with driver name "sqlite", WAL mode, embedded SQL migrations
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests, viper.Reset() between config tests
- **Error handling:** Cobra RunE pattern, wrap errors with `fmt.Errorf("context: %w", err)`
- **Subprocess:** os/exec with exec.CommandContext for timeout control
- **Rate limiting mandatory:** Must include protections against hammering Audible servers
- **Fault tolerance mandatory:** Downloads must survive interruptions, network failures, and partial downloads with clear recovery
- **M4A only for v1**
- **GSD workflow:** Do not make repo edits outside GSD workflow

## Standard Stack

### Core (already in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| os/exec (stdlib) | Go 1.26 | audible-cli subprocess with streaming output | Project convention. exec.CommandContext for timeout, StdoutPipe/StderrPipe for progress parsing. |
| os/signal (stdlib) | Go 1.26 | Two-stage Ctrl+C handling | Stdlib. signal.NotifyContext for first SIGINT; raw signal.Notify for second. |
| context (stdlib) | Go 1.26 | Cancellation propagation through download pipeline | Stdlib. Context flows from signal handler through pipeline to subprocess. |
| sync (stdlib) | Go 1.26 | WaitGroup for cleanup, atomic for progress counters | Stdlib. sync/atomic for thread-safe progress state. |
| bufio (stdlib) | Go 1.26 | Line-by-line stdout/stderr parsing from subprocess | Stdlib. bufio.Scanner on StdoutPipe for real-time output reading. |
| time (stdlib) | Go 1.26 | Rate limiter delays, backoff calculation, elapsed time tracking | Stdlib. time.Sleep for rate limiting, time.Duration for backoff. |
| modernc.org/sqlite | v1.48.1 | Download state persistence (status, retry_count, last_error) | Already in go.mod. Pure Go, no CGo. |
| spf13/cobra | v1.10.2 | Download command with flags | Already in go.mod. Project convention. |
| spf13/viper | v1.21.0 | Config for rate_limit_seconds, max_retries, backoff_multiplier, staging_path | Already in go.mod. Project convention. |
| dhowden/tag | v0.0.0-20240417053706 | M4A verification before moving from staging to library | Already in go.mod. `tag.ReadFrom()` returns error on corrupt/invalid M4A. |
| testify | v1.11.1 | Test assertions | Already in go.mod. Project convention. |

### No New Dependencies Required
Phase 4 requires zero new Go dependencies. Rate limiting and backoff are simple enough to hand-roll with stdlib. The cenkalti/backoff library is overkill for a sequential download loop with 3 retries.

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── audible/
│   └── download.go         # MODIFIED: un-stub Download method, implement actual audible-cli invocation
├── download/                # NEW: download pipeline orchestration
│   ├── pipeline.go          # Pipeline struct: orchestrates batch download loop
│   ├── ratelimiter.go       # RateLimiter: configurable sleep between downloads
│   ├── backoff.go           # BackoffCalculator: exponential backoff with multiplier
│   ├── progress.go          # ProgressTracker: per-book and batch progress state
│   ├── staging.go           # Staging: verify, move-to-library, cleanup orphans
│   ├── pipeline_test.go     # Unit tests for pipeline logic
│   ├── ratelimiter_test.go  # Unit tests for rate limiter
│   ├── backoff_test.go      # Unit tests for backoff calculator
│   ├── progress_test.go     # Unit tests for progress tracker
│   └── staging_test.go      # Unit tests for staging workflow
├── cli/
│   └── download.go          # MODIFIED: add actual download execution (Phase 3 creates --dry-run only)
└── db/
    ├── books.go             # MODIFIED: add download tracking columns access
    └── migrations/
        └── 004_add_download_tracking.sql  # NEW: retry_count, last_error, download timestamps
```

### Pattern 1: Pipeline Orchestrator with Interface-Based Downloader
**What:** The Pipeline struct accepts an `audible.AudibleClient` interface and a `*sql.DB`. It queries for downloadable books, runs the download loop with rate limiting and retry, and updates DB state at each step.
**When to use:** The `earworm download` command creates a Pipeline and calls `Run()`.
**Example:**
```go
// Source: project patterns + stdlib
type PipelineConfig struct {
    StagingDir        string
    LibraryDir        string
    RateLimitSeconds  int
    MaxRetries        int
    BackoffMultiplier float64
    Quiet             bool
}

type Pipeline struct {
    client  audible.AudibleClient
    db      *sql.DB
    config  PipelineConfig
    progress *ProgressTracker
}

func NewPipeline(client audible.AudibleClient, db *sql.DB, cfg PipelineConfig) *Pipeline {
    return &Pipeline{
        client:   client,
        db:       db,
        config:   cfg,
        progress: NewProgressTracker(cfg.Quiet),
    }
}

// Run executes the download pipeline. Returns summary of results.
// The context should be derived from signal handling for graceful shutdown.
func (p *Pipeline) Run(ctx context.Context) (*Summary, error) {
    // 1. Clean up orphaned staging files (D-07)
    // 2. Query downloadable books (new + previously failed)
    // 3. Report resume state if applicable (D-06)
    // 4. For each book: rate limit -> download -> verify -> move
    // 5. Return summary
}
```

### Pattern 2: Two-Stage Signal Handling
**What:** First SIGINT cancels the pipeline context (finish current book, stop batch). Second SIGINT triggers immediate exit with cleanup.
**When to use:** The download command's RunE function sets up signal handling before creating the pipeline.
**Example:**
```go
// Source: os/signal documentation, Go graceful shutdown patterns
func runDownload(cmd *cobra.Command, args []string) error {
    // First stage: context cancellation on first SIGINT
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    // Second stage: force exit on second SIGINT
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigChan // first signal handled by NotifyContext
        <-sigChan // second signal: force exit
        fmt.Fprintln(os.Stderr, "\nForce stopping. Cleaning up...")
        // Mark current book as incomplete in DB
        os.Exit(1)
    }()

    // Pipeline checks ctx.Done() between books
    pipeline := download.NewPipeline(client, database, config)
    summary, err := pipeline.Run(ctx)
    // Print summary even on cancellation (D-04)
}
```

### Pattern 3: Sequential Download with Rate Limiting
**What:** Simple sleep-based rate limiting between sequential downloads. No token bucket needed for v1 -- books download one at a time with a configurable pause.
**When to use:** Between each book download in the pipeline loop.
**Example:**
```go
// Source: stdlib time package
type RateLimiter struct {
    delay time.Duration
}

func NewRateLimiter(seconds int) *RateLimiter {
    return &RateLimiter{delay: time.Duration(seconds) * time.Second}
}

// Wait blocks for the configured delay, respecting context cancellation.
func (r *RateLimiter) Wait(ctx context.Context) error {
    select {
    case <-time.After(r.delay):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Pattern 4: Exponential Backoff Calculator
**What:** Pure function that calculates delay for retry N given base delay and multiplier. No external library needed.
**When to use:** When a download fails with a retryable error.
**Example:**
```go
// Source: exponential backoff algorithm
type BackoffCalculator struct {
    baseDelay  time.Duration
    multiplier float64
    maxDelay   time.Duration // cap to prevent absurd waits
}

func NewBackoffCalculator(baseSeconds int, multiplier float64) *BackoffCalculator {
    return &BackoffCalculator{
        baseDelay:  time.Duration(baseSeconds) * time.Second,
        multiplier: multiplier,
        maxDelay:   5 * time.Minute, // reasonable cap
    }
}

// Delay returns the backoff duration for attempt N (0-indexed).
// Formula: baseDelay * multiplier^attempt, capped at maxDelay.
func (b *BackoffCalculator) Delay(attempt int) time.Duration {
    delay := float64(b.baseDelay) * math.Pow(b.multiplier, float64(attempt))
    if delay > float64(b.maxDelay) {
        return b.maxDelay
    }
    return time.Duration(delay)
}
```

### Pattern 5: Streaming Subprocess Output for Progress
**What:** Use StdoutPipe + bufio.Scanner to read audible-cli output line-by-line. Parse for progress indicators.
**When to use:** The un-stubbed Download method in `internal/audible/download.go`.
**Example:**
```go
// Source: os/exec StdoutPipe documentation
func (c *client) Download(ctx context.Context, asin string, outputDir string) error {
    args := c.buildArgs("download", "--asin", asin, "--aaxc",
        "--cover", "--cover-size", "500", "--chapter",
        "--output-dir", outputDir, "--no-confirm", "--quality", "best")
    cmd := c.command(ctx, args...)

    stdout, err := cmd.StdoutPipe()
    if err != nil { return fmt.Errorf("stdout pipe: %w", err) }

    stderr, err := cmd.StderrPipe()
    if err != nil { return fmt.Errorf("stderr pipe: %w", err) }

    if err := cmd.Start(); err != nil { return fmt.Errorf("start download: %w", err) }

    // Read stdout for progress (in goroutine)
    var stderrBuf strings.Builder
    go func() { io.Copy(&stderrBuf, stderr) }()

    scanner := bufio.NewScanner(stdout)
    for scanner.Scan() {
        line := scanner.Text()
        // Parse progress from line (audible-cli uses DownloadCounter)
        // Callback to progress tracker
    }

    if err := cmd.Wait(); err != nil {
        exitCode := 1
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        }
        return classifyError("download", stderrBuf.String(), exitCode, err)
    }
    return nil
}
```

### Pattern 6: M4A Verification via dhowden/tag
**What:** Open the downloaded M4A file and call `tag.ReadFrom()`. If it returns metadata without error, the file is valid. If it errors, the file is corrupt.
**When to use:** After download completes, before moving from staging to library (D-13).
**Example:**
```go
// Source: dhowden/tag documentation
func VerifyM4A(filePath string) error {
    f, err := os.Open(filePath)
    if err != nil { return fmt.Errorf("open for verification: %w", err) }
    defer f.Close()

    info, err := f.Stat()
    if err != nil { return fmt.Errorf("stat for verification: %w", err) }
    if info.Size() == 0 { return fmt.Errorf("file is empty: %s", filePath) }

    _, err = tag.ReadFrom(f)
    if err != nil { return fmt.Errorf("M4A metadata unreadable (corrupt?): %w", err) }
    return nil
}
```

### Anti-Patterns to Avoid
- **Parallel downloads in v1:** audible-cli supports `--jobs` but rate limiting is critical. Sequential downloads with configurable delay is safer. Can upgrade later.
- **Token bucket rate limiter:** Overkill for sequential downloads. Simple `time.Sleep` with context cancellation is sufficient.
- **cenkalti/backoff library:** Adding a dependency for 15 lines of math is unnecessary. Hand-roll the exponential backoff calculator.
- **Polling for subprocess completion:** Use `cmd.Wait()` after reading all pipe output. Never call `Wait()` before pipes are fully drained -- this causes race conditions.
- **Catching SIGKILL:** SIGKILL cannot be caught. Only handle SIGINT and SIGTERM. Document that SIGKILL may leave orphaned staging files (cleaned up on next run via D-07).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Subprocess management | Custom process runner | os/exec with CommandContext | Handles process lifecycle, timeouts, pipe management |
| Signal handling | Manual signal loop | signal.NotifyContext + signal.Notify | stdlib handles the complexity of signal routing |
| M4A validation | Custom MP4 parser | dhowden/tag ReadFrom | Already in project; detects corrupt files reliably |
| File move across filesystems | os.Rename only | os.Rename with fallback to copy+delete | os.Rename fails across filesystem boundaries (staging on local SSD, library on NAS) |
| Config reading | Manual file parsing | viper.GetInt/GetFloat64/GetString | Already set up with defaults and validation |

**Key insight:** The download pipeline is an orchestration problem, not a concurrency problem. The complexity is in state management (DB tracking), error categorization, and fault recovery -- not in parallelism. Keep it sequential for v1.

## Common Pitfalls

### Pitfall 1: os.Rename Across Filesystems
**What goes wrong:** `os.Rename()` fails with "invalid cross-device link" when staging dir is on a different filesystem than library dir (e.g., local SSD vs NAS mount).
**Why it happens:** os.Rename is a hardlink operation that only works within a single filesystem.
**How to avoid:** Implement a `moveFile` function that tries `os.Rename` first, then falls back to copy+delete on EXDEV error. This is also relevant for Phase 5 (ORG-03 explicitly requires this).
**Warning signs:** Downloads succeed but "move to library" step fails on NAS-mounted library paths.

### Pitfall 2: Calling cmd.Wait() Before Draining Pipes
**What goes wrong:** Race condition where `Wait()` closes the pipe before the scanner finishes reading, causing lost output or panics.
**Why it happens:** Go's exec.Cmd documentation explicitly warns: "it is incorrect to call Wait before all reads from the pipe have completed."
**How to avoid:** Read stdout/stderr completely (scanner loop finishes, goroutine for stderr completes), THEN call `Wait()`.
**Warning signs:** Intermittent lost progress updates, test flakiness.

### Pitfall 3: audible-cli Output Parsing Fragility
**What goes wrong:** Progress percentage parsing breaks when audible-cli changes its output format.
**Why it happens:** audible-cli output is not a stable API. The DownloadCounter uses Python's click.echo() which can vary between versions.
**How to avoid:** Make progress parsing best-effort. If parsing fails, show "Downloading..." without percentage. Never fail the download because progress couldn't be parsed. Use regex patterns that are forgiving.
**Warning signs:** Progress stuck at 0% but download completes successfully.

### Pitfall 4: Staging Cleanup Deleting User Files
**What goes wrong:** Orphan cleanup (D-07) deletes files the user placed in the staging directory manually.
**Why it happens:** Cleanup logic is too aggressive -- deletes anything without a matching DB record.
**How to avoid:** Only delete files in ASIN-named subdirectories within the staging dir. Better yet, create per-download subdirectories named by ASIN, and only clean those. Never delete the staging root.
**Warning signs:** User reports missing files after running `earworm download`.

### Pitfall 5: Second SIGINT Race Condition
**What goes wrong:** Force exit on second SIGINT doesn't properly mark the current book as incomplete in the DB.
**Why it happens:** os.Exit() is called before the DB update completes.
**How to avoid:** On second SIGINT, set a "force stopping" flag and give a very short grace period (500ms) for the DB update before os.Exit. Or accept that the orphaned staging file will be cleaned up on next run (D-07 handles this).
**Warning signs:** Book stuck in "downloading" status with no staging file after force quit.

### Pitfall 6: Migration Numbering
**What goes wrong:** Using wrong migration number.
**Why it happens:** Not checking existing migrations directory.
**How to avoid:** Current migrations: 001, 002, 003. Next migration MUST be 004.
**Warning signs:** Migration runner errors or skips.

### Pitfall 7: audible-cli --aaxc Downloads as MP3 Sometimes
**What goes wrong:** Some Audible books are encoded with MPEG codec, and `--aaxc` downloads them as `.mp3` instead of `.m4a`.
**Why it happens:** audible-cli's source shows: for MPEG codec, the extension is `.mp3`; for other codecs, it's `.aaxc` which needs decryption. The actual M4A file comes after potential decryption steps.
**How to avoid:** After download, glob the staging directory for the ASIN's files rather than assuming a specific extension. The verification step (D-13) uses dhowden/tag which supports both M4A and MP3 metadata.
**Warning signs:** Verification fails because it looks for `.m4a` but file is `.mp3`.

## Code Examples

### audible-cli Download Command Construction
```bash
# Correct invocation for earworm's needs:
audible download \
  --asin B08C6YJ1LS \
  --aaxc \
  --cover --cover-size 500 \
  --chapter \
  --output-dir /path/to/staging \
  --no-confirm \
  --quality best
```

This produces in the output directory:
- `{basename}-{codec}.aaxc` or `{basename}-{codec}.mp3` (audio)
- `{basename}_(500).jpg` (cover)
- `{basename}-chapters.json` (chapters)

Where `{basename}` is derived from `--filename-mode` (default: ascii).

### DB Migration 004: Download Tracking
```sql
-- Add download tracking columns
ALTER TABLE books ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN download_started_at DATETIME;
ALTER TABLE books ADD COLUMN download_completed_at DATETIME;
```

### DB Functions Needed
```go
// UpdateDownloadStart marks a book as downloading with timestamp
func UpdateDownloadStart(db *sql.DB, asin string) error {
    _, err := db.Exec(`UPDATE books SET status = 'downloading',
        download_started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
        WHERE asin = ?`, asin)
    return err
}

// UpdateDownloadComplete marks download success
func UpdateDownloadComplete(db *sql.DB, asin string, localPath string) error {
    _, err := db.Exec(`UPDATE books SET status = 'downloaded',
        local_path = ?, retry_count = 0, last_error = '',
        download_completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
        WHERE asin = ?`, localPath, asin)
    return err
}

// UpdateDownloadError records a failed attempt
func UpdateDownloadError(db *sql.DB, asin string, retryCount int, errMsg string) error {
    _, err := db.Exec(`UPDATE books SET status = 'error',
        retry_count = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
        WHERE asin = ?`, retryCount, errMsg, asin)
    return err
}

// ListDownloadable returns books that need downloading (new + previously failed).
// Reuses ListNewBooks criteria but also includes books with status 'error'.
func ListDownloadable(db *sql.DB) ([]Book, error) {
    rows, err := db.Query(`SELECT ` + allColumns + ` FROM books
        WHERE (audible_status != '' AND (local_path = '' OR status NOT IN ('downloaded', 'organized')))
        OR status = 'error'
        ORDER BY purchase_date DESC`)
    // ...
}
```

### Pipeline Download Loop
```go
func (p *Pipeline) Run(ctx context.Context) (*Summary, error) {
    // 1. Clean orphaned staging files
    if err := p.cleanOrphans(); err != nil {
        slog.Warn("staging cleanup failed", "error", err)
    }

    // 2. Get downloadable books
    books, err := db.ListDownloadable(p.db)
    if err != nil { return nil, fmt.Errorf("list downloadable: %w", err) }

    if len(books) == 0 {
        return &Summary{Total: 0}, nil
    }

    // 3. Report resume state
    previouslyDone := p.countPreviouslyCompleted()
    if previouslyDone > 0 {
        p.progress.ReportResume(len(books), previouslyDone)
    }

    summary := &Summary{Total: len(books)}
    rateLimiter := NewRateLimiter(p.config.RateLimitSeconds)
    backoff := NewBackoffCalculator(p.config.RateLimitSeconds, p.config.BackoffMultiplier)
    start := time.Now()

    for i, book := range books {
        // Check for cancellation between books (first Ctrl+C)
        select {
        case <-ctx.Done():
            summary.Interrupted = true
            summary.Elapsed = time.Since(start)
            return summary, nil
        default:
        }

        // Rate limit (skip on first book)
        if i > 0 {
            if err := rateLimiter.Wait(ctx); err != nil {
                summary.Interrupted = true
                summary.Elapsed = time.Since(start)
                return summary, nil
            }
        }

        // Download with retry
        err := p.downloadWithRetry(ctx, book, backoff)
        if err != nil {
            var authErr *audible.AuthError
            if errors.As(err, &authErr) {
                summary.AuthFailed = true
                summary.Elapsed = time.Since(start)
                return summary, fmt.Errorf("authentication failed: run `earworm auth` to re-authenticate")
            }
            summary.Failed++
        } else {
            summary.Succeeded++
        }
    }

    summary.Elapsed = time.Since(start)
    return summary, nil
}
```

### Cross-Filesystem File Move
```go
// moveFile moves src to dst, falling back to copy+delete if rename fails
// across filesystem boundaries.
func moveFile(src, dst string) error {
    err := os.Rename(src, dst)
    if err == nil { return nil }

    // Check if it's a cross-device error
    var linkErr *os.LinkError
    if errors.As(err, &linkErr) {
        // Fallback: copy then delete
        if err := copyFile(src, dst); err != nil {
            return fmt.Errorf("copy fallback: %w", err)
        }
        return os.Remove(src)
    }
    return err
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| audible-cli AAX format | AAXC format (--aaxc flag) | audible-cli 0.2+ | M4A/MP3 output without separate decryption step |
| Global rate limiting library | Simple sleep-based delay | N/A (design choice) | Simpler for sequential downloads, no dependency |
| Goroutine-per-download | Sequential with rate limit | N/A (design choice) | Safer for rate limiting, simpler error handling |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed (go test discovers tests) |
| Quick run command | `go test ./internal/download/ ./internal/audible/ ./internal/db/ -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TEST-07a | Rate limiter delays correctly | unit | `go test ./internal/download/ -run TestRateLimiter -count=1` | Wave 0 |
| TEST-07b | Backoff calculator exponential growth and cap | unit | `go test ./internal/download/ -run TestBackoff -count=1` | Wave 0 |
| TEST-07c | Retry state machine respects max_retries | unit | `go test ./internal/download/ -run TestRetry -count=1` | Wave 0 |
| TEST-07d | Progress tracker output format | unit | `go test ./internal/download/ -run TestProgress -count=1` | Wave 0 |
| TEST-07e | M4A verification (valid + corrupt + empty) | unit | `go test ./internal/download/ -run TestVerify -count=1` | Wave 0 |
| TEST-08a | Pipeline resumes from incomplete state | integration | `go test ./internal/download/ -run TestResume -count=1` | Wave 0 |
| TEST-08b | Auth failure aborts batch | integration | `go test ./internal/download/ -run TestAuthAbort -count=1` | Wave 0 |
| TEST-08c | Failed books tracked in DB and retried | integration | `go test ./internal/download/ -run TestFailureTracking -count=1` | Wave 0 |
| TEST-08d | Orphan staging cleanup | integration | `go test ./internal/download/ -run TestOrphanCleanup -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/download/ ./internal/audible/ ./internal/db/ -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/download/` -- entire package is new (pipeline, rate limiter, backoff, progress, staging, all tests)
- [ ] `internal/db/migrations/004_add_download_tracking.sql` -- new migration
- [ ] DB functions: UpdateDownloadStart, UpdateDownloadComplete, UpdateDownloadError, ListDownloadable

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Build/test | Yes | 1.26.1 | -- |
| Python 3.9+ | audible-cli runtime | Yes | Available | -- |
| audible-cli | Actual downloads | No (not installed) | -- | Tests use fake AudibleClient via interface. Manual install needed for real usage. |
| dhowden/tag | M4A verification | Yes | In go.mod | -- |
| modernc.org/sqlite | State persistence | Yes | v1.48.1 | -- |

**Missing dependencies with no fallback:**
- audible-cli is not installed. This does not block development or testing (tests use interface-based fakes). Installation: `pip install audible-cli`.

**Missing dependencies with fallback:**
- None.

## Open Questions

1. **audible-cli download progress output format**
   - What we know: audible-cli uses a DownloadCounter class and click.echo() for output. It prints a summary at the end.
   - What's unclear: Whether audible-cli prints real-time percentage progress to stdout during download, or only a summary after completion. The async download queue makes it likely there's minimal per-byte progress output.
   - Recommendation: Parse stdout line-by-line for any progress indicators. If none found, show an indeterminate "Downloading..." status. The completion is detected by subprocess exit. Do not depend on parsing percentage -- treat it as a nice-to-have enhancement.

2. **audible-cli AAXC vs M4A output**
   - What we know: `--aaxc` downloads the AAXC format, which may need decryption. For MPEG codec, it downloads .mp3 directly. The actual M4A may require ffmpeg for decryption.
   - What's unclear: Whether `--aaxc` alone produces playable M4A files or requires a post-download decryption step.
   - Recommendation: Use `--aaxc` as the default. After download, check what files exist in the staging directory. If `.aaxc` files exist (not `.m4a`), this may indicate decryption is needed -- but that is likely handled by audible-cli itself. Test with actual audible-cli to confirm. Verification via dhowden/tag will catch unplayable files.

3. **`--limit N` flag for batch size**
   - What we know: CONTEXT.md lists this as Claude's discretion.
   - Recommendation: Include `--limit` flag (default 0 = no limit). Simple to implement, useful for users who want to download a few books at a time. Also useful for testing.

4. **ASIN-based filtering**
   - What we know: CONTEXT.md lists book selection as Claude's discretion.
   - Recommendation: Support `--asin` flag (repeatable) to download specific books. Default behavior downloads all undownloaded + previously failed books. The `--asin` filter is useful for re-downloading specific books.

## Discretion Recommendations

Based on the "Claude's Discretion" areas from CONTEXT.md:

| Area | Recommendation | Rationale |
|------|----------------|-----------|
| Rate limiter implementation | Simple sleep-based with `time.After` + context cancellation | Sequential downloads don't need token bucket. Simpler code, easier to test. |
| audible-cli output parsing | Best-effort regex on stdout lines. Fall back to "Downloading..." if no match. | audible-cli output is not a stable API. Defensive parsing prevents breakage. |
| DB schema additions | Add retry_count (INT), last_error (TEXT), download_started_at (DATETIME), download_completed_at (DATETIME) to books table via migration 004 | Minimal additions that support all recovery scenarios. |
| Book selection | Download all undownloaded + error status books by default. Support `--asin` and `--limit` flags for filtering. | Matches D-09 (auto-include failed books) while giving users control. |
| `--limit N` flag | Include with default 0 (no limit). | Low implementation cost, useful for testing and cautious users. |

## Sources

### Primary (HIGH confidence)
- Existing codebase: `internal/audible/` package -- AudibleClient interface, Download stub, error types, command factory pattern
- Existing codebase: `internal/db/books.go` -- Book struct, ValidStatuses, CRUD functions, UpsertBook pattern
- Existing codebase: `internal/config/config.go` -- download.rate_limit_seconds, download.max_retries, download.backoff_multiplier, staging_path defaults
- [Go os/exec documentation](https://pkg.go.dev/os/exec) -- subprocess pipe management, CommandContext
- [Go os/signal documentation](https://pkg.go.dev/os/signal) -- signal.NotifyContext for graceful shutdown
- [dhowden/tag documentation](https://pkg.go.dev/github.com/dhowden/tag) -- ReadFrom for M4A metadata verification

### Secondary (MEDIUM confidence)
- [audible-cli source cmd_download.py](https://github.com/mkb79/audible-cli) -- download flags (--asin, --aaxc, --cover, --chapter, --output-dir, --no-confirm, --quality)
- [Graceful Shutdown in Go: Practical Patterns](https://victoriametrics.com/blog/go-graceful-shutdown/) -- two-stage signal handling
- [Go os/exec Patterns (DoltHub)](https://www.dolthub.com/blog/2022-11-28-go-os-exec-patterns/) -- subprocess pipe reading patterns
- [Reading os/exec.Cmd Output Without Race Conditions](https://hackmysql.com/rand/reading-os-exec-cmd-output-without-race-conditions/) -- Wait() vs pipe draining order

### Tertiary (LOW confidence)
- audible-cli download progress output format -- inferred from DownloadCounter class, not verified with actual output
- AAXC decryption behavior -- unclear whether audible-cli handles decryption automatically

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries already in go.mod, no new dependencies
- Architecture: HIGH -- follows established project patterns, well-understood stdlib patterns
- Pitfalls: HIGH -- identified from codebase analysis, os/exec documentation, and cross-filesystem behavior
- audible-cli integration: MEDIUM -- flags confirmed from source code, but output parsing and AAXC behavior not verified against actual tool

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (30 days -- stable domain, core Go patterns don't change)
