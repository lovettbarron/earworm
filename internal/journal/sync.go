package journal

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/lovettbarron/earworm/internal/db"
)

// Syncer renders entries and delivers them to a journal.
type Syncer struct {
	DB     *sql.DB
	Writer Writer
	Logger *slog.Logger

	// JournalID is the destination journal.
	JournalID string
	// DryRun renders and reports without writing anything. It defaults to
	// true at the CLI layer: this is the only part of earworm that writes to
	// an irreplaceable personal record.
	DryRun bool
	// IncludeFinishes additionally emits an entry per genuine finish event.
	IncludeFinishes bool
	// EstimatedFinishes also recovers finishes whose date must be estimated.
	EstimatedFinishes bool
}

// SyncResult reports what a run did or would do.
type SyncResult struct {
	Rendered  int     `json:"rendered"`
	Written   int     `json:"written"`
	Updated   int     `json:"updated"`
	Unchanged int     `json:"unchanged"`
	DryRun    bool    `json:"dry_run"`
	Entries   []Entry `json:"-"`
}

func (s *Syncer) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// Sync renders entries from stored data and writes the ones that changed.
//
// Entries whose rendered text matches what was written before are skipped, so
// a repeated run is a no-op rather than a stream of pointless rewrites.
func (s *Syncer) Sync(ctx context.Context, opts BuildOptions) (SyncResult, error) {
	res := SyncResult{DryRun: s.DryRun}

	sessions, err := db.ListListeningSessions(s.DB)
	if err != nil {
		return res, err
	}
	entries, err := BuildDayEntries(sessions, opts)
	if err != nil {
		return res, err
	}

	if s.IncludeFinishes || opts.IncludeFinishes {
		opts.EstimatedFinishes = opts.EstimatedFinishes || s.EstimatedFinishes
		books, err := db.ListBookListening(s.DB, "")
		if err != nil {
			return res, err
		}
		finishes, err := BuildFinishEntries(books, opts)
		if err != nil {
			return res, err
		}
		entries = append(entries, finishes...)
	}

	res.Rendered = len(entries)
	res.Entries = entries

	if s.DryRun {
		// Still report what would change, so a dry run is informative rather
		// than just a count of everything.
		for _, e := range entries {
			prior, ok, err := db.GetJournalEntry(s.DB, e.Key)
			if err != nil {
				return res, err
			}
			switch {
			case !ok:
				res.Written++
			case prior.ContentHash != e.ContentHash():
				res.Updated++
			default:
				res.Unchanged++
			}
		}
		return res, nil
	}

	if s.JournalID == "" {
		return res, fmt.Errorf("journal.journal_id is not configured")
	}
	if s.Writer == nil {
		return res, fmt.Errorf("no journal writer configured")
	}

	var wrote bool
	for _, e := range entries {
		prior, ok, err := db.GetJournalEntry(s.DB, e.Key)
		if err != nil {
			return res, err
		}
		if ok && prior.ContentHash == e.ContentHash() && prior.JournalID == s.JournalID {
			res.Unchanged++
			continue
		}

		if err := s.Writer.Write(ctx, s.JournalID, e); err != nil {
			return res, fmt.Errorf("write entry %s: %w", e.Key, err)
		}
		wrote = true

		// The ledger is updated only after a successful write, so an
		// interrupted run retries the entry rather than believing it landed.
		if err := db.UpsertJournalEntry(s.DB, db.JournalEntry{
			EntryKey:    e.Key,
			Kind:        e.Kind,
			EntryID:     e.ID,
			JournalID:   s.JournalID,
			EntryDate:   e.Date.Format("2006-01-02"),
			ContentHash: e.ContentHash(),
		}); err != nil {
			return res, err
		}

		if ok {
			res.Updated++
		} else {
			res.Written++
		}
	}

	if wrote {
		// The CLI queues writes locally; without this they never leave the
		// machine. Its failure is reported distinctly because the entries did
		// get written, just not pushed.
		if err := s.Writer.Flush(ctx); err != nil {
			return res, fmt.Errorf("entries written but sync to Day One failed: %w", err)
		}
	}

	s.logger().Debug("journal sync complete",
		"rendered", res.Rendered, "written", res.Written,
		"updated", res.Updated, "unchanged", res.Unchanged)

	return res, nil
}
