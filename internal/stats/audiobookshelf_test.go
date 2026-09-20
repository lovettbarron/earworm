package stats

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

// fakeABSClient is a scripted ABSSessionSource.
type fakeABSClient struct {
	user      audiobookshelf.User
	userErr   error
	sessions  []audiobookshelf.PlaybackSession
	sessErr   error
	items     map[string]audiobookshelf.LibraryItem
	itemsErr  error
	lastOpts  audiobookshelf.SessionsOptions
	itemCalls int
}

func (f *fakeABSClient) Me(context.Context) (audiobookshelf.User, []audiobookshelf.MediaProgress, error) {
	return f.user, nil, f.userErr
}

func (f *fakeABSClient) ListSessions(_ context.Context, opts audiobookshelf.SessionsOptions) ([]audiobookshelf.PlaybackSession, error) {
	f.lastOpts = opts
	if f.sessErr != nil {
		return nil, f.sessErr
	}
	if opts.Since.IsZero() {
		return f.sessions, nil
	}
	var out []audiobookshelf.PlaybackSession
	for _, s := range f.sessions {
		if !s.UpdatedAt.Time().Before(opts.Since) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeABSClient) GetItems(_ context.Context, ids []string) (map[string]audiobookshelf.LibraryItem, error) {
	f.itemCalls++
	if f.itemsErr != nil {
		return nil, f.itemsErr
	}
	out := make(map[string]audiobookshelf.LibraryItem)
	for _, id := range ids {
		if it, ok := f.items[id]; ok {
			out[id] = it
		}
	}
	return out, nil
}

func millis(t time.Time) audiobookshelf.EpochMillis {
	return audiobookshelf.EpochMillis(t.UnixMilli())
}

func newABSIngestor(t *testing.T, database *sql.DB, client ABSSessionSource) *ABSIngestor {
	t.Helper()
	bucket, err := listening.NewBucketer("UTC")
	require.NoError(t, err)
	return &ABSIngestor{
		DB:     database,
		Client: client,
		Bucket: bucket,
		Clock:  listening.FixedClock{T: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)},
		UserID: "user-1",
	}
}

func sampleSession(id string, started time.Time, seconds float64) audiobookshelf.PlaybackSession {
	return audiobookshelf.PlaybackSession{
		ID:            id,
		UserID:        "user-1",
		LibraryItemID: "item-1",
		MediaType:     "book",
		DisplayTitle:  "A Test Title",
		DisplayAuthor: "An Author",
		TimeListening: audiobookshelf.LenientSeconds(seconds),
		Duration:      audiobookshelf.LenientSeconds(36000),
		StartedAt:     millis(started),
		UpdatedAt:     millis(started.Add(time.Hour)),
	}
}

func TestABSSyncStoresSessionsDaysAndBooks(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{
		sampleSession("s1", start, 1800),
		sampleSession("s2", start.Add(2*time.Hour), 900),
	}}

	ing := newABSIngestor(t, database, client)
	res, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)

	assert.Equal(t, 2, res.Sessions)
	assert.Equal(t, 1, res.Days)
	assert.Equal(t, 1, res.Books)

	sessions, err := db.ListListeningSessions(database)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	assert.Equal(t, "2026-03-01", sessions[0].Day)

	days, err := db.ListListeningDays(database, listening.SourceABS)
	require.NoError(t, err)
	require.Len(t, days, 1)
	assert.Equal(t, 2700, days[0].Seconds, "day total is the sum of its sessions")

	books, err := db.ListBookListening(database, listening.SourceABS)
	require.NoError(t, err)
	require.Len(t, books, 1)
	assert.Equal(t, 2700, books[0].SecondsListened)
	assert.Equal(t, "A Test Title", books[0].Title)
}

// The defining behaviour of this source: a session keeps growing while open,
// so re-observing it must update rather than insert.
func TestABSSyncUpdatesGrowingSessionInPlace(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{
		sampleSession("s1", start, 1800),
	}}
	ing := newABSIngestor(t, database, client)

	_, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)

	// The same session, still open, now with more listening time.
	client.sessions = []audiobookshelf.PlaybackSession{sampleSession("s1", start, 3600)}
	_, err = ing.Sync(context.Background(), true)
	require.NoError(t, err)

	n, err := db.CountListeningSessions(database)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a growing session must not become two rows")

	sessions, err := db.ListListeningSessions(database)
	require.NoError(t, err)
	assert.Equal(t, 3600, sessions[0].Seconds, "the later value wins")

	days, err := db.ListListeningDays(database, listening.SourceABS)
	require.NoError(t, err)
	assert.Equal(t, 3600, days[0].Seconds,
		"the day total must reflect the updated session, not the sum of both observations")
}

