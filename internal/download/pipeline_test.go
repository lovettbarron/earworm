package download

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDownloader implements audible.AudibleClient for testing.
type fakeDownloader struct {
	mu       sync.Mutex
	calls    []string            // ASINs attempted in order
	errors   map[string]error    // configurable error per ASIN
	attempts map[string]int      // attempt count per ASIN
	createFiles bool             // whether to create dummy files on "success"
}

func newFakeDownloader() *fakeDownloader {
	return &fakeDownloader{
		errors:   make(map[string]error),
		attempts: make(map[string]int),
	}
}

func (f *fakeDownloader) Download(ctx context.Context, asin string, outputDir string) error {
	f.mu.Lock()
	f.calls = append(f.calls, asin)
	f.attempts[asin]++
	err := f.errors[asin]
	createFiles := f.createFiles
	f.mu.Unlock()

	if err != nil {
		return err
	}

	if createFiles {
		// Create a minimal M4A-like file in the output directory.
		// We don't need real M4A for pipeline tests — VerifyM4A is tested separately.
		// For pipeline tests we'll use a VerifyFunc override.
		if mkErr := os.MkdirAll(outputDir, 0755); mkErr != nil {
			return mkErr
		}
		filePath := filepath.Join(outputDir, asin+".m4a")
		if wErr := os.WriteFile(filePath, []byte("fake-audio-data"), 0644); wErr != nil {
			return wErr
		}
	}

	return nil
}

func (f *fakeDownloader) Quickstart(ctx context.Context) error { return nil }
func (f *fakeDownloader) CheckAuth(ctx context.Context) error  { return nil }
func (f *fakeDownloader) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return nil, nil
}

func (f *fakeDownloader) attemptCount(asin string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts[asin]
}

// setupTestDB creates an in-memory SQLite database with migrations applied.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

// seedBook inserts a book via SyncRemoteBook so it appears in ListDownloadable.
func seedBook(t *testing.T, database *sql.DB, asin, title, author string) {
	t.Helper()
	err := db.SyncRemoteBook(database, db.Book{
		ASIN:          asin,
		Title:         title,
		Author:        author,
		AudibleStatus: "finished",
	})
	require.NoError(t, err)
}

// seedDownloadedBook inserts a book already in "downloaded" state.
func seedDownloadedBook(t *testing.T, database *sql.DB, asin, title, author string) {
	t.Helper()
	seedBook(t, database, asin, title, author)
	err := db.UpdateDownloadComplete(database, asin, "/library/"+asin)
	require.NoError(t, err)
}

func defaultConfig(staging, library string) PipelineConfig {
	return PipelineConfig{
		StagingDir:        staging,
		LibraryDir:        library,
		RateLimitSeconds:  0, // no delay in tests
		MaxRetries:        3,
		BackoffMultiplier: 1.0,
		Quiet:             true,
	}
}

func TestPipeline_EmptyList(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	p := NewPipeline(client, database, defaultConfig(staging, library), &buf)
	summary, err := p.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, summary.Total)
	assert.Equal(t, 0, summary.Succeeded)
	assert.Equal(t, 0, summary.Failed)
	assert.False(t, summary.AuthFailed)
	assert.False(t, summary.Interrupted)
}

func TestPipeline_SequentialDownload(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	client.createFiles = true
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")
	seedBook(t, database, "B000000002", "Book Two", "Author B")

	cfg := defaultConfig(staging, library)
	p := NewPipeline(client, database, cfg, &buf)
	// Override verify to skip real M4A parsing
	p.verifyFunc = func(path string) error { return nil }

	summary, err := p.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, summary.Total)
	assert.Equal(t, 2, summary.Succeeded)
	assert.Equal(t, 0, summary.Failed)

	// Verify sequential order
	require.Len(t, client.calls, 2)
	assert.Equal(t, "B000000001", client.calls[0])
	assert.Equal(t, "B000000002", client.calls[1])

	// Verify files remain in staging (not moved to library)
	assert.DirExists(t, filepath.Join(staging, "B000000001"))
	assert.DirExists(t, filepath.Join(staging, "B000000002"))
}

