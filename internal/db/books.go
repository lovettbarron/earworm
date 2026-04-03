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
	ASIN           string
	Title          string
	Author         string
	Narrator       string
	Genre          string
	Year           int
	Series         string
	HasCover       bool
	Duration       int    // seconds
	ChapterCount   int
	MetadataSource string // "tag", "ffprobe", "folder"
	FileCount      int
	Status         string
	LocalPath      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
		`INSERT INTO books (asin, title, author, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count, status, local_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ASIN, book.Title, book.Author, book.Narrator, book.Genre, book.Year,
		book.Series, hasCoverToInt(book.HasCover), book.Duration, book.ChapterCount,
		book.MetadataSource, book.FileCount, book.Status, book.LocalPath,
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
		`INSERT INTO books (asin, title, author, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count, status, local_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			updated_at = CURRENT_TIMESTAMP`,
		book.ASIN, book.Title, book.Author, book.Narrator, book.Genre, book.Year,
		book.Series, hasCoverToInt(book.HasCover), book.Duration, book.ChapterCount,
		book.MetadataSource, book.FileCount, book.Status, book.LocalPath,
	)
	if err != nil {
		return fmt.Errorf("upsert book %s: %w", book.ASIN, err)
	}
	return nil
}

// scanBook scans a row into a Book struct, handling has_cover int->bool conversion.
func scanBook(scanner interface{ Scan(dest ...any) error }) (*Book, error) {
	var b Book
	var hasCover int
	err := scanner.Scan(
		&b.ASIN, &b.Title, &b.Author, &b.Narrator, &b.Genre, &b.Year,
		&b.Series, &hasCover, &b.Duration, &b.ChapterCount, &b.MetadataSource,
		&b.FileCount, &b.Status, &b.LocalPath, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	b.HasCover = hasCover != 0
	return &b, nil
}

// allColumns is the shared column list for SELECT queries.
const allColumns = `asin, title, author, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count, status, local_path, created_at, updated_at`

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
