package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration008CreatesListeningTables(t *testing.T) {
	db := setupTestDB(t)

	for _, table := range []string{"listening_days", "book_listening", "stats_sync_state"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		require.NoError(t, err, "table %s should exist", table)
		assert.Equal(t, table, name)
	}
}

// Re-running a backfill window must correct the stored total, not add to it.
func TestUpsertListeningDayReplacesRatherThanAccumulates(t *testing.T) {
	db := setupTestDB(t)

	require.NoError(t, UpsertListeningDay(db, "2026-01-15", "audible", 3600))
	require.NoError(t, UpsertListeningDay(db, "2026-01-15", "audible", 5400))

	days, err := ListListeningDays(db, "audible")
	require.NoError(t, err)
	require.Len(t, days, 1, "a repeated day must not create a second row")
	assert.Equal(t, 5400, days[0].Seconds, "the later value wins")
}

func TestListeningDaysAreScopedPerSource(t *testing.T) {
	db := setupTestDB(t)

	require.NoError(t, UpsertListeningDay(db, "2026-01-15", "audible", 3600))
	require.NoError(t, UpsertListeningDay(db, "2026-01-15", "abs", 1800))

	audible, err := ListListeningDays(db, "audible")
	require.NoError(t, err)
	require.Len(t, audible, 1)
	assert.Equal(t, 3600, audible[0].Seconds)

	abs, err := ListListeningDays(db, "abs")
	require.NoError(t, err)
	require.Len(t, abs, 1)
	assert.Equal(t, 1800, abs[0].Seconds,
		"the same day in two sources is two rows; totals are never merged in storage")

	all, err := ListListeningDays(db, "")
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestUpsertListeningDaysBatch(t *testing.T) {
	db := setupTestDB(t)

	batch := []ListeningDay{
		{Day: "2026-01-01", Source: "audible", Seconds: 100},
		{Day: "2026-01-02", Source: "audible", Seconds: 200},
		{Day: "2026-01-03", Source: "audible", Seconds: 300},
	}
	require.NoError(t, UpsertListeningDays(db, batch))

	n, err := CountListeningDays(db, "audible")
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	// Re-running the same batch with revised values stays at three rows.
	batch[0].Seconds = 999
	require.NoError(t, UpsertListeningDays(db, batch))
	n, err = CountListeningDays(db, "audible")
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	days, err := ListListeningDays(db, "audible")
	require.NoError(t, err)
	assert.Equal(t, 999, days[0].Seconds)
}

func TestUpsertListeningDaysEmptyIsNoop(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, UpsertListeningDays(db, nil))
	n, err := CountListeningDays(db, "audible")
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestUpsertBookListeningRoundTrips(t *testing.T) {
	db := setupTestDB(t)

	in := BookListening{
		Source:          "audible",
		SourceKey:       "ASIN001",
		ASIN:            "ASIN001",
		Title:           "A Test Title",
		Author:          "Author One",
		Narrator:        "Narrator One",
		Series:          "Test Series",
		SeriesPosition:  "2",
		Genres:          "Fantasy,Epic",
		RuntimeSeconds:  36000,
		PercentComplete: 98.5,
		IsFinished:      true,
		PurchaseDate:    "2025-01-02T03:04:05Z",
		AddedDate:       "2025-01-02T03:04:05Z",
		StatusChangedAt: "2026-01-06T17:20:37Z",
		StatusIsBulk:    false,
		LastPositionAt:  "2026-01-06T17:20:38Z",
		LastPositionMS:  123456,
		SecondsListened: 35000,
	}
	require.NoError(t, UpsertBookListening(db, in))

	got, err := ListBookListening(db, "audible")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, in, got[0])
}