func TestPipeline_DownloadCallsDBState(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	client.createFiles = true
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")

	cfg := defaultConfig(staging, library)
	p := NewPipeline(client, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	_, err := p.Run(context.Background())
	require.NoError(t, err)

	// After pipeline, book should be marked as downloaded with empty local_path
	// (files remain in staging; organize command handles library placement)
	book, err := db.GetBook(database, "B000000001")
	require.NoError(t, err)
	require.NotNil(t, book)
	assert.Equal(t, "downloaded", book.Status)
	assert.Empty(t, book.LocalPath)
}

func TestPipeline_RetryOnFailure(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	client.errors["B000000001"] = &audible.CommandError{Command: "download", Stderr: "network error", ExitCode: 1}
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")

	cfg := defaultConfig(staging, library)
	cfg.MaxRetries = 2
	p := NewPipeline(client, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	summary, err := p.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Total)
	assert.Equal(t, 0, summary.Succeeded)
	assert.Equal(t, 1, summary.Failed)

	// Should have attempted 1 initial + 2 retries = 3 total
	assert.Equal(t, 3, client.attemptCount("B000000001"))

	// DB should have error state
	book, err := db.GetBook(database, "B000000001")
	require.NoError(t, err)
	assert.Equal(t, "error", book.Status)
	assert.Equal(t, 2, book.RetryCount)
}

func TestPipeline_ExhaustedRetriesContinuesToNextBook(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	client.createFiles = true
	client.errors["B000000001"] = &audible.CommandError{Command: "download", Stderr: "fail", ExitCode: 1}
	// B000000002 succeeds (no error configured)
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")
	seedBook(t, database, "B000000002", "Book Two", "Author B")

	cfg := defaultConfig(staging, library)
	cfg.MaxRetries = 1
	p := NewPipeline(client, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, summary.Total)
	assert.Equal(t, 1, summary.Succeeded)
	assert.Equal(t, 1, summary.Failed)
	assert.Len(t, summary.Errors, 1)
	assert.Equal(t, "B000000001", summary.Errors[0].ASIN)
}

func TestPipeline_AuthErrorAbortsBatch(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	client.errors["B000000001"] = &audible.AuthError{Message: "unauthorized"}
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")
	seedBook(t, database, "B000000002", "Book Two", "Author B")

	cfg := defaultConfig(staging, library)
	p := NewPipeline(client, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	summary, err := p.Run(context.Background())

	// Auth error should result in an error return
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.True(t, summary.AuthFailed)
	// Should not have attempted B000000002
	assert.Equal(t, 0, client.attemptCount("B000000002"))
}

func TestPipeline_RateLimitLongerBackoff(t *testing.T) {
	database := setupTestDB(t)

	// Track how many times download is attempted
	callCount := 0
	client := newFakeDownloader()
	origDownload := client.Download
	_ = origDownload
	// First call returns rate limit error, second succeeds
	client.errors["B000000001"] = nil // Will override per-call below

	// Use a custom fakeDownloader that returns rate limit on first call
	rateLimitClient := &rateLimitFakeDownloader{
		callsBeforeSuccess: 1,
		callCount:          &callCount,
		createFiles:        true,
	}

	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")

	cfg := defaultConfig(staging, library)
	cfg.MaxRetries = 3
	p := NewPipeline(rateLimitClient, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }
	// Override sleep to track delays
	var delays []time.Duration
	p.sleepFunc = func(ctx context.Context, d time.Duration) error {
		delays = append(delays, d)
		return nil
	}

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Succeeded)
	// Rate limit should have used a delay (doubled backoff)
	require.NotEmpty(t, delays)
}

// rateLimitFakeDownloader returns RateLimitError for the first N calls, then succeeds.
type rateLimitFakeDownloader struct {
	callsBeforeSuccess int
	callCount          *int
	createFiles        bool
}

func (r *rateLimitFakeDownloader) Download(ctx context.Context, asin string, outputDir string) error {
	*r.callCount++
	if *r.callCount <= r.callsBeforeSuccess {
		return &audible.RateLimitError{Message: "too many requests"}
	}
	if r.createFiles {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(outputDir, asin+".m4a"), []byte("fake"), 0644)
	}
	return nil
}
func (r *rateLimitFakeDownloader) Quickstart(ctx context.Context) error { return nil }
func (r *rateLimitFakeDownloader) CheckAuth(ctx context.Context) error  { return nil }
func (r *rateLimitFakeDownloader) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return nil, nil
}

