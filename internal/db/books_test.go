package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateDownloadStart(t *testing.T) {
	db := setupTestDB(t)

	// Insert a book to update
	err := InsertBook(db, Book{
		ASIN:          "DL001",
		Title:         "Download Test",
		Author:        "Author",
		Status:        "unknown",
		AudibleStatus: "new",
	})
	require.NoError(t, err)

	err = UpdateDownloadStart(db, "DL001")
	require.NoError(t, err)

	got, err := GetBook(db, "DL001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "downloading", got.Status)
	assert.NotNil(t, got.DownloadStartedAt, "download_started_at should be set")
}

func TestUpdateDownloadComplete(t *testing.T) {
	db := setupTestDB(t)

	// Insert a book and set it to downloading with an error state first
	err := InsertBook(db, Book{
		ASIN:          "DL002",
		Title:         "Complete Test",
		Author:        "Author",
		Status:        "downloading",
		AudibleStatus: "finished",
	})
	require.NoError(t, err)

	// Simulate a previous error
	err = UpdateDownloadError(db, "DL002", 2, "previous error")
	require.NoError(t, err)

	// Now complete successfully
	err = UpdateDownloadComplete(db, "DL002", "/library/Author/Complete Test [DL002]")
	require.NoError(t, err)

	got, err := GetBook(db, "DL002")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "downloaded", got.Status)
	assert.Equal(t, "/library/Author/Complete Test [DL002]", got.LocalPath)
	assert.NotNil(t, got.DownloadCompletedAt, "download_completed_at should be set")
	assert.Equal(t, 0, got.RetryCount, "retry_count should be cleared")
	assert.Equal(t, "", got.LastError, "last_error should be cleared")
}

func TestUpdateDownloadError(t *testing.T) {
	db := setupTestDB(t)

	err := InsertBook(db, Book{
		ASIN:          "DL003",
		Title:         "Error Test",
		Author:        "Author",
		Status:        "downloading",
		AudibleStatus: "new",
	})
	require.NoError(t, err)

	err = UpdateDownloadError(db, "DL003", 1, "network timeout")
	require.NoError(t, err)

	got, err := GetBook(db, "DL003")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "error", got.Status)
	assert.Equal(t, 1, got.RetryCount)
	assert.Equal(t, "network timeout", got.LastError)
}

func TestListDownloadable(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name     string
		book     Book
		expected bool
	}{
		{
			name: "new remote book - should appear",
			book: Book{
				ASIN:          "LD001",
				Title:         "New Remote",
				Author:        "Author",
				Status:        "unknown",
				AudibleStatus: "new",
			},
			expected: true,
		},
		{
			name: "downloaded book - should NOT appear",
			book: Book{
				ASIN:          "LD002",
				Title:         "Downloaded",
				Author:        "Author",
				Status:        "downloaded",
				LocalPath:     "/library/path",
				AudibleStatus: "finished",
			},
			expected: false,
		},
		{
			name: "organized book - should NOT appear",
			book: Book{
				ASIN:          "LD003",
				Title:         "Organized",
				Author:        "Author",
				Status:        "organized",
				LocalPath:     "/library/path",
				AudibleStatus: "finished",
			},
			expected: false,
		},
		{
			name: "error status book - should appear",
			book: Book{
				ASIN:          "LD004",
				Title:         "Error Book",
				Author:        "Author",
				Status:        "error",
				AudibleStatus: "finished",
			},
			expected: true,
		},
		{
			name: "scanned with audible_status - should appear",
			book: Book{
				ASIN:          "LD005",
				Title:         "Scanned Remote",
				Author:        "Author",
				Status:        "scanned",
				AudibleStatus: "in_progress",
			},
			expected: true,
		},
		{
			name: "no audible_status - should NOT appear",
			book: Book{
				ASIN:   "LD006",
				Title:  "Local Only",
				Author: "Author",
				Status: "scanned",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		if tt.book.Status == "" {
			tt.book.Status = "unknown"
		}
		err := InsertBook(db, tt.book)
		require.NoError(t, err, "inserting %s", tt.name)
	}

	books, err := ListDownloadable(db)
	require.NoError(t, err)

	gotASINs := make(map[string]bool)
	for _, b := range books {
		gotASINs[b.ASIN] = true
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected {
				assert.True(t, gotASINs[tt.book.ASIN], "expected %s to be downloadable", tt.book.ASIN)
			} else {
				assert.False(t, gotASINs[tt.book.ASIN], "expected %s to NOT be downloadable", tt.book.ASIN)
			}
		})
	}
}

func TestListDownloadable_Empty(t *testing.T) {
	db := setupTestDB(t)

	// Insert only books that shouldn't be downloadable
	err := InsertBook(db, Book{ASIN: "LDE001", Title: "Local", Author: "A", Status: "scanned"})
	require.NoError(t, err)

	books, err := ListDownloadable(db)
	require.NoError(t, err)
	assert.NotNil(t, books, "should return empty slice, not nil")
	assert.Empty(t, books)
}

func TestScanBookNewColumns(t *testing.T) {
	db := setupTestDB(t)

	// Insert a book and set download tracking fields
	err := InsertBook(db, Book{
		ASIN:          "SCAN001",
		Title:         "Scan Test",
		Author:        "Author",
		Status:        "unknown",
		AudibleStatus: "new",
	})
	require.NoError(t, err)

	// Use UpdateDownloadStart to set download_started_at
	err = UpdateDownloadStart(db, "SCAN001")
	require.NoError(t, err)

	// Use UpdateDownloadError to set retry_count and last_error
	err = UpdateDownloadError(db, "SCAN001", 3, "test error")
	require.NoError(t, err)

	got, err := GetBook(db, "SCAN001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 3, got.RetryCount)
	assert.Equal(t, "test error", got.LastError)
	assert.NotNil(t, got.DownloadStartedAt)
	// download_completed_at should still be nil
	assert.Nil(t, got.DownloadCompletedAt)
}

func TestMigration004Applied(t *testing.T) {
	db := setupTestDB(t)

	// Verify migration 004 was recorded
	var version int
	err := db.QueryRow("SELECT version FROM schema_versions WHERE version = 4").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 4, version)

	// Verify new columns exist with correct defaults
	_, err = db.Exec(`INSERT INTO books (asin) VALUES ('MIGTEST004')`)
	require.NoError(t, err)

	var retryCount int
	var lastError string
	err = db.QueryRow(`SELECT retry_count, last_error FROM books WHERE asin = 'MIGTEST004'`).
		Scan(&retryCount, &lastError)
	require.NoError(t, err)
	assert.Equal(t, 0, retryCount)
	assert.Equal(t, "", lastError)
}
