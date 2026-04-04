package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	require.NotNil(t, db)
	db.Close()
}

func TestOpenCreatesSchema(t *testing.T) {
	db := setupTestDB(t)

	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='books'").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "books", name)
}

func TestOpenCreatesSchemaVersions(t *testing.T) {
	db := setupTestDB(t)

	var version int
	err := db.QueryRow("SELECT version FROM schema_versions WHERE version = 1").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestMigration002Applied(t *testing.T) {
	db := setupTestDB(t)

	// Verify migration 002 was recorded
	var version int
	err := db.QueryRow("SELECT version FROM schema_versions WHERE version = 2").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 2, version)

	// Verify new columns exist by inserting a row with them
	_, err = db.Exec(`INSERT INTO books (asin, narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count)
		VALUES ('TEST001', 'Narrator', 'Fiction', 2024, 'Series 1', 1, 3600, 10, 'tag', 5)`)
	require.NoError(t, err)

	// Read back and verify
	var narrator, genre, series, metadataSource string
	var year, hasCover, duration, chapterCount, fileCount int
	err = db.QueryRow(`SELECT narrator, genre, year, series, has_cover, duration, chapter_count, metadata_source, file_count FROM books WHERE asin = 'TEST001'`).
		Scan(&narrator, &genre, &year, &series, &hasCover, &duration, &chapterCount, &metadataSource, &fileCount)
	require.NoError(t, err)
	assert.Equal(t, "Narrator", narrator)
	assert.Equal(t, "Fiction", genre)
	assert.Equal(t, 2024, year)
	assert.Equal(t, "Series 1", series)
	assert.Equal(t, 1, hasCover)
	assert.Equal(t, 3600, duration)
	assert.Equal(t, 10, chapterCount)
	assert.Equal(t, "tag", metadataSource)
	assert.Equal(t, 5, fileCount)
}

func TestOpenIdempotent(t *testing.T) {
	// Use a temp file so we can open the same path twice
	dir := t.TempDir()
	dbPath := dir + "/test.db"

	db1, err := Open(dbPath)
	require.NoError(t, err)
	db1.Close()

	db2, err := Open(dbPath)
	require.NoError(t, err)
	assert.NotNil(t, db2)
	db2.Close()
}

func TestWALMode(t *testing.T) {
	// WAL mode is not supported for :memory: databases, so use a temp file
	dir := t.TempDir()
	db, err := Open(dir + "/wal_test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	var mode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	require.NoError(t, err)
	assert.Equal(t, "wal", mode)
}

func TestInsertBook(t *testing.T) {
	db := setupTestDB(t)

	book := Book{
		ASIN:   "B08C6YJ1LS",
		Title:  "Project Hail Mary",
		Author: "Andy Weir",
		Status: "unknown",
	}
	err := InsertBook(db, book)
	require.NoError(t, err)

	got, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "B08C6YJ1LS", got.ASIN)
	assert.Equal(t, "Project Hail Mary", got.Title)
	assert.Equal(t, "Andy Weir", got.Author)
	assert.Equal(t, "unknown", got.Status)
}

func TestInsertBookDuplicateASIN(t *testing.T) {
	db := setupTestDB(t)

	book := Book{ASIN: "B08C6YJ1LS", Title: "First", Author: "A", Status: "unknown"}
	err := InsertBook(db, book)
	require.NoError(t, err)

	dup := Book{ASIN: "B08C6YJ1LS", Title: "Second", Author: "B", Status: "unknown"}
	err = InsertBook(db, dup)
	assert.Error(t, err)
}

