package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ValidItemTypes defines the allowed library item type values.
var ValidItemTypes = []string{"book", "audiobook", "podcast", "unknown"}

// LibraryItem represents a content entry in the library, keyed by filesystem path.
type LibraryItem struct {
	Path           string
	ItemType       string
	Title          string
	Author         string
	ASIN           string
	FolderName     string
	FileCount      int
	TotalSizeBytes int64
	HasCover       bool
	MetadataSource string
	LastScannedAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// isValidItemType checks whether an item type string is in the allowed set.
func isValidItemType(itemType string) bool {
	for _, t := range ValidItemTypes {
		if t == itemType {
			return true
		}
	}
	return false
}

// NormalizePath cleans a filesystem path and strips trailing slashes.
// This prevents duplicate entries from path variations like "/foo/bar/" vs "/foo/bar".
func NormalizePath(p string) string {
	cleaned := filepath.Clean(p)
	// filepath.Clean already removes trailing slashes for non-root paths,
	// but ensure we handle edge cases consistently.
	cleaned = strings.TrimRight(cleaned, string(filepath.Separator))
	if cleaned == "" {
		return "/"
	}
	return cleaned
}

// UpsertLibraryItem inserts a new library item or updates an existing one matched by path.
// On conflict, all fields except path and created_at are updated.
func UpsertLibraryItem(db *sql.DB, item LibraryItem) error {
	item.Path = NormalizePath(item.Path)

	if !isValidItemType(item.ItemType) {
		return fmt.Errorf("invalid item type %q", item.ItemType)
	}

	hasCover := 0
	if item.HasCover {
		hasCover = 1
	}

	_, err := db.Exec(
		`INSERT INTO library_items (path, item_type, title, author, asin, folder_name, file_count, total_size_bytes, has_cover, metadata_source, last_scanned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			item_type = excluded.item_type,
			title = excluded.title,
			author = excluded.author,
			asin = excluded.asin,
			folder_name = excluded.folder_name,
			file_count = excluded.file_count,
			total_size_bytes = excluded.total_size_bytes,
			has_cover = excluded.has_cover,
			metadata_source = excluded.metadata_source,
			last_scanned_at = excluded.last_scanned_at,
			updated_at = CURRENT_TIMESTAMP`,
		item.Path, item.ItemType, item.Title, item.Author, item.ASIN,
		item.FolderName, item.FileCount, item.TotalSizeBytes, hasCover,
		item.MetadataSource, item.LastScannedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert library item %s: %w", item.Path, err)
	}
	return nil
}

// scanLibraryItem scans a row into a LibraryItem struct, handling has_cover int->bool
// and nullable LastScannedAt conversions.
func scanLibraryItem(scanner interface{ Scan(dest ...any) error }) (*LibraryItem, error) {
	var item LibraryItem
	var hasCover int
	var lastScannedAt sql.NullTime

	err := scanner.Scan(
		&item.Path, &item.ItemType, &item.Title, &item.Author, &item.ASIN,
		&item.FolderName, &item.FileCount, &item.TotalSizeBytes, &hasCover,
		&item.MetadataSource, &lastScannedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	item.HasCover = hasCover != 0
	if lastScannedAt.Valid {
		item.LastScannedAt = &lastScannedAt.Time
	}
	return &item, nil
}

// libraryItemColumns is the shared column list for SELECT queries on library_items.
const libraryItemColumns = `path, item_type, title, author, asin, folder_name, file_count, total_size_bytes, has_cover, metadata_source, last_scanned_at, created_at, updated_at`

// GetLibraryItem retrieves a library item by path. Returns nil and no error if not found.
func GetLibraryItem(db *sql.DB, path string) (*LibraryItem, error) {
	path = NormalizePath(path)

	row := db.QueryRow(
		`SELECT `+libraryItemColumns+` FROM library_items WHERE path = ?`,
		path,
	)

	item, err := scanLibraryItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get library item %s: %w", path, err)
	}
	return item, nil
}

// ListLibraryItems returns all library items ordered by path ascending.
// Returns an empty slice (not nil) when no items exist.
func ListLibraryItems(db *sql.DB) ([]LibraryItem, error) {
	rows, err := db.Query(
		`SELECT ` + libraryItemColumns + ` FROM library_items ORDER BY path ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list library items: %w", err)
	}
	defer rows.Close()

	var items []LibraryItem
	for rows.Next() {
		item, err := scanLibraryItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan library item row: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate library items: %w", err)
	}

	if items == nil {
		items = []LibraryItem{}
	}
	return items, nil
}

// DeleteLibraryItem deletes a library item by path.
// Returns an error if the path does not exist.
func DeleteLibraryItem(db *sql.DB, path string) error {
	path = NormalizePath(path)

	result, err := db.Exec(
		`DELETE FROM library_items WHERE path = ?`,
		path,
	)
	if err != nil {
		return fmt.Errorf("delete library item %s: %w", path, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("library item %s not found", path)
	}
	return nil
}
