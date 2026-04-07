package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration005Applied(t *testing.T) {
	db := setupTestDB(t)

	// Verify migration 005 was recorded
	var version int
	err := db.QueryRow("SELECT version FROM schema_versions WHERE version = 5").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 5, version)

	// Verify all four tables exist
	tables := []string{"library_items", "plans", "plan_operations", "audit_log"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		require.NoError(t, err, "table %s should exist", table)
		assert.Equal(t, table, name)
	}
}

func TestUpsertLibraryItem(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	item := LibraryItem{
		Path:           "/library/Author Name/Book Title",
		ItemType:       "audiobook",
		Title:          "Test Book",
		Author:         "Test Author",
		ASIN:           "B00TEST1234",
		FolderName:     "Book Title [B00TEST1234]",
		FileCount:      3,
		TotalSizeBytes: 1048576,
		HasCover:       true,
		MetadataSource: "tag",
		LastScannedAt:  &now,
	}
	err := UpsertLibraryItem(db, item)
	require.NoError(t, err)

	got, err := GetLibraryItem(db, "/library/Author Name/Book Title")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "/library/Author Name/Book Title", got.Path)
	assert.Equal(t, "audiobook", got.ItemType)
	assert.Equal(t, "Test Book", got.Title)
	assert.Equal(t, "Test Author", got.Author)
	assert.Equal(t, "B00TEST1234", got.ASIN)
	assert.Equal(t, "Book Title [B00TEST1234]", got.FolderName)
	assert.Equal(t, 3, got.FileCount)
	assert.Equal(t, int64(1048576), got.TotalSizeBytes)
	assert.True(t, got.HasCover)
	assert.Equal(t, "tag", got.MetadataSource)
	assert.NotNil(t, got.LastScannedAt)
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestUpsertLibraryItemUpdate(t *testing.T) {
	db := setupTestDB(t)

	// Insert initial version
	item := LibraryItem{
		Path:     "/library/Author/Book",
		ItemType: "audiobook",
		Title:    "Original Title",
		Author:   "Original Author",
	}
	err := UpsertLibraryItem(db, item)
	require.NoError(t, err)

	// Get original created_at
	got, err := GetLibraryItem(db, "/library/Author/Book")
	require.NoError(t, err)
	originalCreatedAt := got.CreatedAt

	// Small delay to ensure updated_at changes
	time.Sleep(10 * time.Millisecond)

	// Upsert with updated fields
	updated := LibraryItem{
		Path:           "/library/Author/Book",
		ItemType:       "audiobook",
		Title:          "Updated Title",
		Author:         "Updated Author",
		ASIN:           "B00UPDATED",
		FileCount:      5,
		TotalSizeBytes: 2097152,
		HasCover:       true,
		MetadataSource: "ffprobe",
	}
	err = UpsertLibraryItem(db, updated)
	require.NoError(t, err)

	got, err = GetLibraryItem(db, "/library/Author/Book")
	require.NoError(t, err)
	require.NotNil(t, got)

	// Verify fields updated
	assert.Equal(t, "Updated Title", got.Title)
	assert.Equal(t, "Updated Author", got.Author)
	assert.Equal(t, "B00UPDATED", got.ASIN)
	assert.Equal(t, 5, got.FileCount)
	assert.Equal(t, int64(2097152), got.TotalSizeBytes)
	assert.True(t, got.HasCover)
	assert.Equal(t, "ffprobe", got.MetadataSource)

	// created_at should be preserved
	assert.Equal(t, originalCreatedAt.Unix(), got.CreatedAt.Unix())

	// updated_at should change
	assert.True(t, got.UpdatedAt.Unix() >= originalCreatedAt.Unix())
}

func TestGetLibraryItemNotFound(t *testing.T) {
	db := setupTestDB(t)

	got, err := GetLibraryItem(db, "/nonexistent/path")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestListLibraryItems(t *testing.T) {
	db := setupTestDB(t)

	// Empty list returns empty slice, not nil
	items, err := ListLibraryItems(db)
	require.NoError(t, err)
	assert.NotNil(t, items)
	assert.Empty(t, items)

	// Insert items
	for _, path := range []string{"/z/item", "/a/item", "/m/item"} {
		err := UpsertLibraryItem(db, LibraryItem{
			Path:     path,
			ItemType: "audiobook",
			Title:    "Book at " + path,
		})
		require.NoError(t, err)
	}

	// Populated list returns items in path order
	items, err = ListLibraryItems(db)
	require.NoError(t, err)
	assert.Len(t, items, 3)
	assert.Equal(t, "/a/item", items[0].Path)
	assert.Equal(t, "/m/item", items[1].Path)
	assert.Equal(t, "/z/item", items[2].Path)
}

func TestDeleteLibraryItem(t *testing.T) {
	db := setupTestDB(t)

	// Insert an item
	err := UpsertLibraryItem(db, LibraryItem{
		Path:     "/library/delete-me",
		ItemType: "audiobook",
		Title:    "Delete Me",
	})
	require.NoError(t, err)

	// Delete existing returns nil
	err = DeleteLibraryItem(db, "/library/delete-me")
	assert.NoError(t, err)

	// Verify it's gone
	got, err := GetLibraryItem(db, "/library/delete-me")
	assert.NoError(t, err)
	assert.Nil(t, got)

	// Delete non-existent returns error containing "not found"
	err = DeleteLibraryItem(db, "/nonexistent/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/foo/bar/", "/foo/bar"},
		{"/foo/bar", "/foo/bar"},
		{"/foo//bar", "/foo/bar"},
		{"/foo/./bar", "/foo/bar"},
		{"/foo/bar/../baz", "/foo/baz"},
		{".", "."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizePath(tt.input))
		})
	}
}

func TestInvalidItemType(t *testing.T) {
	db := setupTestDB(t)

	item := LibraryItem{
		Path:     "/library/bad-type",
		ItemType: "invalid_type",
		Title:    "Bad Type",
	}
	err := UpsertLibraryItem(db, item)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid item type")
}

func TestPathNormalizationPreventsDoubles(t *testing.T) {
	db := setupTestDB(t)

	// Insert with trailing slash
	err := UpsertLibraryItem(db, LibraryItem{
		Path:     "/foo/bar/",
		ItemType: "audiobook",
		Title:    "First Title",
	})
	require.NoError(t, err)

	// Upsert without trailing slash (should update, not create new)
	err = UpsertLibraryItem(db, LibraryItem{
		Path:     "/foo/bar",
		ItemType: "audiobook",
		Title:    "Second Title",
	})
	require.NoError(t, err)

	// Should have exactly 1 item with the second title
	items, err := ListLibraryItems(db)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "Second Title", items[0].Title)
}