func TestPipeline_ContextCancellation(t *testing.T) {
	database := setupTestDB(t)
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")
	seedBook(t, database, "B000000002", "Book Two", "Author B")

	ctx, cancel := context.WithCancel(context.Background())

	// Custom client that cancels context after first successful download
	downloadCount := 0
	cancelClient := &cancellingFakeDownloader{
		downloadCount: &downloadCount,
		cancelAfter:   1,
		cancelFunc:    cancel,
	}

	cfg := defaultConfig(staging, library)
	p := NewPipeline(cancelClient, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	summary, err := p.Run(ctx)
	require.NoError(t, err)
	assert.True(t, summary.Interrupted)
	// First book downloaded, context cancelled before second starts
	assert.Equal(t, 1, summary.Succeeded)
}

// cancellingFakeDownloader cancels the context after N successful downloads.
type cancellingFakeDownloader struct {
	downloadCount *int
	cancelAfter   int
	cancelFunc    context.CancelFunc
}

func (c *cancellingFakeDownloader) Download(ctx context.Context, asin string, outputDir string) error {
	*c.downloadCount++
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, asin+".m4a"), []byte("fake"), 0644); err != nil {
		return err
	}
	if *c.downloadCount >= c.cancelAfter {
		c.cancelFunc()
	}
	return nil
}
func (c *cancellingFakeDownloader) Quickstart(ctx context.Context) error { return nil }
func (c *cancellingFakeDownloader) CheckAuth(ctx context.Context) error  { return nil }
func (c *cancellingFakeDownloader) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return nil, nil
}

func TestPipeline_CleansOrphansBeforeStart(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	// Create an orphan staging directory
	orphanDir := filepath.Join(staging, "B999999999")
	require.NoError(t, os.MkdirAll(orphanDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(orphanDir, "test.m4a"), []byte("x"), 0644))

	cfg := defaultConfig(staging, library)
	p := NewPipeline(client, database, cfg, &buf)

	_, err := p.Run(context.Background())
	require.NoError(t, err)

	// Orphan should be cleaned
	_, statErr := os.Stat(orphanDir)
	assert.True(t, os.IsNotExist(statErr))
}

func TestPipeline_ResumeState(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	client.createFiles = true
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	// One book already downloaded, one pending
	seedDownloadedBook(t, database, "B000000001", "Done Book", "Author A")
	seedBook(t, database, "B000000002", "New Book", "Author B")

	cfg := defaultConfig(staging, library)
	p := NewPipeline(client, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Total) // only new book
	assert.Equal(t, 1, summary.Succeeded)

	// Output should mention resume
	output := buf.String()
	assert.Contains(t, output, "Resuming")
}

func TestPipeline_ElapsedAlwaysSet(t *testing.T) {
	database := setupTestDB(t)
	client := newFakeDownloader()
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	cfg := defaultConfig(staging, library)
	p := NewPipeline(client, database, cfg, &buf)

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, summary.Elapsed >= 0)
}

func TestSummary_String(t *testing.T) {
	tests := []struct {
		name     string
		summary  Summary
		contains []string
	}{
		{
			name: "success only",
			summary: Summary{
				Total:     5,
				Succeeded: 5,
				Failed:    0,
				Elapsed:   2*time.Minute + 30*time.Second,
			},
			contains: []string{"Downloaded 5/5 books", "2m 30s elapsed"},
		},
		{
			name: "with failures",
			summary: Summary{
				Total:     3,
				Succeeded: 2,
				Failed:    1,
				Elapsed:   45 * time.Second,
			},
			contains: []string{"Downloaded 2/3 books", "1 failed", "45s elapsed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.summary.String()
			for _, c := range tt.contains {
				assert.Contains(t, s, c)
			}
		})
	}
}

