package statsexport

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/lovettbarron/earworm/internal/bookidentity"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

// File names written by an export.
const (
	BooksFile    = "books.csv"
	DaysFile     = "days.csv"
	SessionsFile = "sessions.csv"
	ReadingFile  = "reading.csv"
	TimelineFile = "timeline.csv"
	ReadmeFile   = "README.md"
)

// Dataset is everything an export needs, already read from the database.
type Dataset struct {
	Books      []db.BookListening
	Days       []db.ListeningDay
	Sessions   []db.ListeningSession
	Identities []bookidentity.Identity
}

// Options controls what an export writes.
type Options struct {
	// Dir is the output directory. It is created if missing.
	Dir string
	// Timeline additionally writes the denormalised timeline.csv.
	Timeline bool
	// Bucket supplies the timezone for any date arithmetic.
	Bucket *listening.Bucketer
}

// Result reports what was written.
type Result struct {
	Dir          string
	Files        []string
	Books        int
	Days         int
	Sessions     int
	Reading      int
	TimelineRows int
	Stats        AllocationStats
}

// Export writes the dataset as CSV files into opts.Dir.
//
// Three normalised files are always written, plus an optional denormalised
// timeline. The normalised set is the source of truth; timeline.csv is derived
// from it in this same pass, so the two cannot drift apart.
func Export(data Dataset, opts Options) (Result, error) {
	var res Result
	if opts.Dir == "" {
		return res, fmt.Errorf("export directory is not set")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return res, fmt.Errorf("create export directory: %w", err)
	}
	res.Dir = opts.Dir

	// Map every source record to its identity so each row can carry it.
	identityBySource := make(map[string]bookidentity.Identity)
	methodBySource := make(map[string]string)
	for _, id := range data.Identities {
		for _, m := range id.Mappings {
			key := m.Source + "\x00" + m.SourceKey
			identityBySource[key] = id
			methodBySource[key] = m.Method
		}
	}

	rows, err := buildBookRows(data, identityBySource, methodBySource)
	if err != nil {
		return res, err
	}
	if err := writeCSV(filepath.Join(opts.Dir, BooksFile), bookHeader, rows); err != nil {
		return res, err
	}
	res.Books = len(rows)
	res.Files = append(res.Files, BooksFile)

	exact := exactAllocations(data, identityBySource)
	allocations := allocateDataset(data, identityBySource, opts.Bucket)
	dayRows := buildDayRows(exact, allocations)
	if err := writeCSV(filepath.Join(opts.Dir, DaysFile), dayHeader, dayRows); err != nil {
		return res, err
	}
	res.Days = len(dayRows)
	res.Files = append(res.Files, DaysFile)
	// Summarise over measured and inferred together, so the reported
	// "measured" figure is not silently zero.
	res.Stats = Summarise(append(append([]Allocation{}, exact...), allocations...))

	sessionRows := buildSessionRows(data, identityBySource)
	if err := writeCSV(filepath.Join(opts.Dir, SessionsFile), sessionHeader, sessionRows); err != nil {
		return res, err
	}
	res.Sessions = len(sessionRows)
	res.Files = append(res.Files, SessionsFile)

	readingRows := buildReadingRows(data, identityBySource)
	if err := writeCSV(filepath.Join(opts.Dir, ReadingFile), readingHeader, readingRows); err != nil {
		return res, err
	}
	res.Reading = len(readingRows)
	res.Files = append(res.Files, ReadingFile)

	if opts.Timeline {
		timelineRows := buildTimelineRows(data, exact, allocations, identityBySource)
		if err := writeCSV(filepath.Join(opts.Dir, TimelineFile), timelineHeader, timelineRows); err != nil {
			return res, err
		}
		res.TimelineRows = len(timelineRows)
		res.Files = append(res.Files, TimelineFile)
	}

	if err := writeReadme(filepath.Join(opts.Dir, ReadmeFile), res); err != nil {
		return res, err
	}
	res.Files = append(res.Files, ReadmeFile)

	return res, nil
}

var bookHeader = []string{
	"identity_id", "asin", "title", "author", "narrator", "series",
	"series_position", "genres", "runtime_seconds", "seconds_listened",
	"percent_complete", "is_finished", "finished_at", "finish_is_bulk",
	"last_heard_at", "purchase_date", "added_date", "sources", "match_method",
}

