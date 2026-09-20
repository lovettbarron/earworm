package journal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/db"
)

// recordingWriter captures deliveries instead of performing them.
type recordingWriter struct {
	writes   []Entry
	journals []string
	flushes  int
	writeErr error
	flushErr error
}

func (r *recordingWriter) Write(_ context.Context, journalID string, e Entry) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.writes = append(r.writes, e)
	r.journals = append(r.journals, journalID)
	return nil
}

func (r *recordingWriter) Flush(context.Context) error {
	if r.flushErr != nil {
		return r.flushErr
	}
	r.flushes++
	return nil
}

func setupSyncDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	require.NoError(t, db.UpsertListeningSessions(database, sessions()))
	require.NoError(t, db.UpsertBookListeningBatch(database, books()))
	return database
}

func TestSyncWritesEntriesAndFlushes(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}

	res, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, res.Rendered)
	assert.Equal(t, 2, res.Written)
	assert.Zero(t, res.Updated)
	assert.Len(t, w.writes, 2)
	assert.Equal(t, 1, w.flushes, "queued writes must be pushed exactly once per run")
	for _, j := range w.journals {
		assert.Equal(t, "journal-1", j)
	}
}

// The behaviour that makes a daily job safe: running twice changes nothing.
func TestSyncIsIdempotent(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}

	_, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	second, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	assert.Zero(t, second.Written, "nothing new should be written on a repeat run")
	assert.Zero(t, second.Updated)
	assert.Equal(t, 2, second.Unchanged)
	assert.Len(t, w.writes, 2, "no duplicate entries")
	assert.Equal(t, 1, w.flushes, "an unchanged run should not even flush")
}

func TestSyncRewritesChangedEntries(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}

	_, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	// More listening arrives for a day already written.
	require.NoError(t, db.UpsertListeningSessions(database, []db.ListeningSession{
		{ID: "s4", LibraryItemID: "item-1", Day: "2026-09-20", Seconds: 1800,
			Title: "Example Chronicle", Author: "An Author"},
	}))

	res, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, res.Updated, "the changed day should be rewritten")
	assert.Equal(t, 1, res.Unchanged)

	last := w.writes[len(w.writes)-1]
	assert.Equal(t, DayKey("2026-09-20"), last.Key)
	assert.Equal(t, EntryID(DayKey("2026-09-20")), last.ID,
		"an update must reuse the original id so it replaces rather than adds")
}

func TestSyncDryRunWritesNothing(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1", DryRun: true}

	res, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	assert.True(t, res.DryRun)
	assert.Equal(t, 2, res.Rendered)
	assert.Equal(t, 2, res.Written, "a dry run still reports what would happen")
	assert.Empty(t, w.writes, "a dry run must not touch the journal")
	assert.Zero(t, w.flushes)

	n, err := db.CountJournalEntries(database)
	require.NoError(t, err)
	assert.Zero(t, n, "a dry run must not record anything in the ledger")
}

func TestSyncDryRunNeedsNoJournalID(t *testing.T) {
	database := setupSyncDB(t)
	s := &Syncer{DB: database, DryRun: true}

	_, err := s.Sync(context.Background(), BuildOptions{})
	assert.NoError(t, err, "previewing should not require a configured destination")
}

func TestSyncRequiresJournalIDWhenWriting(t *testing.T) {
	database := setupSyncDB(t)
	s := &Syncer{DB: database, Writer: &recordingWriter{}}

	_, err := s.Sync(context.Background(), BuildOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal_id is not configured")
}

func TestSyncRequiresWriter(t *testing.T) {
	database := setupSyncDB(t)
	s := &Syncer{DB: database, JournalID: "journal-1"}

	_, err := s.Sync(context.Background(), BuildOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no journal writer")
}

// An entry must not be recorded as written if the write failed, or a retry
// would skip it.
func TestSyncDoesNotRecordFailedWrites(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{writeErr: fmt.Errorf("cli exploded")}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}

	_, err := s.Sync(context.Background(), BuildOptions{})
	require.Error(t, err)

	n, err := db.CountJournalEntries(database)
	require.NoError(t, err)
	assert.Zero(t, n, "a failed write must leave no ledger entry behind")
}

// A flush failure is a distinct problem: the entries exist locally but have
// not been pushed, and saying so points at the right fix.
func TestSyncReportsFlushFailureDistinctly(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{flushErr: fmt.Errorf("network down")}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}

	_, err := s.Sync(context.Background(), BuildOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entries written but sync to Day One failed")
	assert.Contains(t, err.Error(), "network down")
}

func TestSyncIncludesFinishEntriesWhenRequested(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1", IncludeFinishes: true}

	res, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	assert.Equal(t, 3, res.Rendered, "two day digests plus one genuine finish")

	var finishes int
	for _, e := range w.writes {
		if e.Kind == KindFinish {
			finishes++
		}
	}
	assert.Equal(t, 1, finishes, "only the non-bulk finish should be written")
}

// The central promise of this phase: reconstructed attribution stays out of
// the journal entirely.
func TestSyncNeverWritesInferredAttribution(t *testing.T) {
	database := setupSyncDB(t)

	// Audible day totals exist, and they cannot name a book.
	require.NoError(t, db.UpsertListeningDays(database, []db.ListeningDay{
		{Day: "2024-03-07", Source: "audible", Seconds: 11520},
		{Day: "2024-03-08", Source: "audible", Seconds: 7200},
	}))

	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1", IncludeFinishes: true}

	_, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	for _, e := range w.writes {
		assert.NotContains(t, e.Body, "2024-03-07",
			"a day known only from an Audible total must never become an entry")
		assert.NotContains(t, e.Body, "2024-03-08")

		lower := strings.ToLower(e.Body)
		for _, word := range []string{"inferred", "estimated", "approximately", "unattributed"} {
			assert.NotContains(t, lower, word,
				"journal entries carry measured facts only, found %q", word)
		}
	}
}

func TestSyncRespectsDateRange(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}

	res, err := s.Sync(context.Background(), BuildOptions{Since: "2026-09-20"})
	require.NoError(t, err)

	assert.Equal(t, 1, res.Rendered)
	require.Len(t, w.writes, 1)
	assert.Equal(t, DayKey("2026-09-20"), w.writes[0].Key)
}

func TestSyncWithNoDataIsHarmless(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	w := &recordingWriter{}
	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}

	res, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	assert.Zero(t, res.Rendered)
	assert.Empty(t, w.writes)
	assert.Zero(t, w.flushes, "nothing to write means nothing to flush")
}

func TestSyncTracksJournalChange(t *testing.T) {
	database := setupSyncDB(t)
	w := &recordingWriter{}

	s := &Syncer{DB: database, Writer: w, JournalID: "journal-1"}
	_, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)

	// Pointing at a different journal means the entries do not exist there yet.
	s.JournalID = "journal-2"
	res, err := s.Sync(context.Background(), BuildOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Updated, "entries must be re-sent when the destination changes")
}
