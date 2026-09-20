// Package journal renders listening history as journal entries and writes them
// to an external journaling CLI.
//
// Only measured facts reach a journal. A journal is a personal record that its
// owner will trust years later, so reconstructed attribution — which explains
// most pre-session listening — is deliberately excluded. It stays in the CSV
// export, where the attribution column makes its status explicit.
package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lovettbarron/earworm/internal/db"
)

// Entry kinds.
const (
	// KindDay is a digest of one day's listening, built from sessions.
	KindDay = "day"
	// KindFinish records that a book was finished on a given date.
	KindFinish = "finish"
)

// Entry is a rendered journal entry awaiting delivery.
type Entry struct {
	// Key identifies the entry across runs, e.g. "day:2026-09-20".
	Key string
	// ID is the deterministic identifier handed to the journal so that
	// rewriting the same key updates in place rather than duplicating.
	ID   string
	Kind string
	// Date is the date the entry is filed under.
	Date time.Time
	Body string
}

// ContentHash fingerprints the body so an unchanged entry can be skipped.
func (e Entry) ContentHash() string {
	sum := sha256.Sum256([]byte(e.Body))
	return hex.EncodeToString(sum[:])
}

// EntryID derives a stable identifier from an entry key.
//
// Day One accepts a caller-supplied entry id of 32 hexadecimal characters and
// treats a repeat as an update. Deriving it from the key means a re-run needs
// no bookkeeping to find the previous entry, and an interrupted run cannot
// leave a duplicate behind.
//
// SHA-256 truncated to 32 hex characters, rather than MD5, to keep to the one
// hash this codebase already uses.
func EntryID(key string) string {
	sum := sha256.Sum256([]byte("earworm:journal:" + key))
	return strings.ToUpper(hex.EncodeToString(sum[:]))[:32]
}

// DayKey returns the entry key for a day digest.
func DayKey(day string) string { return "day:" + day }

// FinishKey returns the entry key for a book-finished entry.
func FinishKey(identityID string) string { return "finish:" + identityID }

// BuildOptions controls entry rendering.
type BuildOptions struct {
	// IncludeFinishes emits an entry for each genuine finish event.
	IncludeFinishes bool
	// Since limits entries to dates at or after this day (YYYY-MM-DD).
	Since string
	// Until limits entries to dates at or before this day.
	Until string
}

// BuildDayEntries renders one digest per day that has session data.
//
// Sessions are the only per-day evidence that names a book, so these are the
// only day entries produced. Days with no listening produce nothing: an entry
// saying nothing happened is noise in a journal.
func BuildDayEntries(sessions []db.ListeningSession, opts BuildOptions) ([]Entry, error) {
	type bookTotal struct {
		title   string
		author  string
		seconds int
		count   int
	}
	byDay := make(map[string]map[string]*bookTotal)

	for _, s := range sessions {
		if s.Day == "" || s.Seconds <= 0 {
			continue
		}
		if opts.Since != "" && s.Day < opts.Since {
			continue
		}
		if opts.Until != "" && s.Day > opts.Until {
			continue
		}

		key := s.LibraryItemID
		if key == "" {
			key = s.BookID
		}
		if key == "" {
			key = s.Title
		}

		if byDay[s.Day] == nil {
			byDay[s.Day] = make(map[string]*bookTotal)
		}
		bt := byDay[s.Day][key]
		if bt == nil {
			bt = &bookTotal{title: s.Title, author: s.Author}
			byDay[s.Day][key] = bt
		}
		bt.seconds += s.Seconds
		bt.count++
		if bt.title == "" {
			bt.title = s.Title
		}
		if bt.author == "" {
			bt.author = s.Author
		}
	}

	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)

	entries := make([]Entry, 0, len(days))
	for _, day := range days {
		books := byDay[day]

		keys := make([]string, 0, len(books))
		total := 0
		for k, b := range books {
			keys = append(keys, k)
			total += b.seconds
		}
		if total <= 0 {
			continue
		}
		sort.Slice(keys, func(i, j int) bool {
			if books[keys[i]].seconds != books[keys[j]].seconds {
				return books[keys[i]].seconds > books[keys[j]].seconds
			}
			return keys[i] < keys[j]
		})

		var b strings.Builder
		fmt.Fprintf(&b, "## Listening — %s\n\n", day)
		fmt.Fprintf(&b, "**%s** across %s.\n\n", formatDuration(total), pluralise(len(books), "book"))

		for _, k := range keys {
			bt := books[k]
			title := bt.title
			if title == "" {
				title = "Unknown title"
			}
			if bt.author != "" {
				fmt.Fprintf(&b, "- **%s** — %s\n", title, bt.author)
			} else {
				fmt.Fprintf(&b, "- **%s**\n", title)
			}
			fmt.Fprintf(&b, "  - %s across %s\n",
				formatDuration(bt.seconds), pluralise(bt.count, "session"))
		}

		b.WriteString("\n*Recorded by earworm from Audiobookshelf playback sessions.*\n")

		parsed, err := time.Parse("2006-01-02", day)
		if err != nil {
			return nil, fmt.Errorf("parse day %q: %w", day, err)
		}

		entries = append(entries, Entry{
			Key:  DayKey(day),
			ID:   EntryID(DayKey(day)),
			Kind: KindDay,
			Date: parsed,
			Body: b.String(),
		})
	}
	return entries, nil
}

