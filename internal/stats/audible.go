// Package stats ingests listening history from each source into the local
// database. It owns the orchestration — windowing, resumption, rate limiting —
// while the source clients own the protocol details.
package stats

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

// Sync state keys for the Audible source.
const (
	// keyDailyThrough is the last day fully covered by a completed window.
	keyDailyThrough = "audible.daily.through"
	// keyBooksSyncedAt is when per-book state was last refreshed.
	keyBooksSyncedAt = "audible.books.synced_at"
)

// recentRefetchDays is how far back a resumed backfill re-fetches even though
// those days are already stored. Audible revises recent days as clients sync,
// so the tail of the last run is refreshed rather than trusted.
const recentRefetchDays = 3

// AudibleIngestor pulls Audible listening history into the database.
type AudibleIngestor struct {
	DB      *sql.DB
	Client  audible.StatsClient
	Bucket  *listening.Bucketer
	Clock   listening.Clock
	Limiter Waiter
	Logger  *slog.Logger

	// BackfillStart bounds how far back Backfill reaches.
	BackfillStart time.Time
	// BulkOptions tunes bulk status-change cluster detection.
	BulkOptions listening.BulkClusterOptions
}

// Waiter paces outbound requests. internal/download.RateLimiter satisfies it.
type Waiter interface {
	Wait(ctx context.Context) error
}

// noopWaiter is used when no limiter is supplied, so tests need not wait.
type noopWaiter struct{}

func (noopWaiter) Wait(context.Context) error { return nil }

// BackfillResult reports what a backfill run did.
type BackfillResult struct {
	WindowsFetched int
	DaysStored     int
	From           string
	Through        string
	Resumed        bool
}

// BooksResult reports what a per-book sync run did.
type BooksResult struct {
	Books           int
	FinishedRecords int
	PositionsFound  int
	BulkFlagged     int
}

func (a *AudibleIngestor) logger() *slog.Logger {
	if a.Logger != nil {
		return a.Logger
	}
	return slog.Default()
}

func (a *AudibleIngestor) clock() listening.Clock {
	if a.Clock != nil {
		return a.Clock
	}
	return listening.SystemClock{}
}

func (a *AudibleIngestor) limiter() Waiter {
	if a.Limiter != nil {
		return a.Limiter
	}
	return noopWaiter{}
}

// BackfillDaily walks Audible's daily listening totals from the configured
// start through today, storing each day.
//
// Windows are capped at the API's maximum and walked forward. Progress is
// recorded after each window, so an interrupted run resumes at the window
// boundary rather than restarting. The most recent few days are always
// re-fetched because Audible revises them.
func (a *AudibleIngestor) BackfillDaily(ctx context.Context) (BackfillResult, error) {
	var res BackfillResult

	today := a.clock().Now().In(a.Bucket.Location())
	start := a.BackfillStart
	if start.IsZero() {
		return res, fmt.Errorf("backfill start date is not set")
	}

	if through, ok, err := db.GetSyncState(a.DB, keyDailyThrough); err != nil {
		return res, err
	} else if ok {
		if t, perr := a.Bucket.ParseDay(through); perr == nil {
			resume := t.AddDate(0, 0, -recentRefetchDays)
			if resume.After(start) {
				start = resume
				res.Resumed = true
			}
		}
	}

	res.From = a.Bucket.Day(start)

	for cursor := start; !cursor.After(today); cursor = cursor.AddDate(0, 0, audible.MaxDailyWindow) {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if res.WindowsFetched > 0 {
			if err := a.limiter().Wait(ctx); err != nil {
				return res, err
			}
		}

		totals, err := a.Client.DailyListening(ctx, cursor, audible.MaxDailyWindow)
		if err != nil {
			// Return what was accomplished so the caller can report partial
			// progress; the watermark already reflects completed windows.
			return res, fmt.Errorf("daily window from %s: %w", a.Bucket.Day(cursor), err)
		}
		res.WindowsFetched++

		rows := make([]db.ListeningDay, 0, len(totals))
		for day, secs := range totals {
			rows = append(rows, db.ListeningDay{
				Day:     day,
				Source:  listening.SourceAudible,
				Seconds: int(secs + 0.5),
			})
		}
		if err := db.UpsertListeningDays(a.DB, rows); err != nil {
			return res, err
		}
		res.DaysStored += len(rows)

		windowEnd := cursor.AddDate(0, 0, audible.MaxDailyWindow-1)
		if windowEnd.After(today) {
			windowEnd = today
		}
		res.Through = a.Bucket.Day(windowEnd)
		if err := db.SetSyncState(a.DB, keyDailyThrough, res.Through); err != nil {
			return res, err
		}

		a.logger().Debug("audible daily window stored",
			"from", a.Bucket.Day(cursor), "through", res.Through, "days", len(rows))
	}

	return res, nil
}

