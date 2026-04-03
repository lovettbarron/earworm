package db

import (
	"database/sql"
	"testing"

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
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
}