// BuildFinishEntries renders one entry per genuine book finish.
//
// Books whose finish timestamp belongs to a bulk-marking cluster are skipped:
// that timestamp records when a shelf of books was marked at once, not when any
// of them was read, and writing it as a finish date would put a fiction in the
// journal.
func BuildFinishEntries(books []db.BookListening, opts BuildOptions) ([]Entry, error) {
	var entries []Entry

	sorted := make([]db.BookListening, len(books))
	copy(sorted, books)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StatusChangedAt != sorted[j].StatusChangedAt {
			return sorted[i].StatusChangedAt < sorted[j].StatusChangedAt
		}
		return sorted[i].SourceKey < sorted[j].SourceKey
	})

	for _, bk := range sorted {
		if !bk.IsFinished || bk.StatusIsBulk || bk.StatusChangedAt == "" {
			continue
		}

		finished := parseTimestamp(bk.StatusChangedAt)
		if finished.IsZero() {
			continue
		}
		day := finished.Format("2006-01-02")
		if opts.Since != "" && day < opts.Since {
			continue
		}
		if opts.Until != "" && day > opts.Until {
			continue
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## Finished — %s\n\n", bk.Title)
		if bk.Author != "" {
			fmt.Fprintf(&b, "by %s", bk.Author)
			if bk.Narrator != "" {
				fmt.Fprintf(&b, ", narrated by %s", bk.Narrator)
			}
			b.WriteString("\n\n")
		}
		if bk.Series != "" {
			fmt.Fprintf(&b, "- Series: %s", bk.Series)
			if bk.SeriesPosition != "" {
				fmt.Fprintf(&b, " #%s", bk.SeriesPosition)
			}
			b.WriteString("\n")
		}
		if bk.Genres != "" {
			fmt.Fprintf(&b, "- Genres: %s\n", strings.ReplaceAll(bk.Genres, ",", ", "))
		}
		if bk.RuntimeSeconds > 0 {
			fmt.Fprintf(&b, "- Runtime: %s\n", formatDuration(bk.RuntimeSeconds))
		}
		if bk.PurchaseDate != "" {
			if p := parseTimestamp(bk.PurchaseDate); !p.IsZero() {
				fmt.Fprintf(&b, "- Acquired: %s\n", p.Format("2006-01-02"))
			}
		}

		b.WriteString("\n*Recorded by earworm from an Audible finish event.*\n")

		key := FinishKey(bk.Source + ":" + bk.SourceKey)
		entries = append(entries, Entry{
			Key:  key,
			ID:   EntryID(key),
			Kind: KindFinish,
			Date: finished,
			Body: b.String(),
		})
	}
	return entries, nil
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "0m"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h > 0 {
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m == 0 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm", m)
}

func pluralise(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05", "2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