// SyncBooks refreshes per-book Audible state: library metadata and listening
// status, mark-as-finished records, and last playback positions.
//
// Finish timestamps are screened for bulk clusters before storage, so that a
// mass-marking event is never mistaken for evidence about individual books.
func (a *AudibleIngestor) SyncBooks(ctx context.Context) (BooksResult, error) {
	var res BooksResult

	lib, err := a.Client.LibraryListening(ctx)
	if err != nil {
		return res, fmt.Errorf("library listening: %w", err)
	}
	res.Books = len(lib)

	if err := a.limiter().Wait(ctx); err != nil {
		return res, err
	}
	finished, err := a.Client.FinishedStatuses(ctx)
	if err != nil {
		return res, fmt.Errorf("finished statuses: %w", err)
	}
	res.FinishedRecords = len(finished)

	asins := make([]string, 0, len(lib))
	for _, b := range lib {
		if b.ASIN != "" {
			asins = append(asins, b.ASIN)
		}
	}

	if err := a.limiter().Wait(ctx); err != nil {
		return res, err
	}
	positions, err := a.Client.LastPositions(ctx, asins)
	if err != nil {
		return res, fmt.Errorf("last positions: %w", err)
	}

	// Screen every status change, from both the library and the dedicated
	// endpoint, in one pass: a cluster may span books that appear in only one.
	events := make([]listening.StatusEvent, 0, len(lib)+len(finished))
	statusByASIN := make(map[string]time.Time, len(finished))
	finishedByASIN := make(map[string]bool, len(finished))
	for _, f := range finished {
		if f.ASIN == "" {
			continue
		}
		statusByASIN[f.ASIN] = f.EventTime
		finishedByASIN[f.ASIN] = f.IsFinished
		events = append(events, listening.StatusEvent{
			Key: f.ASIN, OccurredAt: f.EventTime, Finished: f.IsFinished,
		})
	}
	for _, b := range lib {
		if _, seen := statusByASIN[b.ASIN]; seen || b.StatusChangedAt.IsZero() {
			continue
		}
		events = append(events, listening.StatusEvent{
			Key: b.ASIN, OccurredAt: b.StatusChangedAt, Finished: b.IsFinished,
		})
	}

	bulk := listening.DetectBulkClusters(events, a.BulkOptions)
	res.BulkFlagged = len(bulk)

	rows := make([]db.BookListening, 0, len(lib))
	bulkKeys := make([]string, 0, len(bulk))
	for _, b := range lib {
		row := db.BookListening{
			Source:          listening.SourceAudible,
			SourceKey:       b.ASIN,
			ASIN:            b.ASIN,
			Title:           b.Title,
			Author:          strings.Join(b.Authors, ", "),
			Narrator:        strings.Join(b.Narrators, ", "),
			Series:          b.Series,
			SeriesPosition:  b.SeriesPosition,
			Genres:          strings.Join(b.Genres, ","),
			RuntimeSeconds:  b.RuntimeMinutes * 60,
			PercentComplete: b.PercentComplete,
			IsFinished:      b.IsFinished,
			PurchaseDate:    b.PurchaseDate,
			AddedDate:       b.DateAdded,
		}

		// The dedicated endpoint is authoritative for status where it has a
		// record; the library field is the fallback.
		if t, ok := statusByASIN[b.ASIN]; ok && !t.IsZero() {
			row.StatusChangedAt = t.Format(time.RFC3339Nano)
			row.IsFinished = finishedByASIN[b.ASIN]
		} else if !b.StatusChangedAt.IsZero() {
			row.StatusChangedAt = b.StatusChangedAt.Format(time.RFC3339Nano)
		}

		if bulk[b.ASIN] {
			row.StatusIsBulk = true
			bulkKeys = append(bulkKeys, b.ASIN)
		}

		if p, ok := positions[b.ASIN]; ok && p.Exists {
			row.LastPositionAt = p.LastUpdated.Format(time.RFC3339Nano)
			row.LastPositionMS = p.PositionMS
			res.PositionsFound++
		}

		rows = append(rows, row)
	}

	if err := db.UpsertBookListeningBatch(a.DB, rows); err != nil {
		return res, err
	}
	// Recompute the flag set wholesale so rows that left a cluster are cleared.
	if err := db.MarkBulkStatus(a.DB, listening.SourceAudible, bulkKeys); err != nil {
		return res, err
	}
	if err := db.SetSyncTime(a.DB, keyBooksSyncedAt, a.clock().Now()); err != nil {
		return res, err
	}

	a.logger().Debug("audible book state synced",
		"books", res.Books, "finished_records", res.FinishedRecords,
		"positions", res.PositionsFound, "bulk_flagged", res.BulkFlagged)

	return res, nil
}
