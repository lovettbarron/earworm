package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ValidStatuses defines the allowed book status values.
var ValidStatuses = []string{
	"unknown",
	"scanned",
	"downloading",
	"downloaded",
	"organized",
	"error",
	"removed",
}

// Book represents an audiobook in the library.
type Book struct {
	ASIN                string
	Title               string
	Author              string
	Narrator            string
	Genre               string
	Year                int
	Series              string
	HasCover            bool
	Duration            int    // seconds
	ChapterCount        int
	MetadataSource      string // "tag", "ffprobe", "folder"
	FileCount           int
	Status              string
	LocalPath           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	AudibleStatus       string // "finished", "in_progress", "new", or ""
	PurchaseDate        string // ISO date string from Audible
	RuntimeMinutes      int    // runtime in minutes from Audible
	Narrators           string // comma-separated narrator names from Audible
	SeriesName          string // series title from Audible
	SeriesPosition      string // position in series (e.g., "1", "2.5")
	RetryCount          int
	LastError           string
	DownloadStartedAt   *time.Time // nullable
	DownloadCompletedAt *time.Time // nullable
}

// isValidStatus checks whether a status string is in the allowed set.
func isValidStatus(status string) bool {
	for _, s := range ValidStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// hasCoverToInt converts bool to SQLite integer (0/1).
func hasCoverToInt(hasCover bool) int {
	if hasCover {
		return 1
	}
	return 0
}

// InsertBook inserts a new book into the database.
// Returns an error if a book with the same ASIN already exists.
func InsertBook(db *sql.DB, book Book) error {
	if book.Status == "" {
		book.Status = "unknown"
	}
	if !isValidStatus(book.Status) {
		return fmt.Errorf("invalid status %q", book.Status)
	}

	_, err := db.Exec(
		`INSERT INTO books (asin, title, author, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count, status, local_path, audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ASIN, book.Title, book.Author, book.Narrator, book.Genre, book.Year,
		book.Series, hasCoverToInt(book.HasCover), book.Duration, book.ChapterCount,
		book.MetadataSource, book.FileCount, book.Status, book.LocalPath,
		book.AudibleStatus, book.PurchaseDate, book.RuntimeMinutes, book.Narrators,
		book.SeriesName, book.SeriesPosition,
	)
	if err != nil {
		return fmt.Errorf("insert book %s: %w", book.ASIN, err)
	}
	return nil
}

// UpsertBook inserts a new book or updates an existing one matched by ASIN.
// On conflict, all fields except asin and created_at are updated.
func UpsertBook(db *sql.DB, book Book) error {
	if book.Status == "" {
		book.Status = "unknown"
	}
	if !isValidStatus(book.Status) {
		return fmt.Errorf("invalid status %q", book.Status)
	}

	_, err := db.Exec(
		`INSERT INTO books (asin, title, author, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count, status, local_path, audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asin) DO UPDATE SET
			title = excluded.title,
			author = excluded.author,
			narrator = excluded.narrator,
			genre = excluded.genre,
			year = excluded.year,
			series = excluded.series,
			has_cover = excluded.has_cover,
			duration = excluded.duration,
			chapter_count = excluded.chapter_count,
			metadata_source = excluded.metadata_source,
			file_count = excluded.file_count,
			status = excluded.status,
			local_path = excluded.local_path,
			audible_status = excluded.audible_status,
			purchase_date = excluded.purchase_date,
			runtime_minutes = excluded.runtime_minutes,
			narrators = excluded.narrators,
			series_name = excluded.series_name,
			series_position = excluded.series_position,
			updated_at = CURRENT_TIMESTAMP`,
		book.ASIN, book.Title, book.Author, book.Narrator, book.Genre, book.Year,
		book.Series, hasCoverToInt(book.HasCover), book.Duration, book.ChapterCount,
		book.MetadataSource, book.FileCount, book.Status, book.LocalPath,
		book.AudibleStatus, book.PurchaseDate, book.RuntimeMinutes, book.Narrators,
		book.SeriesName, book.SeriesPosition,
	)
	if err != nil {
		return fmt.Errorf("upsert book %s: %w", book.ASIN, err)
	}
	return nil
}

// scanBook scans a row into a Book struct, handling has_cover int->bool conversion
// and nullable datetime columns.
func scanBook(scanner interface{ Scan(dest ...any) error }) (*Book, error) {
	var b Book
	var hasCover int
	var downloadStartedAt sql.NullTime
	var downloadCompletedAt sql.NullTime
	err := scanner.Scan(
		&b.ASIN, &b.Title, &b.Author, &b.Narrator, &b.Genre, &b.Year,
		&b.Series, &hasCover, &b.Duration, &b.ChapterCount, &b.MetadataSource,
		&b.FileCount, &b.Status, &b.LocalPath, &b.CreatedAt, &b.UpdatedAt,
		&b.AudibleStatus, &b.PurchaseDate, &b.RuntimeMinutes, &b.Narrators,
		&b.SeriesName, &b.SeriesPosition,
		&b.RetryCount, &b.LastError, &downloadStartedAt, &downloadCompletedAt,
	)
	if err != nil {
		return nil, err
	}
	b.HasCover = hasCover != 0
	if downloadStartedAt.Valid {
		b.DownloadStartedAt = &downloadStartedAt.Time
	}
	if downloadCompletedAt.Valid {
		b.DownloadCompletedAt = &downloadCompletedAt.Time
	}
	return &b, nil
}

// allColumns is the shared column list for SELECT queries.
const allColumns = `asin, title, author, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count, status, local_path, created_at, updated_at, audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position, retry_count, last_error, download_started_at, download_completed_at`

// GetBook retrieves a book by ASIN. Returns nil and no error if not found.
func GetBook(db *sql.DB, asin string) (*Book, error) {
	row := db.QueryRow(
		`SELECT `+allColumns+` FROM books WHERE asin = ?`,
		asin,
	)

	b, err := scanBook(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book %s: %w", asin, err)
	}
	return b, nil
}

// ListBooks returns all books ordered by created_at descending.
func ListBooks(db *sql.DB) ([]Book, error) {
	rows, err := db.Query(
		`SELECT ` + allColumns + ` FROM books ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan book row: %w", err)
		}
		books = append(books, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate books: %w", err)
	}

	if books == nil {
		books = []Book{}
	}
	return books, nil
}

// UpdateBookStatus updates a book's status and updated_at timestamp.
// Returns an error if the ASIN does not exist.
func UpdateBookStatus(db *sql.DB, asin string, status string) error {
	if !isValidStatus(status) {
		return fmt.Errorf("invalid status %q", status)
	}

	result, err := db.Exec(
		`UPDATE books SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE asin = ?`,
		status, asin,
	)
	if err != nil {
		return fmt.Errorf("update book status %s: %w", asin, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("book %s not found", asin)
	}
	return nil
}

// SyncRemoteBook upserts a book from Audible remote metadata.
// On insert, sets local-only fields to defaults (status="unknown", local_path="", metadata_source="", file_count=0).
// On conflict, updates remote metadata fields but preserves local-only fields
// (status, local_path, metadata_source, file_count, has_cover, duration, chapter_count).
func SyncRemoteBook(db *sql.DB, book Book) error {
	_, err := db.Exec(
		`INSERT INTO books (asin, title, author, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count, status, local_path, audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, 'unknown', '', ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asin) DO UPDATE SET
			title = excluded.title,
			author = excluded.author,
			narrator = excluded.narrator,
			genre = excluded.genre,
			year = excluded.year,
			series = excluded.series,
			audible_status = excluded.audible_status,
			purchase_date = excluded.purchase_date,
			runtime_minutes = excluded.runtime_minutes,
			narrators = excluded.narrators,
			series_name = excluded.series_name,
			series_position = excluded.series_position,
			updated_at = CURRENT_TIMESTAMP`,
		book.ASIN, book.Title, book.Author, book.Narrator, book.Genre, book.Year,
		book.Series, hasCoverToInt(book.HasCover), book.Duration, book.ChapterCount,
		book.AudibleStatus, book.PurchaseDate, book.RuntimeMinutes, book.Narrators,
		book.SeriesName, book.SeriesPosition,
	)
	if err != nil {
		return fmt.Errorf("sync remote book %s: %w", book.ASIN, err)
	}
	return nil
}

// UpdateDownloadStart marks a book as downloading and sets download_started_at.
func UpdateDownloadStart(db *sql.DB, asin string) error {
	_, err := db.Exec(`UPDATE books SET status = 'downloading',
		download_started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE asin = ?`, asin)
	if err != nil {
		return fmt.Errorf("update download start %s: %w", asin, err)
	}
	return nil
}

// UpdateDownloadComplete marks a book as downloaded, sets local_path and download_completed_at,
// and clears retry_count and last_error.
func UpdateDownloadComplete(db *sql.DB, asin string, localPath string) error {
	_, err := db.Exec(`UPDATE books SET status = 'downloaded',
		local_path = ?, retry_count = 0, last_error = '',
		download_completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE asin = ?`, localPath, asin)
	if err != nil {
		return fmt.Errorf("update download complete %s: %w", asin, err)
	}
	return nil
}

// UpdateDownloadError marks a book as error with retry count and error message.
func UpdateDownloadError(db *sql.DB, asin string, retryCount int, errMsg string) error {
	_, err := db.Exec(`UPDATE books SET status = 'error',
		retry_count = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE asin = ?`, retryCount, errMsg, asin)
	if err != nil {
		return fmt.Errorf("update download error %s: %w", asin, err)
	}
	return nil
}

// ListDownloadable returns books that are eligible for download:
// books with audible_status set and not yet downloaded/organized, plus books in error state.
// Returns an empty slice (not nil) when no books match.
func ListDownloadable(db *sql.DB) ([]Book, error) {
	rows, err := db.Query(
		`SELECT `+allColumns+` FROM books
		WHERE (audible_status != '' AND status NOT IN ('downloaded', 'organized'))
		   OR status = 'error'
		ORDER BY purchase_date DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list downloadable: %w", err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan downloadable row: %w", err)
		}
		books = append(books, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate downloadable: %w", err)
	}

	if books == nil {
		books = []Book{}
	}
	return books, nil
}

// ListOrganizable returns books that are ready to be organized into the library:
// books with 'downloaded' status, ordered by updated_at ascending (oldest first).
// Returns an empty slice (not nil) when no books match.
func ListOrganizable(db *sql.DB) ([]Book, error) {
	rows, err := db.Query(
		`SELECT `+allColumns+` FROM books WHERE status = 'downloaded' ORDER BY updated_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list organizable: %w", err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan organizable row: %w", err)
		}
		books = append(books, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organizable: %w", err)
	}

	if books == nil {
		books = []Book{}
	}
	return books, nil
}

// UpdateOrganizeResult updates a book's status, local_path, and last_error atomically.
// Used after organize attempts to record success (status="organized") or failure (status="error").
// Returns an error if the ASIN does not exist or if the status is invalid.
func UpdateOrganizeResult(db *sql.DB, asin, status, localPath, lastError string) error {
	if !isValidStatus(status) {
		return fmt.Errorf("invalid status %q", status)
	}

	result, err := db.Exec(
		`UPDATE books SET status = ?, local_path = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE asin = ?`,
		status, localPath, lastError, asin,
	)
	if err != nil {
		return fmt.Errorf("update organize result %s: %w", asin, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("book %s not found", asin)
	}
	return nil
}

// ListNewBooks returns books that exist in Audible (audible_status is set)
// but are not yet downloaded locally. This includes books with no local_path,
// or books whose status is not yet 'downloaded' or 'organized'.
// Returns an empty slice (not nil) when no new books exist.
func ListNewBooks(db *sql.DB) ([]Book, error) {
	rows, err := db.Query(
		`SELECT `+allColumns+` FROM books
		WHERE audible_status != ''
		  AND (local_path = '' OR status NOT IN ('downloaded', 'organized'))
		ORDER BY purchase_date DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list new books: %w", err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan new book row: %w", err)
		}
		books = append(books, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate new books: %w", err)
	}

	if books == nil {
		books = []Book{}
	}
	return books, nil
}
