package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ListeningDay is one source's total listening for one day bucket.
type ListeningDay struct {
	Day     string // YYYY-MM-DD in the configured stats timezone
	Source  string // listening.SourceAudible or listening.SourceABS
	Seconds int
}

// BookListening is one source's view of one book's listening state.
type BookListening struct {
	Source          string
	SourceKey       string
	ASIN            string
	Title           string
	Author          string
	Narrator        string
	Series          string
	SeriesPosition  string
	Genres          string
	RuntimeSeconds  int
	PercentComplete float64
	IsFinished      bool
	PurchaseDate    string
	AddedDate       string
	// StatusChangedAt is the source's last status-change timestamp. It is not
	// proof of completion: it is also written when a book is un-finished.
	StatusChangedAt string
	// StatusIsBulk marks a StatusChangedAt that belongs to a mass-marking
	// cluster and therefore carries no per-book meaning.
	StatusIsBulk    bool
	LastPositionAt  string
	LastPositionMS  int64
	SecondsListened int
}

// UpsertListeningDay records a day total, replacing any previous value for
// that (day, source) pair.
//
// Daily totals are mutable: a source can revise a past day as late-syncing
// clients report in. Replacing rather than accumulating is what makes a
// re-run of the same window a no-op instead of a doubling.
func UpsertListeningDay(db *sql.DB, day, source string, seconds int) error {
	_, err := db.Exec(`
		INSERT INTO listening_days (day, source, seconds, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(day, source) DO UPDATE SET
			seconds = excluded.seconds,
			updated_at = CURRENT_TIMESTAMP`,
		day, source, seconds)
	if err != nil {
		return fmt.Errorf("upsert listening day %s/%s: %w", source, day, err)
	}
	return nil
}

// UpsertListeningDays records many day totals in one transaction.
func UpsertListeningDays(db *sql.DB, days []ListeningDay) error {
	if len(days) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin listening days tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO listening_days (day, source, seconds, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(day, source) DO UPDATE SET
			seconds = excluded.seconds,
			updated_at = CURRENT_TIMESTAMP`)
	if err != nil {
		return fmt.Errorf("prepare listening days upsert: %w", err)
	}
	defer stmt.Close()

	for _, d := range days {
		if _, err := stmt.Exec(d.Day, d.Source, d.Seconds); err != nil {
			return fmt.Errorf("upsert listening day %s/%s: %w", d.Source, d.Day, err)
		}
	}
	return tx.Commit()
}

// ListListeningDays returns stored day totals for a source, ordered by day.
// An empty source returns every source's rows.
func ListListeningDays(db *sql.DB, source string) ([]ListeningDay, error) {
	query := `SELECT day, source, seconds FROM listening_days`
	var args []any
	if source != "" {
		query += ` WHERE source = ?`
		args = append(args, source)
	}
	query += ` ORDER BY day, source`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list listening days: %w", err)
	}
	defer rows.Close()

	var out []ListeningDay
	for rows.Next() {
		var d ListeningDay
		if err := rows.Scan(&d.Day, &d.Source, &d.Seconds); err != nil {
			return nil, fmt.Errorf("scan listening day: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CountListeningDays returns how many day rows a source has stored.
func CountListeningDays(db *sql.DB, source string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM listening_days WHERE source = ?`, source).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count listening days: %w", err)
	}
	return n, nil
}

