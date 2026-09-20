package stats

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

// Sync state keys for the Audiobookshelf source.
const (
	// keyABSWatermark is the newest session update time seen by a completed sync.
	keyABSWatermark = "abs.sessions.watermark"
)

// ABSOverlapWindow is how far before the watermark a sync re-reads.
//
// Audiobookshelf leaves a session open while playback continues and only
// auto-closes it after 36 hours of inactivity. A session that was open during
// the previous sync therefore keeps accumulating time afterwards, and syncing
// strictly after the watermark would freeze it at a stale value. Re-reading a
// window wider than that idle timeout picks up the final value; upserting by
// session id makes the re-read free of double counting.
const ABSOverlapWindow = 48 * time.Hour

// ABSSessionSource is the subset of the Audiobookshelf client this ingestor
// needs. Declaring it here keeps the ingestor testable without a live server.
type ABSSessionSource interface {
	Me(ctx context.Context) (audiobookshelf.User, []audiobookshelf.MediaProgress, error)
	ListSessions(ctx context.Context, opts audiobookshelf.SessionsOptions) ([]audiobookshelf.PlaybackSession, error)
	GetItems(ctx context.Context, ids []string) (map[string]audiobookshelf.LibraryItem, error)
}

// ABSIngestor pulls Audiobookshelf playback sessions into the database.
type ABSIngestor struct {
	DB     *sql.DB
	Client ABSSessionSource
	Bucket *listening.Bucketer
	Clock  listening.Clock
	Logger *slog.Logger

	// UserID scopes the admin sessions endpoint. Resolved from the API token
	// when empty.
	UserID string
	// Enrich fetches library items to fill in genres and series that session
	// metadata snapshots omit.
	Enrich bool
}

// ABSResult reports what a sync run did.
type ABSResult struct {
	Sessions    int
	Days        int
	Books       int
	Enriched    int
	Incremental bool
	Since       string
	Watermark   string
}

func (a *ABSIngestor) logger() *slog.Logger {
	if a.Logger != nil {
		return a.Logger
	}
	return slog.Default()
}

func (a *ABSIngestor) clock() listening.Clock {
	if a.Clock != nil {
		return a.Clock
	}
	return listening.SystemClock{}
}

// ResolveUser returns the configured user id, looking it up from the token if
// it was not set.
func (a *ABSIngestor) ResolveUser(ctx context.Context) (string, error) {
	if a.UserID != "" {
		return a.UserID, nil
	}
	user, _, err := a.Client.Me(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve audiobookshelf user: %w", err)
	}
	if user.ID == "" {
		return "", fmt.Errorf("audiobookshelf returned no user id for the configured token")
	}
	a.UserID = user.ID
	return user.ID, nil
}

// Sync fetches sessions since the stored watermark, or everything when full is
// true, and stores them along with derived day totals and per-book rollups.
func (a *ABSIngestor) Sync(ctx context.Context, full bool) (ABSResult, error) {
	var res ABSResult

	userID, err := a.ResolveUser(ctx)
	if err != nil {
		return res, err
	}

	opts := audiobookshelf.SessionsOptions{UserID: userID}
	if !full {
		if watermark, ok, err := db.GetSyncTime(a.DB, keyABSWatermark); err != nil {
			return res, err
		} else if ok {
			opts.Since = watermark.Add(-ABSOverlapWindow)
			res.Incremental = true
			res.Since = opts.Since.Format(time.RFC3339)
		}
	}

	sessions, err := a.Client.ListSessions(ctx, opts)
	if err != nil {
		return res, fmt.Errorf("list sessions: %w", err)
	}
	res.Sessions = len(sessions)

	if len(sessions) == 0 {
		return res, nil
	}

	// Optional enrichment: session metadata snapshots frequently carry empty
	// genre and series arrays even when the library item has them.
	items := map[string]audiobookshelf.LibraryItem{}
	if a.Enrich {
		ids := uniqueItemIDs(sessions)
		items, err = a.Client.GetItems(ctx, ids)
		if err != nil {
			return res, fmt.Errorf("enrich library items: %w", err)
		}
		res.Enriched = len(items)
	}

	rows := make([]db.ListeningSession, 0, len(sessions))
	dayTotals := make(map[string]int)
	var newest time.Time

	for _, s := range sessions {
		started := s.StartedAt.Time()
		// The day bucket comes from the raw timestamp, not the server's own
		// date string, so it does not shift with the server's timezone.
		day := ""
		if !started.IsZero() {
			day = a.Bucket.Day(started)
		}

		asin := s.Metadata.ASIN
		title := firstNonEmpty(s.DisplayTitle, s.Metadata.Title)
		author := firstNonEmpty(s.DisplayAuthor, s.Metadata.AuthorName)
		if it, ok := items[s.LibraryItemID]; ok {
			asin = firstNonEmpty(it.Media.Metadata.ASIN, asin)
			title = firstNonEmpty(it.Media.Metadata.Title, title)
			author = firstNonEmpty(it.AuthorDisplay(), author)
		}

		rows = append(rows, db.ListeningSession{
			ID:              s.ID,
			UserID:          s.UserID,
			LibraryItemID:   s.LibraryItemID,
			BookID:          s.BookID,
			EpisodeID:       s.EpisodeID,
			MediaType:       s.MediaType,
			ASIN:            asin,
			Title:           title,
			Author:          author,
			Day:             day,
			Seconds:         s.TimeListening.Int(),
			DurationSeconds: s.Duration.Int(),
			StartSeconds:    s.StartTime.Int(),
			CurrentSeconds:  s.CurrentTime.Int(),
			StartedAt:       formatTime(started),
			UpdatedAt:       formatTime(s.UpdatedAt.Time()),
			Device:          s.DeviceDescription(),
		})

		if day != "" {
			dayTotals[day] += s.TimeListening.Int()
		}
		if u := s.UpdatedAt.Time(); u.After(newest) {
			newest = u
		}
	}

	if err := db.UpsertListeningSessions(a.DB, rows); err != nil {
		return res, err
	}

	// Day totals are recomputed from all stored sessions rather than from this
	// batch alone. An incremental run sees only part of a day, and writing that
	// partial sum would overwrite the fuller figure from an earlier run.
	if err := a.refreshDayTotals(); err != nil {
		return res, err
	}
	res.Days = len(dayTotals)

	books, err := a.refreshBookRollups(items)
	if err != nil {
		return res, err
	}
	res.Books = books

	if !newest.IsZero() {
		if err := db.SetSyncTime(a.DB, keyABSWatermark, newest); err != nil {
			return res, err
		}
		res.Watermark = newest.Format(time.RFC3339)
	}

	a.logger().Debug("audiobookshelf sync complete",
		"sessions", res.Sessions, "days", res.Days, "books", res.Books,
		"incremental", res.Incremental)

	return res, nil
}

