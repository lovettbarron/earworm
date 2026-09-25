package stats

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/komga"
	"github.com/lovettbarron/earworm/internal/listening"
)

// fakeKomgaClient is a scripted KomgaSource.
type fakeKomgaClient struct {
	books []komga.Book
	err   error
}

func (f *fakeKomgaClient) BooksWithProgress(context.Context) ([]komga.Book, error) {
	return f.books, f.err
}

var komgaNow = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func newKomgaIngestor(t *testing.T, database *sql.DB, client KomgaSource) *KomgaIngestor {
	t.Helper()
	bucket, err := listening.NewBucketer("UTC")
	require.NoError(t, err)
	return &KomgaIngestor{
		DB:     database,
		Client: client,
		Bucket: bucket,
		Clock:  listening.FixedClock{T: komgaNow},
	}
}

func komgaBook(id, series string, completed bool, readAt time.Time, page, pages int) komga.Book {
	b := komga.Book{
		ID:          id,
		SeriesTitle: series,
		Name:        series + " " + id,
		Number:      1,
		Progress: &komga.ReadProgress{
			Page:      page,
			Completed: completed,
			ReadDate:  readAt,
			Created:   readAt,
		},
	}
	b.Media.PagesCount = pages
	return b
}

func komgaRows(t *testing.T, database *sql.DB) map[string]db.BookListening {
	t.Helper()
	rows, err := db.ListBookListening(database, listening.SourceKomga)
	require.NoError(t, err)
	out := make(map[string]db.BookListening, len(rows))
	for _, r := range rows {
		out[r.SourceKey] = r
	}
	return out
}

func TestKomgaSyncStoresBooks(t *testing.T) {
	database := setupDB(t)
	read := time.Date(2026, 8, 3, 21, 15, 0, 0, time.UTC)

	done := komgaBook("b1", "Gantz (2018-2023) (Digital) (1r0n)", true, read, 180, 200)
	done.Metadata.Title = "Gantz v01 (Digital)"
	done.Metadata.Tags = []string{"action", "horror"}
	done.Metadata.Authors = append(done.Metadata.Authors, struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}{Name: "Hiroya Oku", Role: "writer"})
	done.Progress.Created = read.Add(-25 * time.Minute)

	client := &fakeKomgaClient{books: []komga.Book{
		done,
		komgaBook("b2", "Gantz", false, read.Add(24*time.Hour), 50, 200),
		{ID: "b3"}, // no progress: skipped
	}}

	res, err := newKomgaIngestor(t, database, client).Sync(context.Background())
	require.NoError(t, err)

	assert.Equal(t, KomgaResult{Books: 3, Completed: 1, InProgress: 1, Series: 1, Days: 2}, res)

	rows := komgaRows(t, database)
	require.Len(t, rows, 2)

	r := rows["b1"]
	assert.Equal(t, "Gantz v01", r.Title)
	assert.Equal(t, "Gantz", r.Series)
	assert.Equal(t, "1", r.SeriesPosition)
	assert.Equal(t, "Hiroya Oku", r.Author)
	assert.Equal(t, "action,horror", r.Genres)
	assert.True(t, r.IsFinished)
	assert.Equal(t, 100.0, r.PercentComplete, "completed is 100% whatever the page")
	assert.Equal(t, read.Format(time.RFC3339Nano), r.StatusChangedAt)
	assert.Equal(t, r.StatusChangedAt, r.LastPositionAt)
	assert.Equal(t, read.Add(-25*time.Minute).Format(time.RFC3339Nano), r.AddedDate)
	assert.Equal(t, 25*60, r.SecondsListened, "span recorded")
	assert.Equal(t, int64(180), r.LastPositionMS, "page stored as position")
	assert.False(t, r.StatusIsBulk)

	assert.Equal(t, 25.0, rows["b2"].PercentComplete, "derived from page / pagesCount")
	assert.False(t, rows["b2"].IsFinished)

	synced, ok, err := db.GetSyncTime(database, keyKomgaSyncedAt)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, synced.Equal(komgaNow))
}