// UpsertBookListening records one book's per-source listening state.
func UpsertBookListening(db *sql.DB, b BookListening) error {
	return upsertBookListening(db, b)
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

const bookListeningUpsert = `
	INSERT INTO book_listening (
		source, source_key, asin, title, author, narrator, series, series_position,
		genres, runtime_seconds, percent_complete, is_finished, purchase_date,
		added_date, status_changed_at, status_is_bulk, last_position_at,
		last_position_ms, seconds_listened, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(source, source_key) DO UPDATE SET
		asin = excluded.asin,
		title = excluded.title,
		author = excluded.author,
		narrator = excluded.narrator,
		series = excluded.series,
		series_position = excluded.series_position,
		genres = excluded.genres,
		runtime_seconds = excluded.runtime_seconds,
		percent_complete = excluded.percent_complete,
		is_finished = excluded.is_finished,
		purchase_date = excluded.purchase_date,
		added_date = excluded.added_date,
		status_changed_at = excluded.status_changed_at,
		status_is_bulk = excluded.status_is_bulk,
		last_position_at = excluded.last_position_at,
		last_position_ms = excluded.last_position_ms,
		-- A later pass that does not know the measured total must not erase one
		-- an earlier pass established, so keep the existing value when the
		-- incoming row carries none.
		seconds_listened = CASE
			WHEN excluded.seconds_listened > 0 THEN excluded.seconds_listened
			ELSE book_listening.seconds_listened END,
		updated_at = CURRENT_TIMESTAMP`

func upsertBookListening(e execer, b BookListening) error {
	_, err := e.Exec(bookListeningUpsert,
		b.Source, b.SourceKey, b.ASIN, b.Title, b.Author, b.Narrator, b.Series,
		b.SeriesPosition, b.Genres, b.RuntimeSeconds, b.PercentComplete,
		boolToInt(b.IsFinished), b.PurchaseDate, b.AddedDate, b.StatusChangedAt,
		boolToInt(b.StatusIsBulk), b.LastPositionAt, b.LastPositionMS, b.SecondsListened)
	if err != nil {
		return fmt.Errorf("upsert book listening %s/%s: %w", b.Source, b.SourceKey, err)
	}
	return nil
}

// UpsertBookListeningBatch records many books in one transaction.
func UpsertBookListeningBatch(db *sql.DB, books []BookListening) error {
	if len(books) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin book listening tx: %w", err)
	}
	defer tx.Rollback()

	for _, b := range books {
		if _, err := tx.Exec(bookListeningUpsert,
			b.Source, b.SourceKey, b.ASIN, b.Title, b.Author, b.Narrator, b.Series,
			b.SeriesPosition, b.Genres, b.RuntimeSeconds, b.PercentComplete,
			boolToInt(b.IsFinished), b.PurchaseDate, b.AddedDate, b.StatusChangedAt,
			boolToInt(b.StatusIsBulk), b.LastPositionAt, b.LastPositionMS, b.SecondsListened,
		); err != nil {
			return fmt.Errorf("upsert book listening %s/%s: %w", b.Source, b.SourceKey, err)
		}
	}
	return tx.Commit()
}

// ListBookListening returns stored per-book state for a source, ordered by
// source key. An empty source returns every source's rows.
func ListBookListening(db *sql.DB, source string) ([]BookListening, error) {
	query := `
		SELECT source, source_key, asin, title, author, narrator, series,
		       series_position, genres, runtime_seconds, percent_complete,
		       is_finished, purchase_date, added_date, status_changed_at,
		       status_is_bulk, last_position_at, last_position_ms, seconds_listened
		FROM book_listening`
	var args []any
	if source != "" {
		query += ` WHERE source = ?`
		args = append(args, source)
	}
	query += ` ORDER BY source, source_key`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list book listening: %w", err)
	}
	defer rows.Close()

	var out []BookListening
	for rows.Next() {
		var b BookListening
		var finished, bulk int
		if err := rows.Scan(&b.Source, &b.SourceKey, &b.ASIN, &b.Title, &b.Author,
			&b.Narrator, &b.Series, &b.SeriesPosition, &b.Genres, &b.RuntimeSeconds,
			&b.PercentComplete, &finished, &b.PurchaseDate, &b.AddedDate,
			&b.StatusChangedAt, &bulk, &b.LastPositionAt, &b.LastPositionMS,
			&b.SecondsListened); err != nil {
			return nil, fmt.Errorf("scan book listening: %w", err)
		}
		b.IsFinished = finished != 0
		b.StatusIsBulk = bulk != 0
		out = append(out, b)
	}
	return out, rows.Err()
}

