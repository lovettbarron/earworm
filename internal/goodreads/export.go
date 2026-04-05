package goodreads

import (
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/lovettbarron/earworm/internal/db"
)

var csvHeader = []string{
	"Title", "Author", "ISBN", "ISBN13", "My Rating",
	"Average Rating", "Publisher", "Year Published",
	"Date Read", "Date Added", "Bookshelves", "Exclusive Shelf",
}

// ExportCSV writes the book list as Goodreads-compatible CSV to w.
func ExportCSV(w io.Writer, books []db.Book) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write(csvHeader); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}

	for _, b := range books {
		if b.Title == "" || b.Author == "" {
			slog.Warn("skipping book with empty title or author", "asin", b.ASIN)
			continue
		}

		record := []string{
			b.Title,
			b.Author,
			"", // ISBN
			"", // ISBN13
			"", // My Rating
			"", // Average Rating
			"", // Publisher
			formatYear(b.Year),
			reformatDate(b.PurchaseDate),
			b.CreatedAt.Format("2006/01/02"),
			"read, audiobook",
			"read",
		}

		if err := cw.Write(record); err != nil {
			return fmt.Errorf("writing CSV record: %w", err)
		}
	}

	return cw.Error()
}

// reformatDate converts ISO date "2024-01-15" to Goodreads format "2024/01/15".
func reformatDate(iso string) string {
	if iso == "" {
		return ""
	}
	return strings.ReplaceAll(iso, "-", "/")
}

// formatYear converts a year integer to string, empty if zero.
func formatYear(year int) string {
	if year <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", year)
}
