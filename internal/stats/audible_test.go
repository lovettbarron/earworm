package stats

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

// fakeStatsClient is a scripted audible.StatsClient. Each call records its
// arguments so tests can assert on windowing and batching.
type fakeStatsClient struct {
	dailyCalls   []time.Time
	dailyByStart map[string]map[string]float64
	dailyErr     error

	monthlyCalls []time.Time

	finished    []audible.FinishedStatus
	finishedErr error

	library    []audible.LibraryListening
	libraryErr error

	positions     map[string]audible.LastPosition
	positionASINs []string
	positionsErr  error
}

func (f *fakeStatsClient) APIGet(context.Context, string, ...audible.APIParam) ([]byte, error) {
	return nil, fmt.Errorf("not used")
}

func (f *fakeStatsClient) DailyListening(_ context.Context, start time.Time, days int) (map[string]float64, error) {
	f.dailyCalls = append(f.dailyCalls, start)
	if f.dailyErr != nil {
		return nil, f.dailyErr
	}
	if days > audible.MaxDailyWindow {
		return nil, fmt.Errorf("window too large: %d", days)
	}
	if m, ok := f.dailyByStart[start.Format("2006-01-02")]; ok {
		return m, nil
	}
	return map[string]float64{}, nil
}

func (f *fakeStatsClient) MonthlyListening(_ context.Context, start time.Time, _ int) (map[string]float64, error) {
	f.monthlyCalls = append(f.monthlyCalls, start)
	return map[string]float64{}, nil
}

func (f *fakeStatsClient) FinishedStatuses(context.Context) ([]audible.FinishedStatus, error) {
	return f.finished, f.finishedErr
}

func (f *fakeStatsClient) LibraryListening(context.Context) ([]audible.LibraryListening, error) {
	return f.library, f.libraryErr
}

func (f *fakeStatsClient) LastPositions(_ context.Context, asins []string) (map[string]audible.LastPosition, error) {
	f.positionASINs = append(f.positionASINs, asins...)
	return f.positions, f.positionsErr
}

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func newIngestor(t *testing.T, database *sql.DB, client audible.StatsClient, now time.Time) *AudibleIngestor {
	t.Helper()
	bucket, err := listening.NewBucketer("UTC")
	require.NoError(t, err)
	return &AudibleIngestor{
		DB:            database,
		Client:        client,
		Bucket:        bucket,
		Clock:         listening.FixedClock{T: now},
		BackfillStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestBackfillDailyWalksWindowsAndStoresDays(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{
		dailyByStart: map[string]map[string]float64{
			"2026-01-01": {"2026-01-01": 3600, "2026-01-02": 1800},
			"2026-01-31": {"2026-02-01": 7200},
		},
	}
	ing := newIngestor(t, database, client, time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC))

	res, err := ing.BackfillDaily(context.Background())
	require.NoError(t, err)

	assert.False(t, res.Resumed)
	assert.Equal(t, 2, res.WindowsFetched, "Jan 1 + 30 days = Jan 31, then one more window")
	assert.Equal(t, 3, res.DaysStored)

	days, err := db.ListListeningDays(database, listening.SourceAudible)
	require.NoError(t, err)
	require.Len(t, days, 3)
	assert.Equal(t, "2026-01-01", days[0].Day)
	assert.Equal(t, 3600, days[0].Seconds)
}

func TestBackfillDailyNeverExceedsAPIWindowLimit(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{}
	ing := newIngestor(t, database, client, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))

	_, err := ing.BackfillDaily(context.Background())
	require.NoError(t, err)

	// Consecutive window starts must be exactly the API maximum apart.
	require.Greater(t, len(client.dailyCalls), 1)
	for i := 1; i < len(client.dailyCalls); i++ {
		gap := client.dailyCalls[i].Sub(client.dailyCalls[i-1]).Hours() / 24
		assert.InDelta(t, float64(audible.MaxDailyWindow), gap, 0.5)
	}
}

