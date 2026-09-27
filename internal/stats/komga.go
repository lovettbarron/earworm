package stats

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/komga"
	"github.com/lovettbarron/earworm/internal/listening"
)

// keyKomgaSyncedAt records when Komga was last read.
const keyKomgaSyncedAt = "komga.synced_at"

// DefaultKomgaBulkWindow is how long a run of completions may keep going and
// still count as one re-marking event.
const DefaultKomgaBulkWindow = 30 * time.Second

// KomgaSource is the subset of the Komga client the ingestor needs.
type KomgaSource interface {
	BooksWithProgress(ctx context.Context) ([]komga.Book, error)
}

// KomgaIngestor pulls reading progress into the database.
//
// Komga rows land in book_listening alongside audio. The table already carries
// series, volume, a completion timestamp and an unreliable-provenance flag, so
// reading needs no schema of its own and the export and journal pick it up
// through the paths that already exist.
type KomgaIngestor struct {
	DB     *sql.DB
	Client KomgaSource
	Bucket *listening.Bucketer
	Clock  listening.Clock
	Logger *slog.Logger

	// BulkOptions tunes detection of mass re-marking, where a run of
	// completions shares one instant. A Komga library re-import writes exactly
	// that: measured on a real server, 103 books landed across 32 seconds at
	// six per second, none of them read that day.
	//
	// The default window is wider than Audible's one second, because a
	// re-import writes continuously for as long as the restore takes. At one
	// second the same event broke into fourteen clusters and the narrow tail of
	// each escaped; at thirty it is one cluster of 78 books across 63 series,
	// while every genuine reading batch stays inside a single series and is
	// spared by MinReMarkSeries.
	BulkOptions listening.BulkClusterOptions

	// MaxReadingBatch is the largest run of completions one instant can hold
	// and still be called reading, whatever it spans. Default 15.
	//
	// The series test alone lets a narrow but enormous batch through: a
	// re-import wrote fifteen Solo Leveling volumes and four of another series
	// in nine seconds, which no evening produces. Genuine sessions on the
	// measured library never exceeded nine books, so the two do not overlap.
	MaxReadingBatch int

	// MinReMarkSeries is how many distinct series a cluster must span before it
	// counts as re-marking rather than reading. Default 3.
	//
	// Timestamps alone cannot tell the two apart: finishing five volumes of one
	// series and marking them together writes the same dense run as a
	// re-import. What differs is breadth. On the library this was measured
	// against, every cluster spanning three or more series belonged to the
	// re-import, and every single-series cluster was a genuine evening of
	// reading. Without this test the detector deletes real journal entries.
	MinReMarkSeries int

	// UnreliableBefore flags completions at or before this instant as not
	// trustworthy evidence of when the book was actually read.
	//
	// This cannot be inferred. Migration re-marks and genuine reads are
	// indistinguishable in Komga's data: measured against a real library, the
	// migration window showed 69% instant progress writes against 97% for
	// genuine reading, and a 3.5-minute median span against 2.5. Anything
	// automatic would be guessing, so the boundary is declared by the user.
	UnreliableBefore time.Time
}

// KomgaResult reports what a sync run did.
type KomgaResult struct {
	Books      int `json:"books"`
	Completed  int `json:"completed"`
	InProgress int `json:"in_progress"`
	Unreliable int `json:"unreliable"`
	Series     int `json:"series"`
	Days       int `json:"days"`
}

func (k *KomgaIngestor) logger() *slog.Logger {
	if k.Logger != nil {
		return k.Logger
	}
	return slog.Default()
}

func (k *KomgaIngestor) clock() listening.Clock {
	if k.Clock != nil {
		return k.Clock
	}
	return listening.SystemClock{}
}

// cutoff renders UnreliableBefore the way status timestamps are stored, so the
// database can compare the two as strings.
func (k *KomgaIngestor) cutoff() string {
	if k.UnreliableBefore.IsZero() {
		return ""
	}
	return k.UnreliableBefore.UTC().Format(time.RFC3339Nano)
}

// reMarked returns the ids belonging to clusters wide enough to be re-marking.
func (k *KomgaIngestor) reMarked(events []listening.StatusEvent, books []komga.Book) map[string]bool {
	opts := k.BulkOptions
	if opts.Window <= 0 {
		opts.Window = DefaultKomgaBulkWindow
	}

	minSeries := k.MinReMarkSeries
	if minSeries <= 0 {
		minSeries = 3
	}
	maxBatch := k.MaxReadingBatch
	if maxBatch <= 0 {
		maxBatch = 15
	}

	seriesOf := make(map[string]string, len(books))
	for _, b := range books {
		seriesOf[b.ID] = b.DisplaySeries()
	}

	out := make(map[string]bool)
	for _, cluster := range listening.DetectBulkClusterGroups(events, opts) {
		distinct := make(map[string]struct{}, len(cluster))
		for _, e := range cluster {
			distinct[seriesOf[e.Key]] = struct{}{}
		}
		if len(distinct) < minSeries && len(cluster) < maxBatch {
			continue
		}
		for _, e := range cluster {
			out[e.Key] = true
		}
	}
	return out
}

