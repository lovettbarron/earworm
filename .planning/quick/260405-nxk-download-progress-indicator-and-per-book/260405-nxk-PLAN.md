---
phase: quick
plan: 260405-nxk
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/config/config.go
  - internal/download/pipeline.go
  - internal/download/progress.go
  - internal/download/pipeline_test.go
  - internal/download/progress_test.go
  - internal/cli/download.go
autonomous: true
requirements: []

must_haves:
  truths:
    - "User sees elapsed time updates during each book download so they know it's alive"
    - "Downloads that hang beyond the timeout fail with a clear timeout error"
    - "Timeout is configurable via download.timeout_minutes (default 30)"
  artifacts:
    - path: "internal/config/config.go"
      provides: "download.timeout_minutes default"
      contains: "download.timeout_minutes"
    - path: "internal/download/pipeline.go"
      provides: "Per-book timeout wrapper and elapsed time goroutine"
      contains: "context.WithTimeout"
    - path: "internal/download/progress.go"
      provides: "Elapsed time printing method"
  key_links:
    - from: "internal/cli/download.go"
      to: "internal/download/pipeline.go"
      via: "PipelineConfig.TimeoutMinutes field"
      pattern: "TimeoutMinutes"
    - from: "internal/download/pipeline.go"
      to: "internal/download/progress.go"
      via: "PrintElapsed calls in ticker goroutine"
      pattern: "PrintElapsed"
---

<objective>
Add download progress indicator (elapsed time ticker) and per-book timeout to the download pipeline.

Purpose: Currently the CLI shows the book title then nothing until complete/error. For long downloads (audiobooks can take 10+ minutes), users have no feedback. Additionally, if a download hangs (NAS disconnect, network failure), it blocks forever. These two changes make downloads observable and fault-tolerant.

Output: Updated pipeline with elapsed-time ticker during downloads and configurable per-book timeout via context.WithTimeout.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@internal/download/pipeline.go
@internal/download/progress.go
@internal/download/pipeline_test.go
@internal/audible/download.go
@internal/config/config.go
@internal/cli/download.go
</context>

<interfaces>
<!-- Key types and contracts the executor needs -->

From internal/download/pipeline.go:
```go
type PipelineConfig struct {
    StagingDir        string
    LibraryDir        string
    RateLimitSeconds  int
    MaxRetries        int
    BackoffMultiplier float64
    Quiet             bool
    Limit             int
    FilterASINs       []string
}

type Pipeline struct {
    client     audible.AudibleClient
    db         *sql.DB
    config     PipelineConfig
    progress   *ProgressTracker
    verifyFunc  func(path string) error
    sleepFunc   func(ctx context.Context, d time.Duration) error
    decryptFunc func(ctx context.Context, stagingDir string, cmdFactory CmdFactory) error
}
```

From internal/download/progress.go:
```go
type ProgressTracker struct {
    quiet bool
    w     io.Writer
}
func (p *ProgressTracker) PrintBookProgress(current, total int, author, title, asin string, pct int)
```