func TestABSSyncIsIdempotent(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{
		sampleSession("s1", start, 1800),
		sampleSession("s2", start.Add(time.Hour), 600),
	}}
	ing := newABSIngestor(t, database, client)

	for i := 0; i < 3; i++ {
		_, err := ing.Sync(context.Background(), true)
		require.NoError(t, err)
	}

	n, err := db.CountListeningSessions(database)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	days, err := db.ListListeningDays(database, listening.SourceABS)
	require.NoError(t, err)
	assert.Equal(t, 2400, days[0].Seconds, "repeated syncs must not inflate day totals")
}

func TestABSSyncUsesWatermarkWithOverlap(t *testing.T) {
	database := setupDB(t)
	watermark := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.SetSyncTime(database, keyABSWatermark, watermark))

	client := &fakeABSClient{}
	ing := newABSIngestor(t, database, client)

	res, err := ing.Sync(context.Background(), false)
	require.NoError(t, err)

	assert.True(t, res.Incremental)
	expected := watermark.Add(-ABSOverlapWindow)
	assert.True(t, client.lastOpts.Since.Equal(expected),
		"sync should reach back past the watermark by the overlap window; want %s got %s",
		expected, client.lastOpts.Since)
	assert.GreaterOrEqual(t, ABSOverlapWindow, 36*time.Hour,
		"the overlap must exceed the server's 36h open-session timeout")
}

func TestABSSyncFullIgnoresWatermark(t *testing.T) {
	database := setupDB(t)
	require.NoError(t, db.SetSyncTime(database, keyABSWatermark, time.Now()))

	client := &fakeABSClient{}
	ing := newABSIngestor(t, database, client)

	res, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)
	assert.False(t, res.Incremental)
	assert.True(t, client.lastOpts.Since.IsZero())
}

func TestABSSyncRecordsWatermarkFromNewestSession(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	newest := start.Add(5 * time.Hour)

	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{
		sampleSession("s1", start, 100),
		sampleSession("s2", newest, 100),
	}}
	ing := newABSIngestor(t, database, client)

	_, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)

	got, ok, err := db.GetSyncTime(database, keyABSWatermark)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, got.Equal(newest.Add(time.Hour)), "watermark is the newest updatedAt seen")
}

// An incremental run sees only part of a day. Writing that partial sum would
// clobber the fuller figure an earlier run stored.
func TestABSSyncRecomputesDayTotalsFromAllStoredSessions(t *testing.T) {
	database := setupDB(t)
	day := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{
		sampleSession("s1", day, 1200),
	}}
	ing := newABSIngestor(t, database, client)
	_, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)

	// A later sync returns only the new session from the same day.
	client.sessions = []audiobookshelf.PlaybackSession{
		sampleSession("s2", day.Add(6*time.Hour), 1800),
	}
	_, err = ing.Sync(context.Background(), true)
	require.NoError(t, err)

	days, err := db.ListListeningDays(database, listening.SourceABS)
	require.NoError(t, err)
	require.Len(t, days, 1)
	assert.Equal(t, 3000, days[0].Seconds,
		"the day total must include sessions from both runs, not just the latest batch")
}

func TestABSSyncEnrichesFromLibraryItems(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	var item audiobookshelf.LibraryItem
	item.ID = "item-1"
	item.Media.Metadata.Title = "Enriched Title"
	item.Media.Metadata.ASIN = "SYNTH00001"
	item.Media.Metadata.Genres = []string{"Fantasy", "Epic"}
	item.Media.Metadata.Series = []struct {
		Name     string `json:"name"`
		Sequence string `json:"sequence"`
	}{{Name: "A Series", Sequence: "2"}}

	client := &fakeABSClient{
		sessions: []audiobookshelf.PlaybackSession{sampleSession("s1", start, 1800)},
		items:    map[string]audiobookshelf.LibraryItem{"item-1": item},
	}
	ing := newABSIngestor(t, database, client)
	ing.Enrich = true

	res, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Enriched)

	books, err := db.ListBookListening(database, listening.SourceABS)
	require.NoError(t, err)
	require.Len(t, books, 1)
	assert.Equal(t, "Fantasy,Epic", books[0].Genres,
		"genres come from the item because session snapshots omit them")
	assert.Equal(t, "A Series", books[0].Series)
	assert.Equal(t, "2", books[0].SeriesPosition)
	assert.Equal(t, "SYNTH00001", books[0].ASIN)
}

