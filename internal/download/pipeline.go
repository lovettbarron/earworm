package download

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/db"
)

// tickInterval controls how often the elapsed time ticker prints during download.
// Package-level var so tests can override to a short duration.
var tickInterval = 10 * time.Second

// PipelineConfig holds all configuration for the download pipeline.
type PipelineConfig struct {
	StagingDir        string
	LibraryDir        string
	RateLimitSeconds  int
	MaxRetries        int
	BackoffMultiplier float64
	Quiet             bool
	Limit             int      // 0 means no limit
	FilterASINs       []string // empty means all books
	TimeoutMinutes    int      // per-book timeout in minutes; 0 disables
}

// Summary holds the results of a pipeline run.
type Summary struct {
	Total       int
	Succeeded   int
	Failed      int
	Interrupted bool
	AuthFailed  bool
	Elapsed     time.Duration
	Errors      []BookError // per-book error details
}

// String formats the summary per D-04: "Downloaded X/Y books (Z failed, Nm Ns elapsed)"
func (s Summary) String() string {
	elapsedStr := formatDuration(s.Elapsed)
	if s.Failed > 0 {
		return fmt.Sprintf("Downloaded %d/%d books (%d failed, %s elapsed)", s.Succeeded, s.Total, s.Failed, elapsedStr)
	}
	return fmt.Sprintf("Downloaded %d/%d books (%s elapsed)", s.Succeeded, s.Total, elapsedStr)
}

// BookError records a download failure for the completion summary.
type BookError struct {
	ASIN    string
	Title   string
	Author  string
	Message string
}

// Pipeline orchestrates batch audiobook downloads with rate limiting,
// retry logic, error categorization, and DB state management.
type Pipeline struct {
	client   audible.AudibleClient
	db       *sql.DB
	config   PipelineConfig
	progress *ProgressTracker

	// verifyFunc allows overriding VerifyM4A in tests.
	verifyFunc func(path string) error
	// sleepFunc allows overriding time-based sleeps in tests.
	sleepFunc func(ctx context.Context, d time.Duration) error
	// decryptFunc allows overriding DecryptStaged in tests.
	decryptFunc func(ctx context.Context, stagingDir string, cmdFactory CmdFactory) error
	// timeoutForBook overrides the per-book timeout duration (for testing).
	// Zero means use config.TimeoutMinutes converted to minutes.
	timeoutForBook time.Duration

	// Runtime state for progress reporting within downloadWithRetry.
	currentIdx  int // 1-based index of current book in batch
	totalBooks  int // total books in batch
	lastPercent int // last reported download percent (-1 = no data)
	lastRate    string
	w           io.Writer // output writer for progress
}

// NewPipeline creates a new download pipeline.
func NewPipeline(client audible.AudibleClient, database *sql.DB, cfg PipelineConfig, w io.Writer) *Pipeline {
	return &Pipeline{
		client:      client,
		db:          database,
		config:      cfg,
		progress:    NewProgressTracker(w, cfg.Quiet),
		verifyFunc:  VerifyM4A,
		sleepFunc:   defaultSleep,
		decryptFunc: DecryptStaged,
		lastPercent: -1,
		w:           w,
	}
}

