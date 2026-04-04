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

// PipelineConfig holds all configuration for the download pipeline.
type PipelineConfig struct {
	StagingDir        string
	LibraryDir        string
	RateLimitSeconds  int
	MaxRetries        int
	BackoffMultiplier float64
	Quiet             bool
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
}

// NewPipeline creates a new download pipeline.
func NewPipeline(client audible.AudibleClient, database *sql.DB, cfg PipelineConfig, w io.Writer) *Pipeline {
	return &Pipeline{
		client:     client,
		db:         database,
		config:     cfg,
		progress:   NewProgressTracker(w, cfg.Quiet),
		verifyFunc: VerifyM4A,
		sleepFunc:  defaultSleep,
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

		// Attempt download.
		downloadErr := p.client.Download(ctx, book.ASIN, asinStagingDir)

		if downloadErr == nil {
			// Success — verify and move files.
			if err := p.verifyAndMove(book.ASIN, asinStagingDir); err != nil {
				lastErr = err
				// Retry on verify/move failure
				if attempt < maxAttempts-1 {
					delay := backoff.Delay(attempt)
					if sleepErr := p.sleepFunc(ctx, delay); sleepErr != nil {
						return sleepErr
					}
				}
				continue
			}

			// Mark complete in DB.
			localPath := filepath.Join(p.config.LibraryDir, book.ASIN)
			if err := db.UpdateDownloadComplete(p.db, book.ASIN, localPath); err != nil {
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

// verifyAndMove verifies downloaded M4A files and moves them to the library.
func (p *Pipeline) verifyAndMove(asin string, stagingDir string) error {
	// Glob for audio files in staging.
	matches, err := filepath.Glob(filepath.Join(stagingDir, "*.m4a"))
	if err != nil {
		return fmt.Errorf("globbing staging dir for %s: %w", asin, err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no M4A files found in staging for %s", asin)
	}

	// Verify each file.
	for _, f := range matches {
		if err := p.verifyFunc(f); err != nil {
			return fmt.Errorf("verifying %s: %w", filepath.Base(f), err)
		}
	}

	// Move to library.
	if err := MoveToLibrary(p.config.StagingDir, p.config.LibraryDir, asin); err != nil {
		return fmt.Errorf("moving %s to library: %w", asin, err)
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