// An enrichment pass that only knows metadata must not wipe a measured total
// that an earlier pass established.
func TestUpsertBookListeningPreservesMeasuredSeconds(t *testing.T) {
	db := setupTestDB(t)

	require.NoError(t, UpsertBookListening(db, BookListening{
		Source: "abs", SourceKey: "item-1", Title: "Original", SecondsListened: 7200,
	}))
	require.NoError(t, UpsertBookListening(db, BookListening{
		Source: "abs", SourceKey: "item-1", Title: "Enriched", Genres: "Fantasy",
		SecondsListened: 0,
	}))

	got, err := ListBookListening(db, "abs")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Enriched", got[0].Title, "metadata should update")
	assert.Equal(t, "Fantasy", got[0].Genres)
	assert.Equal(t, 7200, got[0].SecondsListened, "a measured total must survive a metadata-only pass")
}

func TestUpsertBookListeningOverwritesWithNewMeasurement(t *testing.T) {
	db := setupTestDB(t)

	require.NoError(t, UpsertBookListening(db, BookListening{
		Source: "abs", SourceKey: "item-1", SecondsListened: 7200,
	}))
	require.NoError(t, UpsertBookListening(db, BookListening{
		Source: "abs", SourceKey: "item-1", SecondsListened: 9000,
	}))

	got, err := ListBookListening(db, "abs")
	require.NoError(t, err)
	assert.Equal(t, 9000, got[0].SecondsListened)
}

func TestUpsertBookListeningBatchIsIdempotent(t *testing.T) {
	db := setupTestDB(t)

	books := []BookListening{
		{Source: "audible", SourceKey: "ASIN001", Title: "One"},
		{Source: "audible", SourceKey: "ASIN002", Title: "Two"},
	}
	require.NoError(t, UpsertBookListeningBatch(db, books))
	require.NoError(t, UpsertBookListeningBatch(db, books))

	got, err := ListBookListening(db, "audible")
	require.NoError(t, err)
	assert.Len(t, got, 2, "re-running a batch must not duplicate rows")
}

func TestUpsertBookListeningBatchEmptyIsNoop(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, UpsertBookListeningBatch(db, nil))
}

func TestMarkBulkStatusSetsAndClears(t *testing.T) {
	db := setupTestDB(t)

	require.NoError(t, UpsertBookListeningBatch(db, []BookListening{
		{Source: "audible", SourceKey: "A"},
		{Source: "audible", SourceKey: "B"},
		{Source: "audible", SourceKey: "C"},
	}))

	require.NoError(t, MarkBulkStatus(db, "audible", []string{"A", "B"}))
	got, err := ListBookListening(db, "audible")
	require.NoError(t, err)
	assert.True(t, got[0].StatusIsBulk)
	assert.True(t, got[1].StatusIsBulk)
	assert.False(t, got[2].StatusIsBulk)

	// Recomputation drops B from the cluster; its flag must be cleared, not
	// left behind from the previous run.
	require.NoError(t, MarkBulkStatus(db, "audible", []string{"A"}))
	got, err = ListBookListening(db, "audible")
	require.NoError(t, err)
	assert.True(t, got[0].StatusIsBulk)
	assert.False(t, got[1].StatusIsBulk, "a stale bulk flag must be cleared on recomputation")
}

func TestMarkBulkStatusEmptyClearsAll(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, UpsertBookListening(db, BookListening{
		Source: "audible", SourceKey: "A", StatusIsBulk: true,
	}))

	require.NoError(t, MarkBulkStatus(db, "audible", nil))
	got, err := ListBookListening(db, "audible")
	require.NoError(t, err)
	assert.False(t, got[0].StatusIsBulk)
}

func TestMarkBulkStatusChunksLargeKeySets(t *testing.T) {
	db := setupTestDB(t)

	// Exceed the internal chunk size to exercise the batching path.
	var books []BookListening
	var keys []string
	for i := 0; i < 950; i++ {
		key := "K" + time.Duration(i).String()
		books = append(books, BookListening{Source: "audible", SourceKey: key})
		keys = append(keys, key)
	}
	require.NoError(t, UpsertBookListeningBatch(db, books))
	require.NoError(t, MarkBulkStatus(db, "audible", keys))

	var flagged int
	err := db.QueryRow(`SELECT COUNT(*) FROM book_listening WHERE status_is_bulk = 1`).Scan(&flagged)
	require.NoError(t, err)
	assert.Equal(t, 950, flagged)
}

