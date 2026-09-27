package stats

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// The second half of the 2026-09-22 incident: the re-import re-marked 103 books
// as read at once, after the cutoff, so nothing flagged them and each produced
// a "Finished" entry. A run of completions sharing an instant is a re-mark, and
// the same detection Audible uses catches it.
func TestKomgaSyncFlagsMassReMarking(t *testing.T) {
	database := setupDB(t)
	remark := time.Date(2026, 9, 22, 12, 17, 40, 0, time.UTC)

	// A re-import re-marks whatever it restores, so one instant covers
	// unrelated series.
	var books []komga.Book
	for i := 0; i < 8; i++ {
		b := komgaBook(fmt.Sprintf("remark-%d", i), fmt.Sprintf("Series %d", i), true,
			remark.Add(time.Duration(i*100)*time.Millisecond), 1, 1)
		b.Progress.Created = b.Progress.ReadDate // instant write, no span
		books = append(books, b)
	}
	// Genuine reading the same day, hours away from the cluster.
	books = append(books, komgaBook("real", "Read Today", true, remark.Add(6*time.Hour), 1, 1))

	res, err := newKomgaIngestor(t, database, &fakeKomgaClient{books: books}).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 8, res.Unreliable, "the cluster is flagged with no cutoff configured")

	rows := komgaRows(t, database)
	assert.True(t, rows["remark-0"].StatusIsBulk)
	assert.True(t, rows["remark-7"].StatusIsBulk)
	assert.False(t, rows["real"].StatusIsBulk, "a book read hours from the cluster is genuine")
	assert.Equal(t, 1, res.Days, "only the genuine completion contributes a reading day")
}

// Reading a few volumes in an evening is ordinary and must survive.
func TestKomgaSyncKeepsGenuineSameEveningReading(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 9, 20, 19, 0, 0, 0, time.UTC)

	var books []komga.Book
	for i := 0; i < 6; i++ {
		books = append(books, komgaBook(fmt.Sprintf("vol-%d", i), "Evening", true,
			start.Add(time.Duration(i)*25*time.Minute), 1, 1))
	}

	res, err := newKomgaIngestor(t, database, &fakeKomgaClient{books: books}).Sync(context.Background())
	require.NoError(t, err)
	assert.Zero(t, res.Unreliable, "volumes minutes apart are reading, not a re-mark")
	for id, r := range komgaRows(t, database) {
		assert.False(t, r.StatusIsBulk, id)
	}
}

// The cutoff and the cluster rule are independent, and neither undoes the other.
func TestKomgaSyncCombinesCutoffAndClusterFlags(t *testing.T) {
	database := setupDB(t)
	cutoff := time.Date(2024, 3, 13, 23, 59, 59, 0, time.UTC)
	remark := time.Date(2026, 9, 22, 12, 17, 40, 0, time.UTC)

	books := []komga.Book{komgaBook("old", "Migrated", true, cutoff.Add(-72*time.Hour), 1, 1)}
	for i := 0; i < 6; i++ {
		books = append(books, komgaBook(fmt.Sprintf("remark-%d", i), fmt.Sprintf("Series %d", i), true,
			remark.Add(time.Duration(i*50)*time.Millisecond), 1, 1))
	}
	books = append(books, komgaBook("real", "Genuine", true, remark.Add(9*time.Hour), 1, 1))

	ing := newKomgaIngestor(t, database, &fakeKomgaClient{books: books})
	ing.UnreliableBefore = cutoff

	res, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 7, res.Unreliable, "one before the cutoff plus the six-book cluster")

	rows := komgaRows(t, database)
	assert.True(t, rows["old"].StatusIsBulk)
	assert.True(t, rows["remark-3"].StatusIsBulk)
	assert.False(t, rows["real"].StatusIsBulk)

	// Idempotent: re-running reports and stores the same thing.
	again, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, res, again)
	assert.Equal(t, rows, komgaRows(t, database))
}

// A cluster that stops being one must lose its flag, which is what makes the
// add-on-top pass safe.
func TestKomgaSyncClearsFlagWhenClusterBreaksUp(t *testing.T) {
	database := setupDB(t)
	remark := time.Date(2026, 9, 22, 12, 17, 40, 0, time.UTC)

	var clustered []komga.Book
	for i := 0; i < 6; i++ {
		clustered = append(clustered, komgaBook(fmt.Sprintf("b-%d", i), fmt.Sprintf("Series %d", i), true,
			remark.Add(time.Duration(i*50)*time.Millisecond), 1, 1))
	}
	client := &fakeKomgaClient{books: clustered}
	ing := newKomgaIngestor(t, database, client)

	res, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 6, res.Unreliable)

	// The server later reports real per-volume dates for the same books.
	var spread []komga.Book
	for i := 0; i < 6; i++ {
		spread = append(spread, komgaBook(fmt.Sprintf("b-%d", i), fmt.Sprintf("Series %d", i), true,
			remark.Add(time.Duration(i)*30*time.Minute), 1, 1))
	}
	client.books = spread

	res, err = ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Zero(t, res.Unreliable)
	for id, r := range komgaRows(t, database) {
		assert.False(t, r.StatusIsBulk, id)
	}
}