func TestGetBookNotFound(t *testing.T) {
	db := setupTestDB(t)

	got, err := GetBook(db, "NONEXISTENT")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestGetBookExtendedFields(t *testing.T) {
	db := setupTestDB(t)

	book := Book{
		ASIN:           "B08C6YJ1LS",
		Title:          "Project Hail Mary",
		Author:         "Andy Weir",
		Narrator:       "Ray Porter",
		Genre:          "Science Fiction",
		Year:           2021,
		Series:         "",
		HasCover:       true,
		Duration:       57600,
		ChapterCount:   30,
		MetadataSource: "tag",
		FileCount:      1,
		Status:         "scanned",
		LocalPath:      "/library/Andy Weir/Project Hail Mary [B08C6YJ1LS]",
	}
	err := InsertBook(db, book)
	require.NoError(t, err)

	got, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Ray Porter", got.Narrator)
	assert.Equal(t, "Science Fiction", got.Genre)
	assert.Equal(t, 2021, got.Year)
	assert.True(t, got.HasCover)
	assert.Equal(t, 57600, got.Duration)
	assert.Equal(t, 30, got.ChapterCount)
	assert.Equal(t, "tag", got.MetadataSource)
	assert.Equal(t, 1, got.FileCount)
}

func TestListBooks(t *testing.T) {
	db := setupTestDB(t)

	books := []Book{
		{ASIN: "A001", Title: "Book One", Author: "Author 1", Status: "unknown"},
		{ASIN: "A002", Title: "Book Two", Author: "Author 2", Status: "scanned"},
		{ASIN: "A003", Title: "Book Three", Author: "Author 3", Status: "downloaded"},
	}
	for _, b := range books {
		require.NoError(t, InsertBook(db, b))
	}

	result, err := ListBooks(db)
	require.NoError(t, err)
	assert.Len(t, result, 3)
}

func TestListBooksExtendedFields(t *testing.T) {
	db := setupTestDB(t)

	book := Book{
		ASIN:     "B08C6YJ1LS",
		Title:    "Project Hail Mary",
		Author:   "Andy Weir",
		Narrator: "Ray Porter",
		Genre:    "Science Fiction",
		Status:   "scanned",
	}
	require.NoError(t, InsertBook(db, book))

	result, err := ListBooks(db)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Ray Porter", result[0].Narrator)
	assert.Equal(t, "Science Fiction", result[0].Genre)
}

func TestListBooksEmpty(t *testing.T) {
	db := setupTestDB(t)

	result, err := ListBooks(db)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestUpdateBookStatus(t *testing.T) {
	db := setupTestDB(t)

	book := Book{ASIN: "B08C6YJ1LS", Title: "Test", Author: "Test", Status: "unknown"}
	require.NoError(t, InsertBook(db, book))

	err := UpdateBookStatus(db, "B08C6YJ1LS", "downloaded")
	require.NoError(t, err)

	got, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "downloaded", got.Status)
}

func TestUpdateBookStatusRemoved(t *testing.T) {
	db := setupTestDB(t)

	book := Book{ASIN: "B08C6YJ1LS", Title: "Test", Author: "Test", Status: "scanned"}
	require.NoError(t, InsertBook(db, book))

	err := UpdateBookStatus(db, "B08C6YJ1LS", "removed")
	require.NoError(t, err)

	got, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "removed", got.Status)
}

func TestUpdateBookStatusNotFound(t *testing.T) {
	db := setupTestDB(t)

	err := UpdateBookStatus(db, "NONEXISTENT", "downloaded")
	assert.Error(t, err)
}

func TestInsertBookMinimalFields(t *testing.T) {
	db := setupTestDB(t)

	book := Book{ASIN: "B00DEKC9GK"}
	err := InsertBook(db, book)
	require.NoError(t, err)

	got, err := GetBook(db, "B00DEKC9GK")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "B00DEKC9GK", got.ASIN)
	assert.Equal(t, "", got.Title)
	assert.Equal(t, "", got.Author)
	assert.Equal(t, "unknown", got.Status) // default applied
	assert.Equal(t, "", got.LocalPath)
	assert.Equal(t, "", got.Narrator)
	assert.Equal(t, "", got.Genre)
	assert.Equal(t, 0, got.Year)
	assert.Equal(t, "", got.Series)
	assert.False(t, got.HasCover)
	assert.Equal(t, 0, got.Duration)
	assert.Equal(t, 0, got.ChapterCount)
	assert.Equal(t, "", got.MetadataSource)
	assert.Equal(t, 0, got.FileCount)
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestUpsertBookInsert(t *testing.T) {
	db := setupTestDB(t)

	book := Book{
		ASIN:           "B08C6YJ1LS",
		Title:          "Project Hail Mary",
		Author:         "Andy Weir",
		Narrator:       "Ray Porter",
		Genre:          "Science Fiction",
		Year:           2021,
		HasCover:       true,
		Duration:       57600,
		ChapterCount:   30,
		MetadataSource: "tag",
		FileCount:      1,
		Status:         "scanned",
		LocalPath:      "/library/Andy Weir/Project Hail Mary [B08C6YJ1LS]",
	}
	err := UpsertBook(db, book)
	require.NoError(t, err)

	got, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Project Hail Mary", got.Title)
	assert.Equal(t, "Andy Weir", got.Author)
	assert.Equal(t, "Ray Porter", got.Narrator)
	assert.Equal(t, "Science Fiction", got.Genre)
	assert.Equal(t, 2021, got.Year)
	assert.True(t, got.HasCover)
	assert.Equal(t, 57600, got.Duration)
	assert.Equal(t, 30, got.ChapterCount)
	assert.Equal(t, "tag", got.MetadataSource)
	assert.Equal(t, 1, got.FileCount)
	assert.Equal(t, "scanned", got.Status)
}