func buildBookRows(data Dataset, identityBySource map[string]bookidentity.Identity,
	methodBySource map[string]string) ([][]string, error) {

	// Merge each identity's per-source rows into one book row.
	byIdentity := make(map[string]*db.BookListening)
	secondsByIdentity := make(map[string]int)
	idByIdentity := make(map[string]bookidentity.Identity)

	for i := range data.Books {
		b := data.Books[i]
		key := b.Source + "\x00" + b.SourceKey
		id, ok := identityBySource[key]
		if !ok {
			// A book with no identity would silently vanish from the export,
			// so give it one of its own rather than dropping it.
			id = bookidentity.Identity{
				ID: b.Source + ":" + b.SourceKey, ASIN: b.ASIN,
				Title: b.Title, Author: b.Author,
			}
		}
		idByIdentity[id.ID] = id
		secondsByIdentity[id.ID] += b.SecondsListened

		existing, seen := byIdentity[id.ID]
		if !seen {
			copyB := b
			byIdentity[id.ID] = &copyB
			continue
		}
		mergeBookInto(existing, b)
	}

	ids := make([]string, 0, len(byIdentity))
	for id := range byIdentity {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	rows := make([][]string, 0, len(ids))
	for _, idKey := range ids {
		b := byIdentity[idKey]
		id := idByIdentity[idKey]
		key := b.Source + "\x00" + b.SourceKey

		rows = append(rows, []string{
			idKey,
			firstNonEmpty(id.ASIN, b.ASIN),
			firstNonEmpty(id.Title, b.Title),
			firstNonEmpty(id.Author, b.Author),
			b.Narrator,
			b.Series,
			b.SeriesPosition,
			b.Genres,
			strconv.Itoa(b.RuntimeSeconds),
			strconv.Itoa(secondsByIdentity[idKey]),
			strconv.FormatFloat(b.PercentComplete, 'f', 1, 64),
			strconv.FormatBool(b.IsFinished),
			b.StatusChangedAt,
			strconv.FormatBool(b.StatusIsBulk),
			b.LastPositionAt,
			b.PurchaseDate,
			b.AddedDate,
			joinSources(id),
			firstNonEmpty(methodBySource[key], bookidentity.MatchNone),
		})
	}
	return rows, nil
}

// mergeBookInto folds a second source's view into an existing row, preferring
// non-empty values rather than letting one source blank out another's data.
func mergeBookInto(dst *db.BookListening, src db.BookListening) {
	if dst.ASIN == "" {
		dst.ASIN = src.ASIN
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.Author == "" {
		dst.Author = src.Author
	}
	if dst.Narrator == "" {
		dst.Narrator = src.Narrator
	}
	if dst.Series == "" {
		dst.Series = src.Series
		dst.SeriesPosition = src.SeriesPosition
	}
	if dst.Genres == "" {
		dst.Genres = src.Genres
	}
	if dst.RuntimeSeconds == 0 {
		dst.RuntimeSeconds = src.RuntimeSeconds
	}
	if src.PercentComplete > dst.PercentComplete {
		dst.PercentComplete = src.PercentComplete
	}
	if src.IsFinished {
		dst.IsFinished = true
	}
	if dst.StatusChangedAt == "" {
		dst.StatusChangedAt = src.StatusChangedAt
		dst.StatusIsBulk = src.StatusIsBulk
	}
	if src.LastPositionAt > dst.LastPositionAt {
		dst.LastPositionAt = src.LastPositionAt
	}
	if dst.PurchaseDate == "" {
		dst.PurchaseDate = src.PurchaseDate
	}
	if dst.AddedDate == "" {
		dst.AddedDate = src.AddedDate
	}
}

var dayHeader = []string{
	"day", "source", "seconds", "identity_id", "title", "attribution",
}

// exactAllocations turns session data into measured per-day, per-book rows.
//
// These are the only rows that can honestly be called measured: a session
// names the book it played. They are produced here, rather than inside
// buildDayRows, so that the exported files and the reported statistics are
// computed from one list and cannot disagree.
func exactAllocations(data Dataset, identityBySource map[string]bookidentity.Identity) []Allocation {
	type key struct{ day, identity string }
	totals := make(map[key]int)
	titles := make(map[string]string)

	for _, s := range data.Sessions {
		if s.Day == "" {
			continue
		}
		id := s.LibraryItemID
		if id == "" {
			id = s.BookID
		}
		if resolved, ok := identityBySource[listening.SourceABS+"\x00"+id]; ok {
			id = resolved.ID
		}
		totals[key{s.Day, id}] += s.Seconds
		if s.Title != "" && titles[id] == "" {
			titles[id] = s.Title
		}
	}

	keys := make([]key, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].day != keys[j].day {
			return keys[i].day < keys[j].day
		}
		return keys[i].identity < keys[j].identity
	})

	out := make([]Allocation, 0, len(keys))
	for _, k := range keys {
		out = append(out, Allocation{
			Day:         k.day,
			IdentityID:  k.identity,
			Title:       titles[k.identity],
			Seconds:     totals[k],
			Attribution: AttributionExact,
		})
	}
	return out
}