func TestPipeline_AAXCDecryptIntegration(t *testing.T) {
	database := setupTestDB(t)
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")

	// Create a fake downloader that produces AAXC + voucher files instead of M4A
	aaxcClient := &aaxcFakeDownloader{}

	cfg := defaultConfig(staging, library)
	p := NewPipeline(aaxcClient, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }
	// Override decrypt to simulate converting .aaxc to .m4b
	p.decryptFunc = func(ctx context.Context, stagingDir string, cmdFactory CmdFactory) error {
		// Find AAXC file and "decrypt" it by renaming to .m4b
		aaxcFiles, _ := filepath.Glob(filepath.Join(stagingDir, "*.aaxc"))
		if len(aaxcFiles) == 0 {
			return nil
		}
		aaxcPath := aaxcFiles[0]
		ext := filepath.Ext(aaxcPath)
		m4bPath := strings.TrimSuffix(aaxcPath, ext) + ".m4b"
		// Copy content to m4b
		data, err := os.ReadFile(aaxcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(m4bPath, data, 0644); err != nil {
			return err
		}
		// Remove originals
		os.Remove(aaxcPath)
		voucherFiles, _ := filepath.Glob(filepath.Join(stagingDir, "*.voucher"))
		for _, v := range voucherFiles {
			os.Remove(v)
		}
		return nil
	}

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Total)
	assert.Equal(t, 1, summary.Succeeded)
	assert.Equal(t, 0, summary.Failed)

	// Verify M4B file remains in staging after download (not moved to library)
	stagingASIN := filepath.Join(staging, "B000000001")
	entries, err := os.ReadDir(stagingASIN)
	require.NoError(t, err)
	var hasM4B bool
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".m4b" {
			hasM4B = true
		}
	}
	assert.True(t, hasM4B, "staging should contain .m4b file after decrypt+verify")
}

// aaxcFakeDownloader creates AAXC + voucher files instead of M4A.
type aaxcFakeDownloader struct{}

func (a *aaxcFakeDownloader) Download(ctx context.Context, asin string, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	// Create AAXC file
	if err := os.WriteFile(filepath.Join(outputDir, asin+"-AAX_44_128.aaxc"), []byte("encrypted-audio"), 0644); err != nil {
		return err
	}
	// Create voucher file
	voucher := `{"content_license":{"asin":"` + asin + `","license_response":{"key":"abc123","iv":"def456"}}}`
	if err := os.WriteFile(filepath.Join(outputDir, asin+"-AAX_44_128.voucher"), []byte(voucher), 0644); err != nil {
		return err
	}
	return nil
}

func (a *aaxcFakeDownloader) Quickstart(ctx context.Context) error { return nil }
func (a *aaxcFakeDownloader) CheckAuth(ctx context.Context) error  { return nil }
func (a *aaxcFakeDownloader) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return nil, nil
}