// Taken from the real library: five Tokyo Ghoul volumes and one from its sequel
// were all marked at 2026-09-20T10:50:13Z. An evening of reading marked in one
// batch looks exactly like a re-import in its timestamps and is not one, and it
// is the entry the journal showed as "Tokyo Ghoul — 5 volumes (10–14)".
func TestKomgaSyncKeepsASingleSeriesBatchMarking(t *testing.T) {
	database := setupDB(t)
	at := time.Date(2026, 9, 20, 10, 50, 13, 0, time.UTC)

	var books []komga.Book
	for i, vol := range []int{10, 11, 12, 13, 14} {
		b := komgaBook(fmt.Sprintf("tg-%d", vol), "Tokyo Ghoul", true, at, 1, 1)
		b.Number = float64(vol)
		b.Progress.ReadDate = at.Add(time.Duration(i) * time.Millisecond)
		books = append(books, b)
	}
	books = append(books, komgaBook("tgre-12", "Tokyo Ghoul - re", true, at.Add(time.Second), 1, 1))

	res, err := newKomgaIngestor(t, database, &fakeKomgaClient{books: books}).Sync(context.Background())
	require.NoError(t, err)
	assert.Zero(t, res.Unreliable, "two series is not a library re-import")
	assert.Equal(t, 1, res.Days, "the day still counts as reading")
	for id, r := range komgaRows(t, database) {
		assert.False(t, r.StatusIsBulk, id)
	}
}

func TestKomgaSyncReMarkSeriesThresholdIsConfigurable(t *testing.T) {
	database := setupDB(t)
	at := time.Date(2026, 9, 22, 12, 17, 40, 0, time.UTC)

	var books []komga.Book
	for i, name := range []string{"A", "A", "B", "B", "C"} {
		books = append(books, komgaBook(fmt.Sprintf("b-%d", i), name, true,
			at.Add(time.Duration(i*50)*time.Millisecond), 1, 1))
	}

	ing := newKomgaIngestor(t, database, &fakeKomgaClient{books: books})
	ing.MinReMarkSeries = 4
	res, err := ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Zero(t, res.Unreliable, "three series is below the raised threshold")

	ing.MinReMarkSeries = 3
	res, err = ing.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 5, res.Unreliable)
}

// A re-import writes for as long as the restore takes, so the run must be read
// as one event. At a one-second window the real 2026-09-22 event broke into
// fourteen clusters and the narrow tail of each escaped.
func TestKomgaSyncTreatsALongReImportRunAsOneCluster(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 9, 22, 12, 17, 35, 0, time.UTC)

	// Two books per second for 18 seconds, each a different series: never five
	// within one second, but plainly one event.
	var books []komga.Book
	for i := 0; i < 36; i++ {
		books = append(books, komgaBook(fmt.Sprintf("b-%d", i), fmt.Sprintf("Series %d", i), true,
			start.Add(time.Duration(i*500)*time.Millisecond), 1, 1))
	}

	res, err := newKomgaIngestor(t, database, &fakeKomgaClient{books: books}).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 36, res.Unreliable, "the whole run is one re-marking event")
	assert.Zero(t, res.Days)
}

// The wider window must not swallow a genuine session that happens to sit
// inside it.
func TestKomgaSyncWideWindowStillSparesOneSeries(t *testing.T) {
	database := setupDB(t)
	at := time.Date(2026, 9, 4, 21, 8, 58, 0, time.UTC)

	var books []komga.Book
	for i := 0; i < 9; i++ {
		books = append(books, komgaBook(fmt.Sprintf("tg-%d", i), "Tokyo Ghoul", true,
			at.Add(time.Duration(i*200)*time.Millisecond), 1, 1))
	}

	res, err := newKomgaIngestor(t, database, &fakeKomgaClient{books: books}).Sync(context.Background())
	require.NoError(t, err)
	assert.Zero(t, res.Unreliable)
	assert.Equal(t, 1, res.Days)
}