func buildDayRows(exact, allocations []Allocation) [][]string {
	rows := make([][]string, 0, len(exact)+len(allocations))
	for _, a := range exact {
		rows = append(rows, []string{
			a.Day, listening.SourceABS, strconv.Itoa(a.Seconds),
			a.IdentityID, a.Title, a.Attribution,
		})
	}
	for _, a := range allocations {
		rows = append(rows, []string{
			a.Day, listening.SourceAudible, strconv.Itoa(a.Seconds),
			a.IdentityID, a.Title, a.Attribution,
		})
	}
	return rows
}

var sessionHeader = []string{
	"session_id", "day", "identity_id", "asin", "title", "author",
	"seconds", "duration_seconds", "started_at", "updated_at", "device",
}

func buildSessionRows(data Dataset, identityBySource map[string]bookidentity.Identity) [][]string {
	rows := make([][]string, 0, len(data.Sessions))
	for _, s := range data.Sessions {
		key := listening.SourceABS + "\x00" + s.LibraryItemID
		identityID := s.LibraryItemID
		if id, ok := identityBySource[key]; ok {
			identityID = id.ID
		}
		rows = append(rows, []string{
			s.ID, s.Day, identityID, s.ASIN, s.Title, s.Author,
			strconv.Itoa(s.Seconds), strconv.Itoa(s.DurationSeconds),
			s.StartedAt, s.UpdatedAt, s.Device,
		})
	}
	return rows
}

var timelineHeader = []string{
	"day", "source", "identity_id", "asin", "title", "author", "series",
	"genres", "seconds", "attribution",
}

// buildTimelineRows denormalises the day rows with book metadata.
//
// Derived from the same in-memory data as the normalised files rather than by
// re-reading them, so the two cannot disagree.
func buildTimelineRows(data Dataset, exact, allocations []Allocation,
	identityBySource map[string]bookidentity.Identity) [][]string {

	meta := make(map[string]db.BookListening)
	for _, b := range data.Books {
		key := b.Source + "\x00" + b.SourceKey
		id, ok := identityBySource[key]
		identityID := b.Source + ":" + b.SourceKey
		if ok {
			identityID = id.ID
		}
		if existing, seen := meta[identityID]; !seen || existing.Genres == "" {
			meta[identityID] = b
		}
	}

	var rows [][]string

	for _, a := range exact {
		b := meta[a.IdentityID]
		title := firstNonEmpty(a.Title, b.Title)
		rows = append(rows, []string{
			a.Day, listening.SourceABS, a.IdentityID, b.ASIN, title, b.Author,
			b.Series, b.Genres, strconv.Itoa(a.Seconds), AttributionExact,
		})
	}

	for _, a := range allocations {
		b := meta[a.IdentityID]
		title := firstNonEmpty(a.Title, b.Title)
		rows = append(rows, []string{
			a.Day, listening.SourceAudible, a.IdentityID, b.ASIN, title,
			b.Author, b.Series, b.Genres, strconv.Itoa(a.Seconds), a.Attribution,
		})
	}
	return rows
}

// allocateDataset builds allocation candidates from stored evidence and runs
// the allocation over Audible's day totals.
func allocateDataset(data Dataset, identityBySource map[string]bookidentity.Identity,
	bucket *listening.Bucketer) []Allocation {

	var days []DayTotal
	for _, d := range data.Days {
		if d.Source == listening.SourceAudible {
			days = append(days, DayTotal{Day: d.Day, Seconds: d.Seconds})
		}
	}
	if len(days) == 0 {
		return nil
	}

	var candidates []AllocCandidate
	for _, b := range data.Books {
		if b.Source != listening.SourceAudible {
			continue
		}

		end := latestTime(b.LastPositionAt, b.StatusChangedAt)
		if end.IsZero() {
			continue
		}

		// Estimated listening: percent complete against runtime. Without both
		// there is nothing to allocate, so the book is skipped rather than
		// guessed at.
		pct := b.PercentComplete
		if b.IsFinished && pct < 100 {
			pct = 100
		}
		if pct <= 0 || b.RuntimeSeconds <= 0 {
			continue
		}
		seconds := int(float64(b.RuntimeSeconds) * pct / 100.0)
		if seconds <= 0 {
			continue
		}

		key := b.Source + "\x00" + b.SourceKey
		identityID := b.Source + ":" + b.SourceKey
		title := b.Title
		if id, ok := identityBySource[key]; ok {
			identityID = id.ID
			title = firstNonEmpty(id.Title, title)
		}

		candidates = append(candidates, AllocCandidate{
			IdentityID:   identityID,
			Title:        title,
			EndDate:      end,
			EarliestDate: parseAnyTime(firstNonEmpty(b.AddedDate, b.PurchaseDate)),
			Seconds:      seconds,
		})
	}

	return Allocate(days, candidates)
}

