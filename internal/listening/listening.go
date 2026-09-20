// Package listening holds conventions shared by every listening-stats source:
// how a timestamp becomes a day bucket, how "now" is obtained in tests, and how
// bulk status-change artifacts are recognised.
//
// These live in one place because the sources disagree. Audiobookshelf ships a
// precomputed date string in the server's own timezone alongside epoch
// milliseconds; Audible returns date strings with no zone at all. Trusting
// either source's own bucketing produces days that silently disagree by one.
// Everything here derives buckets from an explicit location instead.
package listening

import (
	"fmt"
	"sort"
	"time"
)

// DayFormat is the canonical day-bucket layout used for every day key.
const DayFormat = "2006-01-02"

// Source identifiers recorded alongside every stored row.
const (
	SourceAudible = "audible"
	SourceABS     = "abs"
)

// Clock supplies the current time. Production code uses SystemClock; tests
// substitute a fixed clock so day-boundary behaviour is deterministic.
type Clock interface {
	Now() time.Time
}

// SystemClock reports the real wall-clock time.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// FixedClock always reports the same instant. Intended for tests.
type FixedClock struct{ T time.Time }

// Now returns the fixed instant.
func (c FixedClock) Now() time.Time { return c.T }

// Bucketer converts instants into day keys in one fixed location.
type Bucketer struct {
	loc *time.Location
}

// NewBucketer returns a Bucketer for the named IANA timezone. An empty name
// resolves to UTC rather than the process-local zone: defaulting to local would
// make stored day keys depend on where the binary happened to run.
func NewBucketer(tzName string) (*Bucketer, error) {
	if tzName == "" {
		return &Bucketer{loc: time.UTC}, nil
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", tzName, err)
	}
	return &Bucketer{loc: loc}, nil
}

// Location returns the timezone this Bucketer buckets in.
func (b *Bucketer) Location() *time.Location { return b.loc }

// Day renders an instant as a day key in the Bucketer's location.
func (b *Bucketer) Day(t time.Time) string {
	return t.In(b.loc).Format(DayFormat)
}

// DayFromEpochMillis renders an epoch-millisecond timestamp as a day key.
// Sources that expose epoch milliseconds should always be bucketed through
// this rather than through any date string they also provide.
func (b *Bucketer) DayFromEpochMillis(ms int64) string {
	return b.Day(time.UnixMilli(ms))
}

// ParseDay interprets a day key as midnight in the Bucketer's location.
func (b *Bucketer) ParseDay(day string) (time.Time, error) {
	t, err := time.ParseInLocation(DayFormat, day, b.loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse day %q: %w", day, err)
	}
	return t, nil
}

// StatusEvent is one source-reported status change for a book.
type StatusEvent struct {
	Key        string    // ASIN or other source key
	OccurredAt time.Time // the reported status-change instant
	Finished   bool      // whether the source reports it as finished
}

// BulkClusterOptions tunes bulk-artifact detection.
type BulkClusterOptions struct {
	// Window is the span within which events count as the same cluster.
	Window time.Duration
	// MinSize is the number of events in a window needed to call it bulk.
	MinSize int
}

// DefaultBulkClusterOptions is deliberately strict. Genuine finishes are
// separated by hours at minimum; a migration writes dozens of rows inside a few
// milliseconds. A one-second window with a threshold of five keeps real
// same-evening finishes intact while catching mass-marking events.
var DefaultBulkClusterOptions = BulkClusterOptions{
	Window:  time.Second,
	MinSize: 5,
}

// DetectBulkClusters returns the set of event keys that belong to a bulk
// status-change cluster.
//
// A cluster is a run of events whose timestamps all fall within Window of the
// run's first event. Such runs are produced by marking a shelf of books
// finished at once, or by an account migration, and must not be mistaken for
// evidence that those books were genuinely finished at that moment.
//
// Events with a zero timestamp are ignored: absence of a timestamp is not
// evidence of bulk marking.
func DetectBulkClusters(events []StatusEvent, opts BulkClusterOptions) map[string]bool {
	if opts.Window <= 0 {
		opts.Window = DefaultBulkClusterOptions.Window
	}
	if opts.MinSize <= 0 {
		opts.MinSize = DefaultBulkClusterOptions.MinSize
	}

	dated := make([]StatusEvent, 0, len(events))
	for _, e := range events {
		if !e.OccurredAt.IsZero() {
			dated = append(dated, e)
		}
	}
	sort.Slice(dated, func(i, j int) bool {
		return dated[i].OccurredAt.Before(dated[j].OccurredAt)
	})

	flagged := make(map[string]bool)
	for i := 0; i < len(dated); {
		j := i + 1
		for j < len(dated) && dated[j].OccurredAt.Sub(dated[i].OccurredAt) <= opts.Window {
			j++
		}
		if j-i >= opts.MinSize {
			for _, e := range dated[i:j] {
				flagged[e.Key] = true
			}
		}
		// Advance past the cluster we just closed, not just one event, so a
		// long dense run is reported once rather than re-scanned per element.
		if j-i > 1 {
			i = j
		} else {
			i++
		}
	}
	return flagged
}