// An interrupted backfill must pick up near where it stopped rather than
// replaying the whole history.
func TestBackfillDailyResumesFromWatermark(t *testing.T) {
	database := setupDB(t)
	require.NoError(t, db.SetSyncState(database, keyDailyThrough, "2026-03-01"))

	client := &fakeStatsClient{}
	ing := newIngestor(t, database, client, time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))
	ing.BackfillStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	res, err := ing.BackfillDaily(context.Background())
	require.NoError(t, err)

	assert.True(t, res.Resumed)
	require.NotEmpty(t, client.dailyCalls)
	first := client.dailyCalls[0]
	assert.True(t, first.After(time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)),
		"resume should start near the watermark, not at the configured start; got %s", first)
}

// The tail of the previous run is re-fetched because Audible revises recent days.
func TestBackfillDailyRefetchesRecentDays(t *testing.T) {
	database := setupDB(t)
	require.NoError(t, db.SetSyncState(database, keyDailyThrough, "2026-03-10"))

	client := &fakeStatsClient{}
	ing := newIngestor(t, database, client, time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))

	_, err := ing.BackfillDaily(context.Background())
	require.NoError(t, err)

	require.NotEmpty(t, client.dailyCalls)
	want := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -recentRefetchDays)
	assert.Equal(t, want.Format("2006-01-02"), client.dailyCalls[0].Format("2006-01-02"))
}

func TestBackfillDailyIsIdempotent(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{
		dailyByStart: map[string]map[string]float64{
			"2026-01-01": {"2026-01-01": 3600, "2026-01-02": 1800},
		},
	}
	ing := newIngestor(t, database, client, time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC))

	_, err := ing.BackfillDaily(context.Background())
	require.NoError(t, err)
	_, err = ing.BackfillDaily(context.Background())
	require.NoError(t, err)

	n, err := db.CountListeningDays(database, listening.SourceAudible)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "a second run must not duplicate stored days")
}

func TestBackfillDailyRecordsWatermarkPerWindow(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{}
	ing := newIngestor(t, database, client, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC))

	_, err := ing.BackfillDaily(context.Background())
	require.NoError(t, err)

	through, ok, err := db.GetSyncState(database, keyDailyThrough)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "2026-03-15", through, "watermark should not run past today")
}