// MarkBulkStatus flags the given source keys as belonging to a bulk
// status-change cluster, and clears the flag on every other row of that source.
//
// The clearing half matters: cluster membership is recomputed from the whole
// event set each run, so a row that no longer qualifies must lose the flag.
func MarkBulkStatus(db *sql.DB, source string, keys []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin bulk status tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE book_listening SET status_is_bulk = 0 WHERE source = ?`, source); err != nil {
		return fmt.Errorf("clear bulk status flags: %w", err)
	}

	if len(keys) > 0 {
		// Chunked to stay clear of SQLite's bound-parameter ceiling.
		const chunk = 400
		for start := 0; start < len(keys); start += chunk {
			end := start + chunk
			if end > len(keys) {
				end = len(keys)
			}
			batch := keys[start:end]
			args := make([]any, 0, len(batch)+1)
			args = append(args, source)
			for _, k := range batch {
				args = append(args, k)
			}
			query := fmt.Sprintf(
				`UPDATE book_listening SET status_is_bulk = 1 WHERE source = ? AND source_key IN (%s)`,
				strings.TrimSuffix(strings.Repeat("?,", len(batch)), ","))
			if _, err := tx.Exec(query, args...); err != nil {
				return fmt.Errorf("set bulk status flags: %w", err)
			}
		}
	}
	return tx.Commit()
}

// SetSyncState stores a watermark or cursor value.
func SetSyncState(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO stats_sync_state (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP`, key, value)
	if err != nil {
		return fmt.Errorf("set sync state %s: %w", key, err)
	}
	return nil
}

// GetSyncState returns a stored value and whether it was present.
func GetSyncState(db *sql.DB, key string) (string, bool, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM stats_sync_state WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get sync state %s: %w", key, err)
	}
	return v, true, nil
}

// SetSyncTime stores a timestamp watermark in RFC3339 form.
func SetSyncTime(db *sql.DB, key string, t time.Time) error {
	return SetSyncState(db, key, t.UTC().Format(time.RFC3339Nano))
}