func TestMarkBulkStatusIsScopedToOneSource(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, UpsertBookListeningBatch(db, []BookListening{
		{Source: "audible", SourceKey: "A", StatusIsBulk: true},
		{Source: "abs", SourceKey: "A", StatusIsBulk: true},
	}))

	require.NoError(t, MarkBulkStatus(db, "audible", nil))

	abs, err := ListBookListening(db, "abs")
	require.NoError(t, err)
	assert.True(t, abs[0].StatusIsBulk, "clearing one source must not touch another")
}

func TestSyncStateRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	_, ok, err := GetSyncState(db, "missing")
	require.NoError(t, err)
	assert.False(t, ok, "an absent key reports not-found rather than erroring")

	require.NoError(t, SetSyncState(db, "audible.daily.through", "2026-01-31"))
	v, ok, err := GetSyncState(db, "audible.daily.through")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "2026-01-31", v)

	require.NoError(t, SetSyncState(db, "audible.daily.through", "2026-02-28"))
	v, _, err = GetSyncState(db, "audible.daily.through")
	require.NoError(t, err)
	assert.Equal(t, "2026-02-28", v)
}

func TestSyncTimeRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	want := time.Date(2026, 9, 20, 12, 30, 45, 0, time.UTC)
	require.NoError(t, SetSyncTime(db, "abs.sessions.watermark", want))

	got, ok, err := GetSyncTime(db, "abs.sessions.watermark")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, want.Equal(got), "want %v got %v", want, got)
}

// A corrupt watermark must degrade to a full backfill, never to syncing from
// an arbitrary date.
func TestGetSyncTimeTreatsUnparseableValueAsAbsent(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, SetSyncState(db, "abs.sessions.watermark", "not-a-timestamp"))

	got, ok, err := GetSyncTime(db, "abs.sessions.watermark")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.True(t, got.IsZero())
}

// closedDB returns a database handle that has already been closed, so every
// query fails. This exercises the error branches that are otherwise
// unreachable without a broken filesystem.
func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, database.Close())
	return database
}

func TestListeningWritesReportDatabaseErrors(t *testing.T) {
	bad := closedDB(t)

	assert.Error(t, UpsertListeningDay(bad, "2026-01-01", "audible", 1))
	assert.Error(t, UpsertListeningDays(bad, []ListeningDay{{Day: "2026-01-01", Source: "audible"}}))
	assert.Error(t, UpsertBookListening(bad, BookListening{Source: "audible", SourceKey: "A"}))
	assert.Error(t, UpsertBookListeningBatch(bad, []BookListening{{Source: "audible", SourceKey: "A"}}))
	assert.Error(t, MarkBulkStatus(bad, "audible", []string{"A"}))
	assert.Error(t, SetSyncState(bad, "k", "v"))
	assert.Error(t, SetSyncTime(bad, "k", time.Now()))
}

func TestListeningReadsReportDatabaseErrors(t *testing.T) {
	bad := closedDB(t)

	_, err := ListListeningDays(bad, "audible")
	assert.Error(t, err)

	_, err = CountListeningDays(bad, "audible")
	assert.Error(t, err)

	_, err = ListBookListening(bad, "audible")
	assert.Error(t, err)

	_, _, err = GetSyncState(bad, "k")
	assert.Error(t, err)

	_, _, err = GetSyncTime(bad, "k")
	assert.Error(t, err)
}

// A scan failure must surface rather than yielding a half-populated slice.
func TestListBookListeningReportsScanErrors(t *testing.T) {
	database := setupTestDB(t)
	// Write a value the scanner cannot coerce into the destination type.
	_, err := database.Exec(
		`INSERT INTO book_listening (source, source_key, last_position_ms) VALUES ('audible','A','not-a-number')`)
	require.NoError(t, err)

	_, err = ListBookListening(database, "audible")
	assert.Error(t, err)
}