func writeCSV(path string, header []string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write header to %s: %w", filepath.Base(path), err)
	}
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			return fmt.Errorf("write row to %s: %w", filepath.Base(path), err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("flush %s: %w", filepath.Base(path), err)
	}
	return f.Sync()
}

// writeReadme documents the dataset for whoever (or whatever) reads it next.
//
// An LLM handed these files has no other way to learn that some rows are
// reconstructions, and that distinction is the whole point of the export.
func writeReadme(path string, res Result) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create README: %w", err)
	}
	defer f.Close()
	return writeReadmeTo(f, res)
}

func writeReadmeTo(w io.Writer, res Result) error {
	_, err := fmt.Fprintf(w, `# Listening history export

Generated by earworm on %s.

## Files

- `+"`books.csv`"+` — one row per book, merged across sources.
- `+"`days.csv`"+` — listening per day, per book where known.
- `+"`sessions.csv`"+` — individual playback sessions (Audiobookshelf only).
- `+"`timeline.csv`"+` — the day rows denormalised with book metadata, if requested.

## Reading this data honestly

The `+"`attribution`"+` column states how a row's book assignment was arrived at.
It is not a confidence score to be averaged away — the categories mean
different things:

- `+"`exact`"+` — a playback session recorded this book on this day. Measured.
- `+"`inferred-single`"+` — no session data exists; one candidate book plausibly
  accounts for the day, based on when it was finished or last played.
  Reconstructed, not observed.
- `+"`inferred-split`"+` — as above, but several books shared the day and the
  time was divided between them. Weaker still.
- `+"`unattributed`"+` — time was recorded but no book could be assigned. The
  duration is real; the book is unknown.

Audible reports how long was listened but never to what, so most rows predating
Audiobookshelf are inferred or unattributed. Treat inferred rows as a hypothesis
about which book the time belonged to, not as evidence.

Two further caveats:

- `+"`finished_at`"+` records a status CHANGE, which can include marking a book
  UNfinished. Where `+"`finish_is_bulk`"+` is true the timestamp belongs to a
  cluster of books marked at once — a migration artifact, not a real finish.
- `+"`percent_complete`"+` for Audiobookshelf books is derived from session time
  against duration, and can exceed a single read-through when a book was
  re-listened to.

## Summary

- Books: %d
- Day rows: %d
- Sessions: %d
- Measured: %.1f hours
- Inferred: %.1f hours
- Unattributed: %.1f hours
`,
		time.Now().Format("2006-01-02"),
		res.Books, res.Days, res.Sessions,
		float64(res.Stats.ExactSeconds)/3600.0,
		float64(res.Stats.InferredSeconds)/3600.0,
		float64(res.Stats.UnattributedSeconds)/3600.0,
	)
	return err
}

func joinSources(id bookidentity.Identity) string {
	srcs := id.Sources()
	out := ""
	for i, s := range srcs {
		if i > 0 {
			out += "+"
		}
		out += s
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

// latestTime returns the most recent parseable timestamp among its arguments.
func latestTime(values ...string) time.Time {
	var best time.Time
	for _, v := range values {
		if t := parseAnyTime(v); t.After(best) {
			best = t
		}
	}
	return best
}

// parseAnyTime accepts the timestamp shapes stored by the ingestors.
func parseAnyTime(s string) time.Time {
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

var readingHeader = []string{
	"day", "identity_id", "source", "series", "volume", "title", "author",
	"genres", "completed_at", "reliable",
}

// buildReadingRows emits one row per finished volume.
//
// Reading is the counterpart to sessions.csv, not to days.csv: a completion is
// a dated event naming the work, but it carries no duration, so it cannot share
// a column denominated in seconds.
//
// Unreliable rows are included and marked rather than dropped. They are real
// records of a library's state even when their date came from a migration
// rather than from reading, and a reader that can see the flag can decide.
func buildReadingRows(data Dataset, identityBySource map[string]bookidentity.Identity) [][]string {
	var rows [][]string

	for _, b := range data.Books {
		if b.Source != listening.SourceKomga || !b.IsFinished {
			continue
		}
		t := parseAnyTime(b.StatusChangedAt)
		if t.IsZero() {
			continue
		}

		identityID := b.Source + ":" + b.SourceKey
		if id, ok := identityBySource[b.Source+"\x00"+b.SourceKey]; ok {
			identityID = id.ID
		}

		rows = append(rows, []string{
			t.Format("2006-01-02"),
			identityID,
			b.Source,
			b.Series,
			b.SeriesPosition,
			b.Title,
			b.Author,
			b.Genres,
			b.StatusChangedAt,
			strconv.FormatBool(!b.StatusIsBulk),
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i][0] != rows[j][0] {
			return rows[i][0] < rows[j][0]
		}
		return rows[i][1] < rows[j][1]
	})
	return rows
}