// GetSyncTime returns a stored timestamp watermark. A missing or unparseable
// value yields the zero time with ok false, so callers fall back to a full
// backfill rather than syncing from an arbitrary date.
func GetSyncTime(db *sql.DB, key string) (time.Time, bool, error) {
	v, ok, err := GetSyncState(db, key)
	if err != nil || !ok {
		return time.Time{}, false, err
	}
	t, perr := time.Parse(time.RFC3339Nano, v)
	if perr != nil {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ListeningSession is one exact playback session from Audiobookshelf.
type ListeningSession struct {
	ID              string
	UserID          string
	LibraryItemID   string
	BookID          string
	EpisodeID       string
	MediaType       string
	ASIN            string
	Title           string
	Author          string
	Day             string
	Seconds         int
	DurationSeconds int
	StartSeconds    int
	CurrentSeconds  int
	StartedAt       string
	UpdatedAt       string
	Device          string
}

const listeningSessionUpsert = `
	INSERT INTO listening_sessions (
		id, user_id, library_item_id, book_id, episode_id, media_type, asin,
		title, author, day, seconds, duration_seconds, start_seconds,
		current_seconds, started_at, updated_at, device, synced_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		user_id = excluded.user_id,
		library_item_id = excluded.library_item_id,
		book_id = excluded.book_id,
		episode_id = excluded.episode_id,
		media_type = excluded.media_type,
		asin = excluded.asin,
		title = excluded.title,
		author = excluded.author,
		day = excluded.day,
		seconds = excluded.seconds,
		duration_seconds = excluded.duration_seconds,
		start_seconds = excluded.start_seconds,
		current_seconds = excluded.current_seconds,
		started_at = excluded.started_at,
		updated_at = excluded.updated_at,
		device = excluded.device,
		synced_at = CURRENT_TIMESTAMP`

// UpsertListeningSessions stores sessions, updating any already present.
//
// Sessions are mutable while playback continues, so a session seen on an
// earlier sync can legitimately return with more listening time. Updating in
// place by server id is what keeps a re-sync from double-counting it.
func UpsertListeningSessions(db *sql.DB, sessions []ListeningSession) error {
	if len(sessions) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin sessions tx: %w", err)
	}
	defer tx.Rollback()

	for _, s := range sessions {
		if _, err := tx.Exec(listeningSessionUpsert,
			s.ID, s.UserID, s.LibraryItemID, s.BookID, s.EpisodeID, s.MediaType,
			s.ASIN, s.Title, s.Author, s.Day, s.Seconds, s.DurationSeconds,
			s.StartSeconds, s.CurrentSeconds, s.StartedAt, s.UpdatedAt, s.Device,
		); err != nil {
			return fmt.Errorf("upsert session %s: %w", s.ID, err)
		}
	}
	return tx.Commit()
}

// ListListeningSessions returns every stored session ordered by start time.
func ListListeningSessions(db *sql.DB) ([]ListeningSession, error) {
	rows, err := db.Query(`
		SELECT id, user_id, library_item_id, book_id, episode_id, media_type,
		       asin, title, author, day, seconds, duration_seconds,
		       start_seconds, current_seconds, started_at, updated_at, device
		FROM listening_sessions
		ORDER BY started_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var out []ListeningSession
	for rows.Next() {
		var s ListeningSession
		if err := rows.Scan(&s.ID, &s.UserID, &s.LibraryItemID, &s.BookID,
			&s.EpisodeID, &s.MediaType, &s.ASIN, &s.Title, &s.Author, &s.Day,
			&s.Seconds, &s.DurationSeconds, &s.StartSeconds, &s.CurrentSeconds,
			&s.StartedAt, &s.UpdatedAt, &s.Device); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SessionsByDay returns total listened seconds per day across all sessions.
func SessionsByDay(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`
		SELECT day, SUM(seconds) FROM listening_sessions
		WHERE day != '' GROUP BY day`)
	if err != nil {
		return nil, fmt.Errorf("sessions by day: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var day string
		var secs int
		if err := rows.Scan(&day, &secs); err != nil {
			return nil, fmt.Errorf("scan session day: %w", err)
		}
		out[day] = secs
	}
	return out, rows.Err()
}

// CountListeningSessions returns how many sessions are stored.
func CountListeningSessions(db *sql.DB) (int, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM listening_sessions`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return n, nil
}

// JournalEntry records one entry earworm has written to an external journal.
type JournalEntry struct {
	EntryKey    string
	Kind        string
	EntryID     string
	JournalID   string
	EntryDate   string
	ContentHash string
}

// UpsertJournalEntry records that an entry was written.
func UpsertJournalEntry(db *sql.DB, e JournalEntry) error {
	_, err := db.Exec(`
		INSERT INTO journal_entries (
			entry_key, kind, entry_id, journal_id, entry_date, content_hash, written_at
		) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(entry_key) DO UPDATE SET
			kind = excluded.kind,
			entry_id = excluded.entry_id,
			journal_id = excluded.journal_id,
			entry_date = excluded.entry_date,
			content_hash = excluded.content_hash,
			written_at = CURRENT_TIMESTAMP`,
		e.EntryKey, e.Kind, e.EntryID, e.JournalID, e.EntryDate, e.ContentHash)
	if err != nil {
		return fmt.Errorf("upsert journal entry %s: %w", e.EntryKey, err)
	}
	return nil
}

// GetJournalEntry returns a previously written entry, if any.
func GetJournalEntry(db *sql.DB, entryKey string) (JournalEntry, bool, error) {
	var e JournalEntry
	err := db.QueryRow(`
		SELECT entry_key, kind, entry_id, journal_id, entry_date, content_hash
		FROM journal_entries WHERE entry_key = ?`, entryKey).
		Scan(&e.EntryKey, &e.Kind, &e.EntryID, &e.JournalID, &e.EntryDate, &e.ContentHash)
	if err == sql.ErrNoRows {
		return e, false, nil
	}
	if err != nil {
		return e, false, fmt.Errorf("get journal entry %s: %w", entryKey, err)
	}
	return e, true, nil
}

// CountJournalEntries returns how many entries have been written.
func CountJournalEntries(db *sql.DB) (int, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count journal entries: %w", err)
	}
	return n, nil
}