func TestKomgaSyncPercentEdgeCases(t *testing.T) {
	database := setupDB(t)
	read := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	client := &fakeKomgaClient{books: []komga.Book{
		komgaBook("over", "S", false, read, 250, 200), // page past the end
		komgaBook("nopages", "S", false, read, 10, 0), // unknown page count
		komgaBook("donezero", "S", true, read, 0, 0),  // completed with no pages
	}}

	_, err := newKomgaIngestor(t, database, client).Sync(context.Background())
	require.NoError(t, err)

	rows := komgaRows(t, database)
	assert.Equal(t, 100.0, rows["over"].PercentComplete, "capped at 100")
	assert.Equal(t, 0.0, rows["nopages"].PercentComplete)
	assert.Equal(t, 100.0, rows["donezero"].PercentComplete)
}

func TestKomgaSyncRecordsSpanOnlyWhenPlausible(t *testing.T) {
	database := setupDB(t)
	read := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	instant := komgaBook("instant", "S", true, read, 1, 1)
	long := komgaBook("long", "S", true, read, 1, 1)
	long.Progress.Created = read.Add(-13 * time.Hour)
	undated := komgaBook("undated", "S", false, time.Time{}, 1, 10)
	undated.Progress.LastModified = time.Time{}

	client := &fakeKomgaClient{books: []komga.Book{instant, long, undated}}
	res, err := newKomgaIngestor(t, database, client).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Days, "an undated book adds no day")

	rows := komgaRows(t, database)
	assert.Zero(t, rows["instant"].SecondsListened, "zero span is not recorded")
	assert.Zero(t, rows["long"].SecondsListened, "a span over 12h is not reading")
	assert.Empty(t, rows["undated"].StatusChangedAt)
	assert.Empty(t, rows["undated"].AddedDate)
}

func TestKomgaSyncFlagsCompletionsBeforeCutoff(t *testing.T) {
	database := setupDB(t)
	cutoff := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	client := &fakeKomgaClient{books: []komga.Book{
		komgaBook("before", "Old", true, cutoff.Add(-time.Hour), 1, 1),
		komgaBook("at", "Old", true, cutoff, 1, 1),
		komgaBook("after", "New", true, cutoff.Add(time.Hour), 1, 1),
	}}
	ing := newKomgaIngestor(t, database, client)
	ing.UnreliableBefore = cutoff

	res, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, res.Unreliable)
	assert.Equal(t, 2, res.Series, "series counts every book, flagged or not")
	assert.Equal(t, 1, res.Days, "flagged completions add no reading days")

	rows := komgaRows(t, database)
	assert.True(t, rows["before"].StatusIsBulk)
	assert.True(t, rows["at"].StatusIsBulk, "the cutoff itself is inclusive")
	assert.False(t, rows["after"].StatusIsBulk)

	// Moving the cutoff earlier must clear flags that no longer apply.
	ing.UnreliableBefore = cutoff.Add(-2 * time.Hour)
	res, err = ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, res.Unreliable)

	rows = komgaRows(t, database)
	for id, r := range rows {
		assert.False(t, r.StatusIsBulk, id)
	}
}

func TestKomgaSyncWithoutCutoffFlagsNothing(t *testing.T) {
	database := setupDB(t)
	client := &fakeKomgaClient{books: []komga.Book{
		komgaBook("a", "S", true, time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), 1, 1),
	}}
	res, err := newKomgaIngestor(t, database, client).Sync(context.Background())
	require.NoError(t, err)
	assert.Zero(t, res.Unreliable)
	assert.False(t, komgaRows(t, database)["a"].StatusIsBulk)
}