func TestUpsertBookUpdate(t *testing.T) {
	db := setupTestDB(t)

	// Insert initial version
	book := Book{
		ASIN:   "B08C6YJ1LS",
		Title:  "Project Hail Mary",
		Author: "Andy Weir",
		Status: "scanned",
	}
	err := UpsertBook(db, book)
	require.NoError(t, err)

	// Get original created_at
	got, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	originalCreatedAt := got.CreatedAt
	originalUpdatedAt := got.UpdatedAt

	// Small delay to ensure updated_at changes
	time.Sleep(10 * time.Millisecond)

	// Upsert with updated metadata
	updated := Book{
		ASIN:           "B08C6YJ1LS",
		Title:          "Project Hail Mary (Updated)",
		Author:         "Andy Weir",
		Narrator:       "Ray Porter",
		Genre:          "Sci-Fi",
		Year:           2021,
		MetadataSource: "tag",
		Status:         "scanned",
	}
	err = UpsertBook(db, updated)
	require.NoError(t, err)

	got, err = GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	require.NotNil(t, got)

	// Verify metadata was updated
	assert.Equal(t, "Project Hail Mary (Updated)", got.Title)
	assert.Equal(t, "Ray Porter", got.Narrator)
	assert.Equal(t, "Sci-Fi", got.Genre)
	assert.Equal(t, 2021, got.Year)
	assert.Equal(t, "tag", got.MetadataSource)

	// created_at should be preserved
	assert.Equal(t, originalCreatedAt.Unix(), got.CreatedAt.Unix())

	// updated_at should change (or at least not be before original)
	assert.True(t, got.UpdatedAt.Unix() >= originalUpdatedAt.Unix())
}

func TestMigration003Applied(t *testing.T) {
	db := setupTestDB(t)

	// Verify migration 003 was recorded
	var version int
	err := db.QueryRow("SELECT version FROM schema_versions WHERE version = 3").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 3, version)

	// Verify new columns exist by inserting a row with them
	_, err = db.Exec(`INSERT INTO books (asin, audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position)
		VALUES ('MIGTEST003', 'finished', '2024-01-15', 720, 'Ray Porter, Wil Wheaton', 'Bobiverse', '1')`)
	require.NoError(t, err)

	// Read back and verify
	var audibleStatus, purchaseDate, narrators, seriesName, seriesPosition string
	var runtimeMinutes int
	err = db.QueryRow(`SELECT audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position FROM books WHERE asin = 'MIGTEST003'`).
		Scan(&audibleStatus, &purchaseDate, &runtimeMinutes, &narrators, &seriesName, &seriesPosition)
	require.NoError(t, err)
	assert.Equal(t, "finished", audibleStatus)
	assert.Equal(t, "2024-01-15", purchaseDate)
	assert.Equal(t, 720, runtimeMinutes)
	assert.Equal(t, "Ray Porter, Wil Wheaton", narrators)
	assert.Equal(t, "Bobiverse", seriesName)
	assert.Equal(t, "1", seriesPosition)
}

func TestInsertBookWithAudibleFields(t *testing.T) {
	db := setupTestDB(t)

	book := Book{
		ASIN:           "B08AUDIBLE1",
		Title:          "Test Audible Book",
		Author:         "Test Author",
		Status:         "unknown",
		AudibleStatus:  "finished",
		PurchaseDate:   "2024-03-20",
		RuntimeMinutes: 480,
		Narrators:      "Narrator One",
		SeriesName:     "Test Series",
		SeriesPosition: "2.5",
	}
	err := InsertBook(db, book)
	require.NoError(t, err)

	got, err := GetBook(db, "B08AUDIBLE1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "finished", got.AudibleStatus)
	assert.Equal(t, "2024-03-20", got.PurchaseDate)
	assert.Equal(t, 480, got.RuntimeMinutes)
	assert.Equal(t, "Narrator One", got.Narrators)
	assert.Equal(t, "Test Series", got.SeriesName)
	assert.Equal(t, "2.5", got.SeriesPosition)
}

func TestUpsertBookPreservesCreatedAt(t *testing.T) {
	db := setupTestDB(t)

	book := Book{
		ASIN:   "B08C6YJ1LS",
		Title:  "Original",
		Author: "Author",
		Status: "scanned",
	}
	err := UpsertBook(db, book)
	require.NoError(t, err)

	got1, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)
	createdAt1 := got1.CreatedAt

	// Upsert again
	book.Title = "Updated"
	err = UpsertBook(db, book)
	require.NoError(t, err)

	got2, err := GetBook(db, "B08C6YJ1LS")
	require.NoError(t, err)

	// created_at must be preserved
	assert.Equal(t, createdAt1.Unix(), got2.CreatedAt.Unix())
}