// defaultSleep waits for the given duration, respecting context cancellation.
func defaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run executes the download pipeline: cleans orphans, lists downloadable books,
// downloads each sequentially with rate limiting and retry, and tracks DB state.
func (p *Pipeline) Run(ctx context.Context) (*Summary, error) {
	start := time.Now()
	summary := &Summary{}

	// Step 1: Clean orphaned staging files (D-07).
	// Query downloaded ASINs for orphan detection.
	if err := p.cleanOrphans(); err != nil {
		slog.Warn("failed to clean orphaned staging files", "error", err)
		// Don't abort on cleanup failure
	}

	// Step 2: List downloadable books.
	books, err := db.ListDownloadable(p.db)
	if err != nil {
		summary.Elapsed = time.Since(start)
		return summary, fmt.Errorf("listing downloadable books: %w", err)
	}

	// Apply ASIN filter if specified.
	if len(p.config.FilterASINs) > 0 {
		asinSet := make(map[string]bool, len(p.config.FilterASINs))
		for _, a := range p.config.FilterASINs {
			asinSet[a] = true
		}
		var filtered []db.Book
		for _, b := range books {
			if asinSet[b.ASIN] {
				filtered = append(filtered, b)
			}
		}
		books = filtered
	}

	// Apply limit if specified.
	if p.config.Limit > 0 && len(books) > p.config.Limit {
		books = books[:p.config.Limit]
	}

	summary.Total = len(books)
	if summary.Total == 0 {
		summary.Elapsed = time.Since(start)
		return summary, nil
	}

	// Step 3: Report resume state if previously completed books exist (D-06).
	p.reportResumeState(len(books))

	// Step 4: Create rate limiter and backoff calculator.
	rateLimiter := NewRateLimiter(p.config.RateLimitSeconds)
	backoff := NewBackoffCalculator(p.config.RateLimitSeconds, p.config.BackoffMultiplier)

	// Step 5: Download loop.
	for i, book := range books {
		// Check context between books.
		select {
		case <-ctx.Done():
			summary.Interrupted = true
			summary.Elapsed = time.Since(start)
			return summary, nil
		default:
		}

		// Rate limit wait (skip on first book).
		if i > 0 {
			if err := rateLimiter.Wait(ctx); err != nil {
				summary.Interrupted = true
				summary.Elapsed = time.Since(start)
				return summary, nil
			}
		}

		// Print progress.
		p.progress.PrintBookProgress(i+1, summary.Total, book.Author, book.Title, book.ASIN, -1)

		// Set runtime state for ticker goroutine in downloadWithRetry.
		p.currentIdx = i + 1
		p.totalBooks = summary.Total

		// Download with retry.
		downloadErr := p.downloadWithRetry(ctx, book, backoff)

		if downloadErr != nil {
			// Check for auth error — abort entire batch.
			var authErr *audible.AuthError
			if errors.As(downloadErr, &authErr) {
				summary.AuthFailed = true
				summary.Elapsed = time.Since(start)
				return summary, fmt.Errorf("authentication failed: run `earworm auth` to re-authenticate")
			}

			// Check for context cancellation during retry.
			if errors.Is(downloadErr, context.Canceled) || errors.Is(downloadErr, context.DeadlineExceeded) {
				summary.Interrupted = true
				summary.Elapsed = time.Since(start)
				return summary, nil
			}

			// Exhausted retries — record failure, continue to next book.
			summary.Failed++
			summary.Errors = append(summary.Errors, BookError{
				ASIN:    book.ASIN,
				Title:   book.Title,
				Author:  book.Author,
				Message: downloadErr.Error(),
			})
			continue
		}

		summary.Succeeded++
	}

	summary.Elapsed = time.Since(start)
	p.progress.PrintSummary(summary.Succeeded, summary.Total, summary.Failed, summary.Elapsed)
	return summary, nil
}

// downloadWithRetry attempts to download a single book with retry logic.
// Returns nil on success, *audible.AuthError immediately (no retry),
// or the last error after exhausting retries.
func (p *Pipeline) downloadWithRetry(ctx context.Context, book db.Book, backoff *BackoffCalculator) error {
	var lastErr error
	maxAttempts := 1 + p.config.MaxRetries // initial + retries

	// Determine per-book timeout duration.
	bookTimeout := p.timeoutForBook
	if bookTimeout == 0 && p.config.TimeoutMinutes > 0 {
		bookTimeout = time.Duration(p.config.TimeoutMinutes) * time.Minute
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Mark download started in DB.
		if err := db.UpdateDownloadStart(p.db, book.ASIN); err != nil {
			slog.Warn("failed to mark download start", "asin", book.ASIN, "error", err)
		}

		// Create per-ASIN staging subdirectory.
		asinStagingDir := filepath.Join(p.config.StagingDir, book.ASIN)
		if err := os.MkdirAll(asinStagingDir, 0755); err != nil {
			lastErr = fmt.Errorf("creating staging dir for %s: %w", book.ASIN, err)
			continue
		}

		// Wrap context with per-book timeout if configured.
		dlCtx := ctx
		var dlCancel context.CancelFunc
		if bookTimeout > 0 {
			dlCtx, dlCancel = context.WithTimeout(ctx, bookTimeout)
		}

		// Reset progress state and set up progress callback on client.
		p.lastPercent = -1
		p.lastRate = ""
		// Start ticker goroutine — shows % + rate when available, falls back to elapsed time.
		downloadStart := time.Now()
		tickStop := make(chan struct{})
		tickDone := make(chan struct{})
		go func() {
			defer close(tickDone)
			ticker := time.NewTicker(tickInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if p.config.Quiet {
						continue
					}
					if p.lastPercent >= 0 {
						fmt.Fprintf(p.w, "\r  %d%% @ %-12s", p.lastPercent, p.lastRate)
					} else {
						elapsed := time.Since(downloadStart)
						fmt.Fprintf(p.w, "\r  %s elapsed", formatDuration(elapsed))
					}
				case <-tickStop:
					return
				}
			}
		}()

		// Set progress callback if client supports it.
		type progressSetter interface {
			SetProgressFunc(func(audible.DownloadProgress))
		}
		if ps, ok := p.client.(progressSetter); ok {
			ps.SetProgressFunc(func(prog audible.DownloadProgress) {
				p.lastPercent = prog.Percent
				p.lastRate = prog.Rate
			})
		}

		// Attempt download.
		downloadErr := p.client.Download(dlCtx, book.ASIN, asinStagingDir)

		// Stop ticker goroutine and clean up timeout context.
		close(tickStop)
		<-tickDone
		if dlCancel != nil {
			dlCancel()
		}

		// Print newline to move past \r-updated line (if not quiet).
		if !p.config.Quiet {
			fmt.Fprintln(p.progress.w)
		}

		// Distinguish per-book timeout from parent context cancellation.
		// If download returned DeadlineExceeded but the parent context is still OK,
		// it's a per-book timeout — wrap WITHOUT %w so errors.Is won't match
		// context.DeadlineExceeded in the Run loop's interrupt check.
		if downloadErr != nil && errors.Is(downloadErr, context.DeadlineExceeded) && ctx.Err() == nil {
			downloadErr = fmt.Errorf("download timed out after %s for %s", bookTimeout, book.ASIN)
		}

		if downloadErr == nil {
			// Success — decrypt (if AAXC) and verify files remain in staging.
			if err := p.verifyStaged(ctx, book.ASIN, asinStagingDir); err != nil {
				lastErr = err
				// Retry on verify failure
				if attempt < maxAttempts-1 {
					delay := backoff.Delay(attempt)
					if sleepErr := p.sleepFunc(ctx, delay); sleepErr != nil {
						return sleepErr
					}
				}
				continue
			}

			// Mark complete in DB with empty local_path (files remain in staging).
			if err := db.UpdateDownloadComplete(p.db, book.ASIN, ""); err != nil {
				slog.Warn("failed to mark download complete", "asin", book.ASIN, "error", err)
			}
			return nil
		}

		lastErr = downloadErr

		// Auth error — don't retry, propagate immediately.
		var authErr *audible.AuthError
		if errors.As(downloadErr, &authErr) {
			return downloadErr
		}

		// Don't sleep/retry after last attempt.
		if attempt >= maxAttempts-1 {
			break
		}

		// Calculate backoff delay.
		delay := backoff.Delay(attempt)

		// Rate limit error — use doubled backoff.
		var rateLimitErr *audible.RateLimitError
		if errors.As(downloadErr, &rateLimitErr) {
			delay *= 2
		}

		// Sleep with context awareness.
		if sleepErr := p.sleepFunc(ctx, delay); sleepErr != nil {
			return sleepErr
		}

		// Check context between retries.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	// Exhausted retries — update DB with error state.
	if err := db.UpdateDownloadError(p.db, book.ASIN, p.config.MaxRetries, lastErr.Error()); err != nil {
		slog.Warn("failed to mark download error", "asin", book.ASIN, "error", err)
	}
	return lastErr
}