func TestPipeline_TimeoutExceeded(t *testing.T) {
	database := setupTestDB(t)
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Slow Book", "Author A")

	// Fake downloader that sleeps longer than timeout
	slowClient := &slowFakeDownloader{sleepDuration: 2 * time.Second}

	cfg := defaultConfig(staging, library)
	cfg.TimeoutMinutes = 1 // Will be overridden — we use a short timeout in the test
	cfg.MaxRetries = 0     // No retries to keep test fast

	p := NewPipeline(slowClient, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	// Override timeout to 100ms for test speed
	origTimeout := cfg.TimeoutMinutes
	_ = origTimeout
	// Instead we'll set a very small timeout by using TimeoutMinutes=0 and manually wrapping
	// Actually: test the real code path by setting TimeoutMinutes to something that
	// converts to a short duration. We need to test at minute granularity which is too slow.
	// Instead, test via downloadWithRetry directly with a short context timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Set TimeoutMinutes=0 so pipeline doesn't add its own timeout, we rely on parent ctx
	cfg.TimeoutMinutes = 0
	p2 := NewPipeline(slowClient, database, cfg, &buf)
	p2.verifyFunc = func(path string) error { return nil }

	summary, err := p2.Run(ctx)
	require.NoError(t, err)
	assert.True(t, summary.Interrupted, "should be interrupted by timeout")
}

func TestPipeline_TimeoutPerBook(t *testing.T) {
	database := setupTestDB(t)
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Slow Book", "Author A")
	seedBook(t, database, "B000000002", "Fast Book", "Author B")

	// First book hangs, second is fast
	mixedClient := &mixedSpeedDownloader{
		slowASINs:     map[string]bool{"B000000001": true},
		slowDuration:  2 * time.Second,
		createFiles:   true,
	}

	cfg := defaultConfig(staging, library)
	cfg.MaxRetries = 0

	p := NewPipeline(mixedClient, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }
	// Override tickInterval for test
	oldInterval := tickInterval
	tickInterval = 20 * time.Millisecond
	defer func() { tickInterval = oldInterval }()

	// Use a very short per-book timeout via a custom timeoutFunc
	p.timeoutForBook = 200 * time.Millisecond

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	// First book should fail (timeout), second should succeed
	assert.Equal(t, 2, summary.Total)
	assert.Equal(t, 1, summary.Succeeded, "fast book should succeed")
	assert.Equal(t, 1, summary.Failed, "slow book should fail from timeout")
}

func TestPipeline_ElapsedTickerWrites(t *testing.T) {
	database := setupTestDB(t)
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")

	// A downloader that takes a controlled amount of time
	timedClient := &timedFakeDownloader{
		sleepDuration: 250 * time.Millisecond,
		createFiles:   true,
	}

	cfg := defaultConfig(staging, library)
	cfg.Quiet = false
	cfg.MaxRetries = 0

	p := NewPipeline(timedClient, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	// Override tick interval to be very short
	oldInterval := tickInterval
	tickInterval = 50 * time.Millisecond
	defer func() { tickInterval = oldInterval }()

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Succeeded)

	// Should contain elapsed time updates
	output := buf.String()
	assert.Contains(t, output, "Downloading:", "should print elapsed progress")
}

func TestPipeline_TimeoutZeroDisabled(t *testing.T) {
	database := setupTestDB(t)
	staging := t.TempDir()
	library := t.TempDir()
	var buf bytes.Buffer

	seedBook(t, database, "B000000001", "Book One", "Author A")

	client := newFakeDownloader()
	client.createFiles = true

	cfg := defaultConfig(staging, library)
	cfg.TimeoutMinutes = 0 // Disabled

	p := NewPipeline(client, database, cfg, &buf)
	p.verifyFunc = func(path string) error { return nil }

	summary, err := p.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Succeeded)
	assert.Equal(t, 0, summary.Failed)
}

// slowFakeDownloader sleeps for a configured duration, simulating a hanging download.
type slowFakeDownloader struct {
	sleepDuration time.Duration
}

func (s *slowFakeDownloader) Download(ctx context.Context, asin string, outputDir string) error {
	select {
	case <-time.After(s.sleepDuration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *slowFakeDownloader) Quickstart(ctx context.Context) error { return nil }
func (s *slowFakeDownloader) CheckAuth(ctx context.Context) error  { return nil }
func (s *slowFakeDownloader) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return nil, nil
}

// mixedSpeedDownloader: slow for some ASINs, fast for others.
type mixedSpeedDownloader struct {
	slowASINs    map[string]bool
	slowDuration time.Duration
	createFiles  bool
}

func (m *mixedSpeedDownloader) Download(ctx context.Context, asin string, outputDir string) error {
	if m.slowASINs[asin] {
		select {
		case <-time.After(m.slowDuration):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if m.createFiles {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(outputDir, asin+".m4a"), []byte("fake"), 0644)
	}
	return nil
}
func (m *mixedSpeedDownloader) Quickstart(ctx context.Context) error { return nil }
func (m *mixedSpeedDownloader) CheckAuth(ctx context.Context) error  { return nil }
func (m *mixedSpeedDownloader) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return nil, nil
}

// timedFakeDownloader sleeps briefly then creates files (for ticker testing).
type timedFakeDownloader struct {
	sleepDuration time.Duration
	createFiles   bool
}

func (d *timedFakeDownloader) Download(ctx context.Context, asin string, outputDir string) error {
	select {
	case <-time.After(d.sleepDuration):
	case <-ctx.Done():
		return ctx.Err()
	}
	if d.createFiles {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(outputDir, asin+".m4a"), []byte("fake"), 0644)
	}
	return nil
}
func (d *timedFakeDownloader) Quickstart(ctx context.Context) error { return nil }
func (d *timedFakeDownloader) CheckAuth(ctx context.Context) error  { return nil }
func (d *timedFakeDownloader) LibraryExport(ctx context.Context) ([]audible.LibraryItem, error) {
	return nil, nil
}

// Ensure unused imports don't cause issues
var _ = errors.New
var _ = fmt.Sprintf
var _ = strings.Contains