From internal/audible/download.go:
```go
// Download uses exec.CommandContext which already respects context cancellation.
func (c *client) Download(ctx context.Context, asin string, outputDir string) error
```
</interfaces>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Add per-book timeout and elapsed time ticker to download pipeline</name>
  <files>internal/config/config.go, internal/download/pipeline.go, internal/download/progress.go, internal/cli/download.go, internal/download/pipeline_test.go, internal/download/progress_test.go</files>
  <behavior>
    - Test: Pipeline with 1s timeout and a slow fake downloader (2s sleep) returns context.DeadlineExceeded error, book marked as failed
    - Test: Pipeline with generous timeout and fast fake downloader succeeds normally (timeout does not interfere)
    - Test: PrintElapsed formats "Downloading... 10s", "Downloading... 25s" correctly
    - Test: PrintElapsed in quiet mode produces no output
    - Test: Elapsed ticker goroutine writes periodic updates (use short interval like 50ms in test, verify multiple writes within 200ms)
    - Test: TimeoutMinutes=0 means no per-book timeout (backward compatible)
  </behavior>
  <action>
    1. **config.go** — Add `viper.SetDefault("download.timeout_minutes", 30)` in SetDefaults(). Add "download.timeout_minutes" to ValidKeys(). Add validation in Validate(): timeout_minutes must be >= 0.

    2. **PipelineConfig** in pipeline.go — Add field `TimeoutMinutes int`. This is read from viper in cli/download.go.

    3. **progress.go** — Add method `PrintElapsed(current, total int, author, title, asin string, elapsed time.Duration)` that prints `\r[current/total] Downloading: Author - Title [ASIN]... Xm Ys` using the existing `formatDuration()` helper. Respects quiet mode (no output). Also add `FormatElapsed(...)` that returns the string (for testing).

    4. **pipeline.go downloadWithRetry()** — Before `p.client.Download(ctx, ...)` on line 235:
       - If `p.config.TimeoutMinutes > 0`, wrap ctx with `context.WithTimeout(ctx, time.Duration(p.config.TimeoutMinutes)*time.Minute)` and defer cancel.
       - Start a goroutine with a `time.NewTicker(10 * time.Second)` that calls `p.progress.PrintElapsed(...)` on each tick with the elapsed time since download start. The goroutine exits when a done channel is closed (after Download returns) or when the timeout ctx is cancelled.
       - After `p.client.Download()` returns, close the done channel to stop the ticker goroutine.
       - Print a newline after download completes to move past the \r-updated line.

    5. **cli/download.go** — Add `TimeoutMinutes: viper.GetInt("download.timeout_minutes")` to the PipelineConfig struct literal (around line 115-124).

    6. **Tests** — Add tests in pipeline_test.go for timeout behavior using the existing fakeDownloader pattern (add a sleep to simulate slow download). Add tests in progress_test.go for FormatElapsed/PrintElapsed.

    Important: The ticker goroutine must use a select on both ticker.C and the done channel to avoid goroutine leaks. Use `defer ticker.Stop()` in the goroutine.

    Important: When TimeoutMinutes is 0, do NOT wrap with context.WithTimeout (preserves current behavior for users who haven't configured it, though default is 30).

    Important: The elapsed ticker interval should be a package-level var (e.g., `var tickInterval = 10 * time.Second`) so tests can override it to a short duration.
  </action>
  <verify>
    <automated>cd /Users/albair/src/earworm && go test ./internal/download/... ./internal/config/... -v -count=1 -run "Timeout|Elapsed|Tick" 2>&1 | tail -30</automated>
  </verify>
  <done>
    - `download.timeout_minutes` config key with default 30, validated >= 0
    - Per-book context.WithTimeout wraps audible-cli subprocess context
    - Elapsed time ticker prints every 10s during download (e.g., "Downloading... 30s")
    - Ticker stops cleanly when download completes or times out
    - TimeoutMinutes=0 disables timeout (no context wrapper)
    - All new and existing tests pass
  </done>
</task>

</tasks>

<verification>
```bash
# All download package tests pass
cd /Users/albair/src/earworm && go test ./internal/download/... -v -count=1

# Config tests pass
go test ./internal/config/... -v -count=1

# Full test suite still passes
go test ./... -count=1

# Config key is recognized
go run ./cmd/earworm config get download.timeout_minutes
```
</verification>

<success_criteria>
- `earworm download` shows elapsed time updates every ~10s during each book download
- Downloads exceeding `download.timeout_minutes` (default 30) fail with clear timeout error and pipeline continues to next book
- Setting `download.timeout_minutes: 0` disables per-book timeout
- All existing tests continue to pass
- New tests cover timeout and elapsed ticker behavior
</success_criteria>

<output>
After completion, create `.planning/quick/260405-nxk-download-progress-indicator-and-per-book/260405-nxk-SUMMARY.md`
</output>