func TestKomgaSyncIsIdempotent(t *testing.T) {
	database := setupDB(t)
	read := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	client := &fakeKomgaClient{books: []komga.Book{
		komgaBook("a", "S", true, read, 1, 1),
		komgaBook("b", "S", false, read, 5, 10),
	}}
	ing := newKomgaIngestor(t, database, client)
	ing.UnreliableBefore = read

	first, err := ing.Sync(context.Background())
	require.NoError(t, err)
	before := komgaRows(t, database)

	second, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, before, komgaRows(t, database))
}

func TestKomgaSyncWithNoBooksIsHarmless(t *testing.T) {
	database := setupDB(t)
	res, err := newKomgaIngestor(t, database, &fakeKomgaClient{}).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, KomgaResult{}, res)
	assert.Empty(t, komgaRows(t, database))
}

func TestKomgaSyncPropagatesErrors(t *testing.T) {
	t.Run("client", func(t *testing.T) {
		database := setupDB(t)
		client := &fakeKomgaClient{err: errors.New("unreachable")}
		_, err := newKomgaIngestor(t, database, client).Sync(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "komga books: unreachable")
	})
	t.Run("database", func(t *testing.T) {
		database := setupDB(t)
		client := &fakeKomgaClient{books: []komga.Book{
			komgaBook("a", "S", true, time.Now(), 1, 1),
		}}
		ing := newKomgaIngestor(t, database, client)
		require.NoError(t, database.Close())
		_, err := ing.Sync(context.Background())
		require.Error(t, err)
	})
}

func TestKomgaIngestorDefaults(t *testing.T) {
	k := &KomgaIngestor{}
	assert.NotNil(t, k.logger())
	assert.IsType(t, listening.SystemClock{}, k.clock())
}

// The defect behind the 2026-09-22 journal flood: a Komga library re-import
// changed every book id, the old books stopped appearing in the response, and
// recomputing flags from the response alone cleared the flag on the whole
// stored migration cluster. Their 2024 dates then read as genuine reading.
func TestKomgaSyncKeepsFlagsOnBooksThatLeaveTheSource(t *testing.T) {
	database := setupDB(t)
	cutoff := time.Date(2024, 3, 13, 23, 59, 59, 0, time.UTC)
	migrated := time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)

	client := &fakeKomgaClient{books: []komga.Book{
		komgaBook("old-1", "Solo Leveling", true, migrated, 1, 1),
		komgaBook("old-2", "Solo Leveling", true, migrated, 1, 1),
	}}
	ing := newKomgaIngestor(t, database, client)
	ing.UnreliableBefore = cutoff

	res, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, res.Unreliable)

	// The re-import: new ids, and the old ones now 404, so the client no
	// longer lists them at all.
	client.books = []komga.Book{
		komgaBook("new-1", "Solo Leveling", true, time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC), 1, 1),
	}
	res, err = ing.Sync(context.Background())
	require.NoError(t, err)

	rows := komgaRows(t, database)
	require.Len(t, rows, 3, "the stale rows stay in the table")
	assert.True(t, rows["old-1"].StatusIsBulk, "a book that vanished is still migration data")
	assert.True(t, rows["old-2"].StatusIsBulk)
	assert.False(t, rows["new-1"].StatusIsBulk, "re-imported after the cutoff")
	assert.Equal(t, 2, res.Unreliable, "the count covers every flagged row, not just this response")
}

// Flags are stored state, so a run that returns nothing must not clear them.
func TestKomgaSyncWithEmptyResponseKeepsStoredFlags(t *testing.T) {
	database := setupDB(t)
	cutoff := time.Date(2024, 3, 13, 0, 0, 0, 0, time.UTC)
	client := &fakeKomgaClient{books: []komga.Book{
		komgaBook("a", "S", true, cutoff.Add(-48*time.Hour), 1, 1),
	}}
	ing := newKomgaIngestor(t, database, client)
	ing.UnreliableBefore = cutoff
	_, err := ing.Sync(context.Background())
	require.NoError(t, err)

	client.books = nil
	res, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Unreliable)
	assert.True(t, komgaRows(t, database)["a"].StatusIsBulk)
}