// Sync reads every book with progress and stores it.
//
// Komga holds current state rather than history, so each run is a full refresh
// and upserts by book id. Re-running neither duplicates rows nor loses flags.
func (k *KomgaIngestor) Sync(ctx context.Context) (KomgaResult, error) {
	var res KomgaResult

	books, err := k.Client.BooksWithProgress(ctx)
	if err != nil {
		return res, fmt.Errorf("komga books: %w", err)
	}
	res.Books = len(books)

	// Detected across every dated completion in the response, before rows are
	// built, so a cluster is judged on the whole set rather than per book.
	events := make([]listening.StatusEvent, 0, len(books))
	for _, b := range books {
		if b.Progress == nil {
			continue
		}
		events = append(events, listening.StatusEvent{
			Key: b.ID, OccurredAt: b.CompletedAt(), Finished: b.Progress.Completed,
		})
	}
	clustered := k.reMarked(events, books)

	rows := make([]db.BookListening, 0, len(books))
	clusterKeys := make([]string, 0)
	series := make(map[string]struct{})
	days := make(map[string]struct{})

	for _, b := range books {
		if b.Progress == nil {
			continue
		}

		completedAt := b.CompletedAt()
		beforeCutoff := !k.UnreliableBefore.IsZero() &&
			!completedAt.IsZero() && !completedAt.After(k.UnreliableBefore)
		unreliable := beforeCutoff || clustered[b.ID]
		if clustered[b.ID] {
			clusterKeys = append(clusterKeys, b.ID)
		}

		if b.Progress.Completed {
			res.Completed++
		} else {
			res.InProgress++
		}
		if sn := b.DisplaySeries(); sn != "" {
			series[sn] = struct{}{}
		}
		if !completedAt.IsZero() && !unreliable {
			days[k.Bucket.Day(completedAt)] = struct{}{}
		}

		// Percent complete is derived from the page reached, since Komga
		// reports a page rather than a percentage.
		var pct float64
		if b.Media.PagesCount > 0 {
			pct = float64(b.Progress.Page) / float64(b.Media.PagesCount) * 100
			if pct > 100 {
				pct = 100
			}
		}
		if b.Progress.Completed {
			pct = 100
		}

		row := db.BookListening{
			Source:          listening.SourceKomga,
			SourceKey:       b.ID,
			Title:           b.DisplayTitle(),
			Author:          b.AuthorDisplay(),
			Series:          b.DisplaySeries(),
			SeriesPosition:  b.VolumeNumber(),
			Genres:          strings.Join(b.Metadata.Tags, ","),
			PercentComplete: pct,
			IsFinished:      b.Progress.Completed,
			StatusIsBulk:    unreliable,
		}
		if !completedAt.IsZero() {
			row.StatusChangedAt = completedAt.UTC().Format(time.RFC3339Nano)
			row.LastPositionAt = row.StatusChangedAt
		}
		if !b.Progress.Created.IsZero() {
			row.AddedDate = b.Progress.Created.UTC().Format(time.RFC3339Nano)
		}
		// The span between the first and last progress write is recorded where
		// it exists, but it is not reading time and most books have none.
		if span := b.ReadingSpan(); span > 0 && span < 12*time.Hour {
			row.SecondsListened = int(span.Seconds())
		}
		row.LastPositionMS = int64(b.Progress.Page)

		rows = append(rows, row)
	}

	res.Series = len(series)
	res.Days = len(days)

	if err := db.UpsertBookListeningBatch(k.DB, rows); err != nil {
		return res, err
	}
	// Flags are recomputed from the stored dates rather than from this
	// response. Komga lists only what currently holds a read status, so a book
	// whose id changes in a library re-import simply stops appearing; deciding
	// from the response alone would unflag a migration cluster that is still in
	// the table, and its dates would then read as genuine reading.
	flagged, err := db.MarkBulkStatusBefore(k.DB, listening.SourceKomga, k.cutoff())
	if err != nil {
		return res, err
	}
	// Cluster flags are added on top, without clearing: the pass above has
	// already set every row from its stored date, so a book that has left a
	// cluster is back to 0 and is not re-added here.
	added, err := db.AddBulkStatus(k.DB, listening.SourceKomga, clusterKeys)
	if err != nil {
		return res, err
	}
	res.Unreliable = flagged + added
	if err := db.SetSyncTime(k.DB, keyKomgaSyncedAt, k.clock().Now()); err != nil {
		return res, err
	}

	k.logger().Debug("komga sync complete",
		"books", res.Books, "completed", res.Completed,
		"unreliable", res.Unreliable, "series", res.Series)

	return res, nil
}
