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
}

// Book represents an audiobook in the library.
type Book struct {
	ASIN      string
	Title     string
	Author    string
	Status    string
	LocalPath string
	CreatedAt time.Time
	UpdatedAt time.Time
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
		`INSERT INTO books (asin, title, author, status, local_path) VALUES (?, ?, ?, ?, ?)`,
		book.ASIN, book.Title, book.Author, book.Status, book.LocalPath,
	)
	if err != nil {
		return fmt.Errorf("insert book %s: %w", book.ASIN, err)
	}
	return nil
}

// GetBook retrieves a book by ASIN. Returns nil and no error if not found.
func GetBook(db *sql.DB, asin string) (*Book, error) {
	row := db.QueryRow(
		`SELECT asin, title, author, status, local_path, created_at, updated_at FROM books WHERE asin = ?`,
		asin,
	)

	var b Book
	err := row.Scan(&b.ASIN, &b.Title, &b.Author, &b.Status, &b.LocalPath, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book %s: %w", asin, err)
	}
	return &b, nil
}

// ListBooks returns all books ordered by created_at descending.
func ListBooks(db *sql.DB) ([]Book, error) {
	rows, err := db.Query(
		`SELECT asin, title, author, status, local_path, created_at, updated_at FROM books ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ASIN, &b.Title, &b.Author, &b.Status, &b.LocalPath, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan book row: %w", err)
		}
		books = append(books, b)
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