func TestMigration009CreatesSessionsTable(t *testing.T) {
	database := setupTestDB(t)
	var name string
	err := database.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='listening_sessions'`).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "listening_sessions", name)
}

func sampleDBSession(id, day string, seconds int) ListeningSession {
	return ListeningSession{
		ID:              id,
		UserID:          "user-1",
		LibraryItemID:   "item-1",
		MediaType:       "book",
		Title:           "A Test Title",
		Author:          "An Author",
		Day:             day,
		Seconds:         seconds,
		DurationSeconds: 36000,
		StartedAt:       day + "T10:00:00Z",
		UpdatedAt:       day + "T11:00:00Z",
		Device:          "Test Client / Test OS",
	}
}

func TestUpsertListeningSessionsRoundTrips(t *testing.T) {
	database := setupTestDB(t)

	in := sampleDBSession("s1", "2026-03-01", 1800)
	require.NoError(t, UpsertListeningSessions(database, []ListeningSession{in}))

	got, err := ListListeningSessions(database)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, in, got[0])
}

// Sessions stay open and keep accumulating time, so re-observing one must
// update it rather than insert a second row.
func TestUpsertListeningSessionsUpdatesInPlace(t *testing.T) {
	database := setupTestDB(t)

	require.NoError(t, UpsertListeningSessions(database,
		[]ListeningSession{sampleDBSession("s1", "2026-03-01", 1800)}))
	require.NoError(t, UpsertListeningSessions(database,
		[]ListeningSession{sampleDBSession("s1", "2026-03-01", 3600)}))

	n, err := CountListeningSessions(database)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := ListListeningSessions(database)
	require.NoError(t, err)
	assert.Equal(t, 3600, got[0].Seconds, "the growing session's later value wins")
}

func TestSessionsByDayAggregates(t *testing.T) {
	database := setupTestDB(t)

	require.NoError(t, UpsertListeningSessions(database, []ListeningSession{
		sampleDBSession("s1", "2026-03-01", 1800),
		sampleDBSession("s2", "2026-03-01", 900),
		sampleDBSession("s3", "2026-03-02", 600),
	}))

	byDay, err := SessionsByDay(database)
	require.NoError(t, err)
	assert.Equal(t, 2700, byDay["2026-03-01"])
	assert.Equal(t, 600, byDay["2026-03-02"])
}

func TestSessionsByDayIgnoresUnbucketedSessions(t *testing.T) {
	database := setupTestDB(t)

	s := sampleDBSession("s1", "", 1800)
	require.NoError(t, UpsertListeningSessions(database, []ListeningSession{s}))

	byDay, err := SessionsByDay(database)
	require.NoError(t, err)
	assert.Empty(t, byDay, "a session with no day bucket contributes to no day")
}

func TestUpsertListeningSessionsEmptyIsNoop(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, UpsertListeningSessions(database, nil))

	n, err := CountListeningSessions(database)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestSessionOperationsReportDatabaseErrors(t *testing.T) {
	bad := closedDB(t)

	assert.Error(t, UpsertListeningSessions(bad, []ListeningSession{sampleDBSession("s1", "2026-03-01", 1)}))

	_, err := ListListeningSessions(bad)
	assert.Error(t, err)

	_, err = SessionsByDay(bad)
	assert.Error(t, err)

	_, err = CountListeningSessions(bad)
	assert.Error(t, err)
}

func TestListListeningSessionsReportsScanErrors(t *testing.T) {
	database := setupTestDB(t)
	_, err := database.Exec(
		`INSERT INTO listening_sessions (id, seconds) VALUES ('s1', 'not-a-number')`)
	require.NoError(t, err)

	_, err = ListListeningSessions(database)
	assert.Error(t, err)
}