func TestBackfillDailyPropagatesClientError(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{dailyErr: fmt.Errorf("boom")}
	ing := newIngestor(t, database, client, time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC))

	_, err := ing.BackfillDaily(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestBackfillDailyHonoursContextCancellation(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{}
	ing := newIngestor(t, database, client, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ing.BackfillDaily(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBackfillDailyRequiresStartDate(t *testing.T) {
	database := setupDB(t)
	ing := newIngestor(t, database, &fakeStatsClient{}, time.Now())
	ing.BackfillStart = time.Time{}

	_, err := ing.BackfillDaily(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start date")
}

func TestSyncBooksStoresMetadataAndPositions(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{
		library: []audible.LibraryListening{{
			ASIN:            "ASIN001",
			Title:           "A Test Title",
			Authors:         []string{"Author One", "Author Two"},
			Narrators:       []string{"Narrator One"},
			Series:          "Test Series",
			SeriesPosition:  "3",
			Genres:          []string{"Fantasy"},
			RuntimeMinutes:  600,
			PercentComplete: 50,
			PurchaseDate:    "2025-01-01T00:00:00Z",
			DateAdded:       "2025-01-01T00:00:00Z",
		}},
		finished: []audible.FinishedStatus{{
			ASIN:       "ASIN001",
			EventTime:  time.Date(2026, 1, 6, 17, 20, 37, 0, time.UTC),
			IsFinished: true,
		}},
		positions: map[string]audible.LastPosition{
			"ASIN001": {ASIN: "ASIN001", Exists: true, PositionMS: 555, LastUpdated: time.Date(2026, 1, 6, 17, 20, 38, 0, time.UTC)},
		},
	}
	ing := newIngestor(t, database, client, time.Now())

	res, err := ing.SyncBooks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Books)
	assert.Equal(t, 1, res.PositionsFound)

	books, err := db.ListBookListening(database, listening.SourceAudible)
	require.NoError(t, err)
	require.Len(t, books, 1)

	b := books[0]
	assert.Equal(t, "ASIN001", b.SourceKey)
	assert.Equal(t, "Author One, Author Two", b.Author)
	assert.Equal(t, "Fantasy", b.Genres)
	assert.Equal(t, 36000, b.RuntimeSeconds, "runtime minutes convert to seconds")
	assert.True(t, b.IsFinished, "the dedicated status endpoint wins over the library field")
	assert.EqualValues(t, 555, b.LastPositionMS)
	assert.NotEmpty(t, b.StatusChangedAt)
	assert.False(t, b.StatusIsBulk)
}

// A mass-marking event must be flagged, not stored as evidence that those
// books were genuinely finished at that moment.
func TestSyncBooksFlagsBulkStatusClusters(t *testing.T) {
	database := setupDB(t)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var lib []audible.LibraryListening
	var fin []audible.FinishedStatus
	for i := 0; i < 20; i++ {
		asin := fmt.Sprintf("BULK%06d", i)
		lib = append(lib, audible.LibraryListening{ASIN: asin, Title: "Bulk Book"})
		fin = append(fin, audible.FinishedStatus{
			ASIN:       asin,
			EventTime:  base.Add(time.Duration(i) * time.Millisecond),
			IsFinished: true,
		})
	}
	// One genuine finish, well separated.
	lib = append(lib, audible.LibraryListening{ASIN: "REAL000001", Title: "Real Book"})
	fin = append(fin, audible.FinishedStatus{
		ASIN: "REAL000001", EventTime: base.AddDate(0, 0, 30), IsFinished: true,
	})

	client := &fakeStatsClient{library: lib, finished: fin, positions: map[string]audible.LastPosition{}}
	ing := newIngestor(t, database, client, time.Now())

	res, err := ing.SyncBooks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 20, res.BulkFlagged)

	books, err := db.ListBookListening(database, listening.SourceAudible)
	require.NoError(t, err)

	flagged, unflagged := 0, 0
	for _, b := range books {
		if b.StatusIsBulk {
			flagged++
		} else {
			unflagged++
		}
	}
	assert.Equal(t, 20, flagged)
	assert.Equal(t, 1, unflagged, "the isolated genuine finish must not be flagged")
}

func TestSyncBooksIsIdempotent(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{
		library:   []audible.LibraryListening{{ASIN: "ASIN001", Title: "One"}},
		positions: map[string]audible.LastPosition{},
	}
	ing := newIngestor(t, database, client, time.Now())

	_, err := ing.SyncBooks(context.Background())
	require.NoError(t, err)
	_, err = ing.SyncBooks(context.Background())
	require.NoError(t, err)

	books, err := db.ListBookListening(database, listening.SourceAudible)
	require.NoError(t, err)
	assert.Len(t, books, 1)
}

func TestSyncBooksOnlyRequestsPositionsForRealASINs(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{
		library: []audible.LibraryListening{
			{ASIN: "ASIN001"},
			{ASIN: ""}, // a library row without an ASIN
		},
		positions: map[string]audible.LastPosition{},
	}
	ing := newIngestor(t, database, client, time.Now())

	_, err := ing.SyncBooks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"ASIN001"}, client.positionASINs)
}

func TestSyncBooksPropagatesErrors(t *testing.T) {
	database := setupDB(t)

	t.Run("library", func(t *testing.T) {
		ing := newIngestor(t, database, &fakeStatsClient{libraryErr: fmt.Errorf("lib down")}, time.Now())
		_, err := ing.SyncBooks(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lib down")
	})

	t.Run("finished", func(t *testing.T) {
		ing := newIngestor(t, database, &fakeStatsClient{finishedErr: fmt.Errorf("status down")}, time.Now())
		_, err := ing.SyncBooks(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status down")
	})

	t.Run("positions", func(t *testing.T) {
		ing := newIngestor(t, database, &fakeStatsClient{
			library:      []audible.LibraryListening{{ASIN: "ASIN001"}},
			positionsErr: fmt.Errorf("positions down"),
		}, time.Now())
		_, err := ing.SyncBooks(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "positions down")
	})
}

// The limiter must actually be consulted, or a backfill would hammer the API.
func TestBackfillDailyPacesRequests(t *testing.T) {
	database := setupDB(t)
	client := &fakeStatsClient{}
	ing := newIngestor(t, database, client, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))

	counter := &countingWaiter{}
	ing.Limiter = counter

	res, err := ing.BackfillDaily(context.Background())
	require.NoError(t, err)
	assert.Equal(t, res.WindowsFetched-1, counter.n,
		"every window after the first should be paced")
}

type countingWaiter struct{ n int }

func (c *countingWaiter) Wait(context.Context) error { c.n++; return nil }
