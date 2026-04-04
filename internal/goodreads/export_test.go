package goodreads

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeBook(asin, title, author string, year int, purchaseDate string) db.Book {
	return db.Book{
		ASIN:         asin,
		Title:        title,
		Author:       author,
		Year:         year,
		PurchaseDate: purchaseDate,
		CreatedAt:    time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestExportCSV_OneBook_HeaderAndData(t *testing.T) {
	books := []db.Book{makeBook("B001", "My Book", "Jane Author", 2021, "2024-01-15")}
	var buf bytes.Buffer

	err := ExportCSV(&buf, books)
	require.NoError(t, err)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)

	require.Len(t, records, 2, "expected header + 1 data row")

	// Verify header columns
	expectedHeader := []string{
		"Title", "Author", "ISBN", "ISBN13", "My Rating",
		"Average Rating", "Publisher", "Year Published",
		"Date Read", "Date Added", "Bookshelves", "Exclusive Shelf",
	}
	assert.Equal(t, expectedHeader, records[0])
}

func TestExportCSV_DataRowValues(t *testing.T) {
	books := []db.Book{makeBook("B001", "My Book", "Jane Author", 2021, "2024-01-15")}
	var buf bytes.Buffer

	err := ExportCSV(&buf, books)
	require.NoError(t, err)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)

	row := records[1]
	assert.Equal(t, "My Book", row[0])          // Title
	assert.Equal(t, "Jane Author", row[1])       // Author
	assert.Equal(t, "", row[2])                   // ISBN
	assert.Equal(t, "", row[3])                   // ISBN13
	assert.Equal(t, "", row[4])                   // My Rating
	assert.Equal(t, "", row[5])                   // Average Rating
	assert.Equal(t, "", row[6])                   // Publisher
	assert.Equal(t, "2021", row[7])               // Year Published
	assert.Equal(t, "2024/01/15", row[8])         // Date Read
	assert.Equal(t, "2024/03/01", row[9])         // Date Added
	assert.Equal(t, "read, audiobook", row[10])   // Bookshelves
	assert.Equal(t, "read", row[11])              // Exclusive Shelf
}

func TestExportCSV_EmptySlice_HeaderOnly(t *testing.T) {
	var buf bytes.Buffer

	err := ExportCSV(&buf, []db.Book{})
	require.NoError(t, err)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)

	assert.Len(t, records, 1, "expected header only")
}

func TestExportCSV_CSVEscaping(t *testing.T) {
	books := []db.Book{makeBook("B002", `Title with "quotes" and, commas`, "Author, Jr.", 2020, "2023-06-01")}
	var buf bytes.Buffer

	err := ExportCSV(&buf, books)
	require.NoError(t, err)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)

	require.Len(t, records, 2)
	assert.Equal(t, `Title with "quotes" and, commas`, records[1][0])
	assert.Equal(t, "Author, Jr.", records[1][1])
}

func TestExportCSV_EmptyTitle_Skipped(t *testing.T) {
	books := []db.Book{
		makeBook("B003", "", "Some Author", 2019, "2023-01-01"),
		makeBook("B004", "Good Book", "Another Author", 2020, "2023-02-01"),
	}
	var buf bytes.Buffer

	err := ExportCSV(&buf, books)
	require.NoError(t, err)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)

	assert.Len(t, records, 2, "header + 1 valid book (empty title skipped)")
	assert.Equal(t, "Good Book", records[1][0])
}

func TestExportCSV_EmptyPurchaseDate(t *testing.T) {
	books := []db.Book{makeBook("B005", "No Date Book", "Author", 2022, "")}
	var buf bytes.Buffer

	err := ExportCSV(&buf, books)
	require.NoError(t, err)

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)

	require.Len(t, records, 2)
	assert.Equal(t, "", records[1][8], "Date Read should be empty when PurchaseDate is empty")
}