// refreshDayTotals recomputes per-day totals from every stored session.
func (a *ABSIngestor) refreshDayTotals() error {
	byDay, err := db.SessionsByDay(a.DB)
	if err != nil {
		return err
	}
	rows := make([]db.ListeningDay, 0, len(byDay))
	for day, secs := range byDay {
		rows = append(rows, db.ListeningDay{
			Day:     day,
			Source:  listening.SourceABS,
			Seconds: secs,
		})
	}
	return db.UpsertListeningDays(a.DB, rows)
}

// refreshBookRollups recomputes per-book totals from every stored session.
func (a *ABSIngestor) refreshBookRollups(items map[string]audiobookshelf.LibraryItem) (int, error) {
	sessions, err := db.ListListeningSessions(a.DB)
	if err != nil {
		return 0, err
	}

	type agg struct {
		row     db.BookListening
		seconds int
		first   string
		last    string
	}
	byItem := make(map[string]*agg)

	for _, s := range sessions {
		key := s.LibraryItemID
		if key == "" {
			key = s.BookID
		}
		if key == "" {
			continue
		}
		a, ok := byItem[key]
		if !ok {
			a = &agg{row: db.BookListening{
				Source:    listening.SourceABS,
				SourceKey: key,
				ASIN:      s.ASIN,
				Title:     s.Title,
				Author:    s.Author,
			}}
			byItem[key] = a
		}
		a.seconds += s.Seconds
		if s.ASIN != "" {
			a.row.ASIN = s.ASIN
		}
		if s.Title != "" {
			a.row.Title = s.Title
		}
		if s.Author != "" {
			a.row.Author = s.Author
		}
		if s.DurationSeconds > 0 {
			a.row.RuntimeSeconds = s.DurationSeconds
		}
		if a.first == "" || (s.StartedAt != "" && s.StartedAt < a.first) {
			a.first = s.StartedAt
		}
		if s.UpdatedAt > a.last {
			a.last = s.UpdatedAt
		}
	}

	rows := make([]db.BookListening, 0, len(byItem))
	for key, agg := range byItem {
		row := agg.row
		row.SecondsListened = agg.seconds
		row.LastPositionAt = agg.last
		row.AddedDate = agg.first

		if it, ok := items[key]; ok {
			if g := it.Media.Metadata.Genres; len(g) > 0 {
				row.Genres = strings.Join(g, ",")
			}
			if se := it.Media.Metadata.Series; len(se) > 0 {
				row.Series = se[0].Name
				row.SeriesPosition = se[0].Sequence
			}
			if n := it.NarratorDisplay(); n != "" {
				row.Narrator = n
			}
		}

		// Percent complete is derived rather than read: ABS stores a progress
		// value that is not recomputed on read and can be stale.
		if row.RuntimeSeconds > 0 {
			pct := float64(agg.seconds) / float64(row.RuntimeSeconds) * 100
			if pct > 100 {
				pct = 100
			}
			row.PercentComplete = pct
		}

		rows = append(rows, row)
	}

	if err := db.UpsertBookListeningBatch(a.DB, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func uniqueItemIDs(sessions []audiobookshelf.PlaybackSession) []string {
	seen := make(map[string]bool, len(sessions))
	var out []string
	for _, s := range sessions {
		if s.LibraryItemID == "" || seen[s.LibraryItemID] {
			continue
		}
		seen[s.LibraryItemID] = true
		out = append(out, s.LibraryItemID)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