// verifyStaged decrypts AAXC files (if present) and verifies audio files
// remain in staging. Files are NOT moved to library — that is handled by
// the organize command.
func (p *Pipeline) verifyStaged(ctx context.Context, asin string, stagingDir string) error {
	// Step 1: Decrypt AAXC to M4B if applicable.
	if !p.config.Quiet {
		fmt.Fprintf(p.w, "  Decrypting...\n")
	}
	if err := p.decryptFunc(ctx, stagingDir, nil); err != nil {
		return fmt.Errorf("decrypting AAXC for %s: %w", asin, err)
	}

	// Step 2: Glob for audio files in staging (.m4b first, then .m4a).
	var matches []string
	for _, ext := range []string{"*.m4b", "*.m4a"} {
		found, err := filepath.Glob(filepath.Join(stagingDir, ext))
		if err != nil {
			return fmt.Errorf("globbing staging dir for %s: %w", asin, err)
		}
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no audio files (.m4b/.m4a) found in staging for %s", asin)
	}

	// Step 3: Verify each audio file.
	if !p.config.Quiet {
		fmt.Fprintf(p.w, "  Verifying...\n")
	}
	for _, f := range matches {
		if err := p.verifyFunc(f); err != nil {
			return fmt.Errorf("verifying %s: %w", filepath.Base(f), err)
		}
	}

	return nil
}

// cleanOrphans removes orphaned ASIN directories from staging.
func (p *Pipeline) cleanOrphans() error {
	// Ensure staging directory exists.
	if _, err := os.Stat(p.config.StagingDir); os.IsNotExist(err) {
		return nil // nothing to clean
	}

	// Get all downloaded ASINs from DB.
	allBooks, err := db.ListBooks(p.db)
	if err != nil {
		return fmt.Errorf("listing books for orphan cleanup: %w", err)
	}

	downloadedASINs := make(map[string]bool)
	for _, b := range allBooks {
		if b.Status == "downloaded" || b.Status == "organized" || b.Status == "downloading" {
			downloadedASINs[b.ASIN] = true
		}
	}

	return CleanOrphans(p.config.StagingDir, downloadedASINs)
}

// reportResumeState checks for previously completed downloads and prints resume info.
func (p *Pipeline) reportResumeState(remaining int) {
	// Count previously completed books.
	allBooks, err := db.ListBooks(p.db)
	if err != nil {
		return
	}

	previouslyDone := 0
	for _, b := range allBooks {
		if b.DownloadCompletedAt != nil {
			previouslyDone++
		}
	}

	if previouslyDone > 0 {
		msg := p.progress.FormatResume(remaining, previouslyDone)
		fmt.Fprintln(p.progress.w, msg)
	}
}
