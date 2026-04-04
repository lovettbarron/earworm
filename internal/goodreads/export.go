package goodreads

import (
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/lovettbarron/earworm/internal/db"
)

// csvHeader defines the exact Goodreads CSV import column names.
var csvHeader = []string{
	"Title", "Author", "ISBN", "ISBN13", "My Rating",
	"Average Rating", "Publisher", "Year Published",
	"Date Read", "Date Added", "Bookshelves", "Exclusive Shelf",
}

// ExportCSV writes books as a Goodreads-compatible CSV to the given writer.
// Books with empty Title or Author are skipped with a warning log.
func ExportCSV(w io.Writer, books []db.Book) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write(csvHeader); err != nil {
		return fmt.Errorf("goodreads csv write header: %w", err)
	}

	for _, b := range books {
		if b.Title == "" || b.Author == "" {
			slog.Warn("skipping book with empty title or author",
				"asin", b.ASIN,
				"title", b.Title,
				"author", b.Author,
			)
			continue
		}

		record := []string{
			b.Title,                       // Title
			b.Author,                      // Author
			"",                            // ISBN (Audible has no ISBN)
			"",                            // ISBN13
			"",                            // My Rating
			"",                            // Average Rating
			"",                            // Publisher
			formatYear(b.Year),            // Year Published
			reformatDate(b.PurchaseDate),  // Date Read
			b.CreatedAt.Format("2006/01/02"), // Date Added
			"read, audiobook",             // Bookshelves
			"read",                        // Exclusive Shelf
		}

		if err := cw.Write(record); err != nil {
			return fmt.Errorf("goodreads csv write book %s: %w", b.ASIN, err)
		}
	}

	return cw.Error()
}

// reformatDate converts an ISO date "2024-01-15" to Goodreads format "2024/01/15".
// Returns empty string if input is empty.
func reformatDate(iso string) string {
	if iso == "" {
		return ""
	}
	return strings.ReplaceAll(iso, "-", "/")
}

// formatYear returns the year as a string, or empty if zero/negative.
func formatYear(year int) string {
	if year <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", year)
}
