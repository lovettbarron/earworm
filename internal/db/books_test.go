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
			name: "scanned with audible_status - should NOT appear (already local)",
			book: Book{
				ASIN:          "LD005",
				Title:         "Scanned Remote",
				Author:        "Author",
				Status:        "scanned",
				AudibleStatus: "in_progress",
			},
			expected: false,
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

func TestListOrganizable(t *testing.T) {
	db := setupTestDB(t)

	// Insert books with various statuses
	books := []Book{
		{ASIN: "ORG001", Title: "Downloaded Book 1", Author: "Author A", Status: "downloaded", AudibleStatus: "finished"},
		{ASIN: "ORG002", Title: "Downloaded Book 2", Author: "Author B", Status: "downloaded", AudibleStatus: "new"},
		{ASIN: "ORG003", Title: "Scanned Book", Author: "Author C", Status: "scanned"},
		{ASIN: "ORG004", Title: "Organized Book", Author: "Author D", Status: "organized", AudibleStatus: "finished"},
		{ASIN: "ORG005", Title: "Error Book", Author: "Author E", Status: "error", AudibleStatus: "finished"},
		{ASIN: "ORG006", Title: "Downloading Book", Author: "Author F", Status: "downloading", AudibleStatus: "new"},
	}
	for _, b := range books {
		err := InsertBook(db, b)
		require.NoError(t, err)
	}

	result, err := ListOrganizable(db)
	require.NoError(t, err)
	assert.Len(t, result, 2, "should return only 'downloaded' books")

	asins := make(map[string]bool)
	for _, b := range result {
		asins[b.ASIN] = true
	}
	assert.True(t, asins["ORG001"])
	assert.True(t, asins["ORG002"])
}

func TestListOrganizable_Empty(t *testing.T) {
	db := setupTestDB(t)

	// Insert only non-downloaded books
	err := InsertBook(db, Book{ASIN: "ORG010", Title: "Scanned", Author: "A", Status: "scanned"})
	require.NoError(t, err)

	result, err := ListOrganizable(db)
	require.NoError(t, err)
	assert.NotNil(t, result, "should return empty slice, not nil")
	assert.Empty(t, result)
}

func TestUpdateOrganizeResult(t *testing.T) {
	db := setupTestDB(t)

	err := InsertBook(db, Book{
		ASIN:          "UOR001",
		Title:         "Organize Me",
		Author:        "Author",
		Status:        "downloaded",
		AudibleStatus: "finished",
	})
	require.NoError(t, err)

	// Update to organized with local path
	err = UpdateOrganizeResult(db, "UOR001", "organized", "/library/Author/Organize Me [UOR001]", "")
	require.NoError(t, err)

	got, err := GetBook(db, "UOR001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "organized", got.Status)
	assert.Equal(t, "/library/Author/Organize Me [UOR001]", got.LocalPath)
	assert.Equal(t, "", got.LastError)
}

func TestUpdateOrganizeResult_Error(t *testing.T) {
	db := setupTestDB(t)

	err := InsertBook(db, Book{
		ASIN:   "UOR002",
		Title:  "Fail Book",
		Author: "Author",
		Status: "downloaded",
	})
	require.NoError(t, err)

	// Update to error with message
	err = UpdateOrganizeResult(db, "UOR002", "error", "", "author is required")
	require.NoError(t, err)

	got, err := GetBook(db, "UOR002")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "error", got.Status)
	assert.Equal(t, "", got.LocalPath)
	assert.Equal(t, "author is required", got.LastError)
}

func TestUpdateOrganizeResult_InvalidStatus(t *testing.T) {
	db := setupTestDB(t)

	err := InsertBook(db, Book{ASIN: "UOR003", Title: "Test", Author: "A", Status: "downloaded"})
	require.NoError(t, err)

	err = UpdateOrganizeResult(db, "UOR003", "invalid_status", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestUpdateOrganizeResult_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := UpdateOrganizeResult(db, "NONEXISTENT", "organized", "/path", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetBookByLocalPath_Found(t *testing.T) {
	db := setupTestDB(t)

	err := UpsertBook(db, Book{
		ASIN:      "GBLP001",
		Title:     "Found Book",
		Author:    "Test Author",
		Status:    "organized",
		LocalPath: "/library/Test Author/Found Book [GBLP001]",
	})
	require.NoError(t, err)

	got, err := GetBookByLocalPath(db, "/library/Test Author/Found Book [GBLP001]")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "GBLP001", got.ASIN)
	assert.Equal(t, "Found Book", got.Title)
	assert.Equal(t, "Test Author", got.Author)
	assert.Equal(t, "/library/Test Author/Found Book [GBLP001]", got.LocalPath)
}

func TestGetBookByLocalPath_NotFound(t *testing.T) {
	db := setupTestDB(t)

	got, err := GetBookByLocalPath(db, "/nonexistent/path")
	require.NoError(t, err)
	assert.Nil(t, got, "should return nil for nonexistent path")
}

func TestGetBookByLocalPath_CleanPath(t *testing.T) {
	db := setupTestDB(t)

	err := UpsertBook(db, Book{
		ASIN:      "GBLP002",
		Title:     "Clean Path Book",
		Author:    "Author",
		Status:    "organized",
		LocalPath: "/library/Author/Clean Path Book [GBLP002]",
	})
	require.NoError(t, err)

	// Query with trailing slash — filepath.Clean should normalize
	got, err := GetBookByLocalPath(db, "/library/Author/Clean Path Book [GBLP002]/")
	require.NoError(t, err)
	require.NotNil(t, got, "should find book after path normalization")
	assert.Equal(t, "GBLP002", got.ASIN)
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