func TestABSSyncSkipsEnrichmentWhenDisabled(t *testing.T) {
	database := setupDB(t)
	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{
		sampleSession("s1", time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), 600),
	}}
	ing := newABSIngestor(t, database, client)
	ing.Enrich = false

	_, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)
	assert.Zero(t, client.itemCalls, "enrichment must not fire when disabled")
}

func TestABSSyncDerivesPercentCompleteRatherThanTrustingServer(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	s := sampleSession("s1", start, 18000) // half of a 36000s book
	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{s}}
	ing := newABSIngestor(t, database, client)

	_, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)

	books, err := db.ListBookListening(database, listening.SourceABS)
	require.NoError(t, err)
	assert.InDelta(t, 50.0, books[0].PercentComplete, 0.01)
}

func TestABSSyncCapsPercentCompleteAtHundred(t *testing.T) {
	database := setupDB(t)
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	// Re-listening produces more listened time than the book's duration.
	s := sampleSession("s1", start, 72000)
	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{s}}
	ing := newABSIngestor(t, database, client)

	_, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)

	books, err := db.ListBookListening(database, listening.SourceABS)
	require.NoError(t, err)
	assert.InDelta(t, 100.0, books[0].PercentComplete, 0.01)
}

func TestABSSyncBucketsDayFromTimestampNotServerDate(t *testing.T) {
	database := setupDB(t)

	// 23:30 UTC: a server in a positive offset would label this the next day.
	start := time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC)
	s := sampleSession("s1", start, 600)
	s.ServerDate = "2026-03-02" // deliberately disagrees

	client := &fakeABSClient{sessions: []audiobookshelf.PlaybackSession{s}}
	ing := newABSIngestor(t, database, client)

	_, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)

	sessions, err := db.ListListeningSessions(database)
	require.NoError(t, err)
	assert.Equal(t, "2026-03-01", sessions[0].Day,
		"the day must derive from the timestamp in the configured zone, not the server's own date")
}

func TestABSResolveUserFromToken(t *testing.T) {
	database := setupDB(t)
	client := &fakeABSClient{user: audiobookshelf.User{ID: "resolved-user", Username: "tester"}}

	ing := newABSIngestor(t, database, client)
	ing.UserID = ""

	id, err := ing.ResolveUser(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "resolved-user", id)
	assert.Equal(t, "resolved-user", ing.UserID, "the resolved id should be cached")
}

func TestABSResolveUserErrors(t *testing.T) {
	database := setupDB(t)

	t.Run("request fails", func(t *testing.T) {
		ing := newABSIngestor(t, database, &fakeABSClient{userErr: fmt.Errorf("unreachable")})
		ing.UserID = ""
		_, err := ing.ResolveUser(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unreachable")
	})

	t.Run("no id returned", func(t *testing.T) {
		ing := newABSIngestor(t, database, &fakeABSClient{})
		ing.UserID = ""
		_, err := ing.ResolveUser(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no user id")
	})
}

func TestABSSyncPropagatesErrors(t *testing.T) {
	database := setupDB(t)

	t.Run("sessions", func(t *testing.T) {
		ing := newABSIngestor(t, database, &fakeABSClient{sessErr: fmt.Errorf("sessions down")})
		_, err := ing.Sync(context.Background(), true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sessions down")
	})

	t.Run("enrichment", func(t *testing.T) {
		client := &fakeABSClient{
			sessions: []audiobookshelf.PlaybackSession{
				sampleSession("s1", time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), 600),
			},
			itemsErr: fmt.Errorf("items down"),
		}
		ing := newABSIngestor(t, database, client)
		ing.Enrich = true
		_, err := ing.Sync(context.Background(), true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "items down")
	})
}

func TestABSSyncWithNoSessionsIsHarmless(t *testing.T) {
	database := setupDB(t)
	ing := newABSIngestor(t, database, &fakeABSClient{})

	res, err := ing.Sync(context.Background(), true)
	require.NoError(t, err)
	assert.Zero(t, res.Sessions)

	n, err := db.CountListeningSessions(database)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestUniqueItemIDs(t *testing.T) {
	sessions := []audiobookshelf.PlaybackSession{
		{LibraryItemID: "a"}, {LibraryItemID: "b"}, {LibraryItemID: "a"}, {LibraryItemID: ""},
	}
	assert.Equal(t, []string{"a", "b"}, uniqueItemIDs(sessions))
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "b", firstNonEmpty("", "b", "c"))
	assert.Equal(t, "", firstNonEmpty("", ""))
}

func TestFormatTimeZeroIsEmpty(t *testing.T) {
	assert.Empty(t, formatTime(time.Time{}))
	assert.NotEmpty(t, formatTime(time.Now()))
}
