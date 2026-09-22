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
	"strconv"
	"strings"
	"time"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

// NearCompleteThreshold is the percentage at which a book that was never
// explicitly marked finished is treated as completed.
//
// Set high: a book abandoned in its final chapter is rare, whereas one
// abandoned at 80% is not, and claiming a finish that did not happen is the
// error that matters in a journal.
const NearCompleteThreshold = 95.0

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
	// EstimatedFinishes additionally emits entries for books that were
	// completed but have no usable finish timestamp, dating them from the last
	// playback position. See BuildFinishEntries for what that date means.
	EstimatedFinishes bool
	// Since limits entries to dates at or after this day (YYYY-MM-DD).
	Since string
	// Until limits entries to dates at or before this day.
	Until string
}

// ReadingCompletion is one volume finished on a given day.
//
// Reading has no duration to report: a source that records a completion does
// not record how long it took, so a day's reading is counted in volumes while
// listening is measured in time.
type ReadingCompletion struct {
	Day    string
	Title  string
	Series string
	Volume string
	Author string
}

// BuildDayEntries renders one digest per day with listening or reading.
//
// Both are measured evidence that names the work: a playback session says what
// was played, a completion says what was finished. Days with neither produce
// nothing — an entry saying nothing happened is noise in a journal.
func BuildDayEntries(sessions []db.ListeningSession, reading []ReadingCompletion, opts BuildOptions) ([]Entry, error) {
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

	readingByDay := make(map[string][]ReadingCompletion)
	for _, r := range reading {
		if r.Day == "" {
			continue
		}
		if opts.Since != "" && r.Day < opts.Since {
			continue
		}
		if opts.Until != "" && r.Day > opts.Until {
			continue
		}
		readingByDay[r.Day] = append(readingByDay[r.Day], r)
	}

	dayset := make(map[string]struct{}, len(byDay)+len(readingByDay))
	for d := range byDay {
		dayset[d] = struct{}{}
	}
	for d := range readingByDay {
		dayset[d] = struct{}{}
	}
	days := make([]string, 0, len(dayset))
	for d := range dayset {
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
		sort.Slice(keys, func(i, j int) bool {
			if books[keys[i]].seconds != books[keys[j]].seconds {
				return books[keys[i]].seconds > books[keys[j]].seconds
			}
			return keys[i] < keys[j]
		})

		read := readingByDay[day]
		if total <= 0 && len(read) == 0 {
			continue
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## %s\n\n", day)

		if total > 0 {
			fmt.Fprintf(&b, "**Listening** — %s across %s\n\n",
				formatDuration(total), pluralise(len(books), "book"))
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
			if len(read) > 0 {
				b.WriteString("\n")
			}
		}

		if len(read) > 0 {
			fmt.Fprintf(&b, "**Reading** — %s\n\n", pluralise(len(read), "volume"))
			for _, line := range summariseReading(read) {
				fmt.Fprintf(&b, "- %s\n", line)
			}
		}

		b.WriteString("\n*Recorded by earworm.*\n")

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

	// Resolve each book's finish date and how well evidenced it is, then sort
	// on the resolved date. Sorting on a single stored column would order the
	// two kinds of entry against different fields.
	type candidate struct {
		book      db.BookListening
		finished  time.Time
		estimated bool
	}
	var candidates []candidate

	for _, bk := range books {
		var when time.Time
		var estimated bool

		switch {
		case bk.IsFinished && !bk.StatusIsBulk && bk.StatusChangedAt != "":
			// A genuine finish event: the source recorded the moment.
			when = parseTimestamp(bk.StatusChangedAt)

		case !opts.EstimatedFinishes:
			continue

		case bk.IsFinished && bk.StatusIsBulk && bk.LastPositionAt != "" &&
			bk.LastPositionAt != bk.StatusChangedAt:
			// Completed, but the finish timestamp was overwritten by a bulk
			// marking. The last playback position keeps its own date, which
			// survives that and is the best remaining evidence.
			//
			// It only counts as evidence when it is INDEPENDENT of the
			// discredited timestamp. Where a source derives both from the same
			// record — as Komga does, having no separate playback clock — the
			// two are identical and "recovering" one from the other would
			// launder a rejected date back into the journal.
			when, estimated = parseTimestamp(bk.LastPositionAt), true

		case !bk.IsFinished && bk.PercentComplete >= NearCompleteThreshold && bk.LastPositionAt != "":
			// Listened to the end but never marked finished.
			when, estimated = parseTimestamp(bk.LastPositionAt), true
		}

		if when.IsZero() {
			continue
		}
		candidates = append(candidates, candidate{book: bk, finished: when, estimated: estimated})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].finished.Equal(candidates[j].finished) {
			return candidates[i].finished.Before(candidates[j].finished)
		}
		return candidates[i].book.SourceKey < candidates[j].book.SourceKey
	})

	for _, c := range candidates {
		bk, finished, estimated := c.book, c.finished, c.estimated
		day := finished.Format("2006-01-02")
		if opts.Since != "" && day < opts.Since {
			continue
		}
		if opts.Until != "" && day > opts.Until {
			continue
		}

		var b strings.Builder
		if estimated {
			fmt.Fprintf(&b, "## Finished (estimated) — %s\n\n", bk.Title)
		} else {
			fmt.Fprintf(&b, "## Finished — %s\n\n", bk.Title)
		}
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

		// Say plainly which part is known and which is inferred, and name the
		// right source: an Audible-worded footer on an Audiobookshelf book
		// would misdescribe where the evidence came from.
		switch {
		case estimated && bk.Source == listening.SourceKomga:
			b.WriteString("\n*Completion inferred by earworm from reading progress; " +
				"the date is the last recorded page turn.*\n")
		case estimated && bk.Source == listening.SourceABS:
			b.WriteString("\n*Completion inferred by earworm from measured playback covering " +
				"most of the runtime; the date is the last session. No finish event was recorded.*\n")
		case estimated:
			b.WriteString("\n*Date estimated by earworm from the last playback position; " +
				"Audible recorded no finish event for this book.*\n")
		case bk.Source == listening.SourceKomga:
			b.WriteString("\n*Recorded by earworm from Komga.*\n")
		case bk.Source == listening.SourceABS:
			b.WriteString("\n*Recorded by earworm from Audiobookshelf.*\n")
		default:
			b.WriteString("\n*Recorded by earworm from an Audible finish event.*\n")
		}

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

// summariseReading renders a day's finished volumes, collapsing a run within
// one series into a single line.
//
// Reading a dozen chapters of one series in an evening is ordinary, and a
// dozen near-identical bullets buries whatever else happened that day.
func summariseReading(reading []ReadingCompletion) []string {
	bySeries := make(map[string][]ReadingCompletion)
	var order []string
	for _, r := range reading {
		key := r.Series
		if key == "" {
			key = r.Title
		}
		if _, seen := bySeries[key]; !seen {
			order = append(order, key)
		}
		bySeries[key] = append(bySeries[key], r)
	}
	sort.Slice(order, func(i, j int) bool {
		if len(bySeries[order[i]]) != len(bySeries[order[j]]) {
			return len(bySeries[order[i]]) > len(bySeries[order[j]])
		}
		return order[i] < order[j]
	})

	out := make([]string, 0, len(order))
	for _, key := range order {
		group := bySeries[key]
		if len(group) == 1 {
			r := group[0]
			line := "**" + firstNonEmptyStr(r.Title, key) + "**"
			if r.Author != "" {
				line += " — " + r.Author
			}
			out = append(out, line)
			continue
		}

		vols := make([]string, 0, len(group))
		for _, r := range group {
			if r.Volume != "" {
				vols = append(vols, r.Volume)
			}
		}
		sort.Slice(vols, func(i, j int) bool { return lessNumeric(vols[i], vols[j]) })

		line := fmt.Sprintf("**%s** — %s", key, pluralise(len(group), "volume"))
		if len(vols) > 0 {
			line += " (" + condenseRange(vols) + ")"
		}
		out = append(out, line)
	}
	return out
}

// condenseRange renders a sorted list of volume numbers compactly.
func condenseRange(vols []string) string {
	if len(vols) <= 2 {
		return strings.Join(vols, ", ")
	}
	first, last := vols[0], vols[len(vols)-1]
	if isContiguous(vols) {
		return first + "–" + last
	}
	return strings.Join(vols, ", ")
}

func isContiguous(vols []string) bool {
	prev, err := strconv.Atoi(strings.TrimLeft(vols[0], "0"))
	if err != nil {
		return false
	}
	for _, v := range vols[1:] {
		n, err := strconv.Atoi(strings.TrimLeft(v, "0"))
		if err != nil || n != prev+1 {
			return false
		}
		prev = n
	}
	return true
}

func lessNumeric(a, b string) bool {
	ai, aerr := strconv.Atoi(strings.TrimLeft(a, "0"))
	bi, berr := strconv.Atoi(strings.TrimLeft(b, "0"))
	if aerr == nil && berr == nil {
		return ai < bi
	}
	return a < b
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
