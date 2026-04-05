package scanner

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovettbarron/earworm/internal/db"
)

// BookMetadata holds metadata extracted from audiobook files or folder names.
// This is used as the return type for the metadata extraction callback in IncrementalSync.
type BookMetadata struct {
	Title        string
	Author       string
	Narrator     string
	Genre        string
	Year         int
	Series       string
	HasCover     bool
	Duration     int // seconds
	ChapterCount int
	FileCount    int
	Source       string // "tag", "ffprobe", "folder"
}

// DiscoveredBook represents an audiobook folder found during scanning.
type DiscoveredBook struct {
	ASIN      string
	Title     string   // parsed from folder name (ASIN stripped)
	Author    string   // parent directory name
	LocalPath string   // absolute path to book directory
	AudioFiles []string // absolute paths to .m4a/.m4b files in directory
}

// SkippedDir represents a directory that was skipped during scanning.
type SkippedDir struct {
	Path   string
	Reason string // "no_asin", "permission_denied", "not_directory"
}

// ScanResult holds the counts from an incremental sync operation.
type ScanResult struct {
	Added   int
	Updated int
	Removed int
	Skipped int
}

// ScanLibrary scans a library root directory for audiobook folders containing ASINs.
// If recursive is true, it walks the entire tree; otherwise it scans two levels (Author/Title).
func ScanLibrary(root string, recursive bool) ([]DiscoveredBook, []SkippedDir, error) {
	if recursive {
		return scanRecursive(root)
	}
	return scanTwoLevel(root)
}

// scanTwoLevel scans one or two levels: root/Title [ASIN]/ (flat) or root/Author/Title [ASIN]/.
// It auto-detects the layout by checking if top-level directories contain ASINs.
func scanTwoLevel(root string) ([]DiscoveredBook, []SkippedDir, error) {
	var discovered []DiscoveredBook
	var skipped []SkippedDir

	topEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read library root %s: %w", root, err)
	}

	for _, entry := range topEntries {
		if !entry.IsDir() {
			continue
		}

		entryPath := filepath.Join(root, entry.Name())

		// Check if this top-level dir itself has an ASIN (flat layout)
		if asin, ok := ExtractASIN(entry.Name()); ok {
			title := stripASIN(entry.Name())
			audioFiles := findAudioFiles(entryPath)

			discovered = append(discovered, DiscoveredBook{
				ASIN:       asin,
				Title:      title,
				Author:     "", // no author directory in flat layout
				LocalPath:  entryPath,
				AudioFiles: audioFiles,
			})
			continue
		}

		// Otherwise treat as Author directory — scan one level deeper
		titleEntries, err := os.ReadDir(entryPath)
		if err != nil {
			if os.IsPermission(err) {
				skipped = append(skipped, SkippedDir{
					Path:   entryPath,
					Reason: "permission_denied",
				})
				slog.Warn("permission denied reading directory", "path", entryPath)
				continue
			}
			return nil, nil, fmt.Errorf("read author dir %s: %w", entryPath, err)
		}

		for _, titleEntry := range titleEntries {
			if !titleEntry.IsDir() {
				continue
			}

			titlePath := filepath.Join(entryPath, titleEntry.Name())
			asin, ok := ExtractASIN(titleEntry.Name())
			if !ok {
				skipped = append(skipped, SkippedDir{
					Path:   titlePath,
					Reason: "no_asin",
				})
				continue
			}

			title := stripASIN(titleEntry.Name())
			audioFiles := findAudioFiles(titlePath)

			discovered = append(discovered, DiscoveredBook{
				ASIN:       asin,
				Title:      title,
				Author:     entry.Name(),
				LocalPath:  titlePath,
				AudioFiles: audioFiles,
			})
		}
	}

	return discovered, skipped, nil
}

// scanRecursive walks the entire directory tree looking for ASIN-bearing folders.
func scanRecursive(root string) ([]DiscoveredBook, []SkippedDir, error) {
	var discovered []DiscoveredBook
	var skipped []SkippedDir

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				skipped = append(skipped, SkippedDir{
					Path:   path,
					Reason: "permission_denied",
				})
				slog.Warn("permission denied", "path", path)
				return nil
			}
			return err
		}

		if !d.IsDir() {
			return nil
		}

		// Skip the root directory itself
		if path == root {
			return nil
		}

		asin, ok := ExtractASIN(d.Name())
		if !ok {
			return nil // continue walking, might have ASIN dirs deeper
		}

		title := stripASIN(d.Name())
		author := filepath.Base(filepath.Dir(path))
		m4aFiles := findAudioFiles(path)

		discovered = append(discovered, DiscoveredBook{
			ASIN:      asin,
			Title:     title,
			Author:    author,
			LocalPath: path,
			AudioFiles:  m4aFiles,
		})

		// Don't descend into this directory; we've already processed it
		return filepath.SkipDir
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk library %s: %w", root, err)
	}

	return discovered, skipped, nil
}

// stripASIN removes the ASIN pattern and surrounding brackets/parens from a folder name.
func stripASIN(name string) string {
	// Remove ASIN in brackets: [B08C6YJ1LS]
	result := asinBracketPattern.ReplaceAllString(name, "")
	// Remove ASIN in parens: (B08C6YJ1LS)
	result = asinParenPattern.ReplaceAllString(result, "")
	// Remove standalone ASIN
	result = asinPattern.ReplaceAllString(result, "")
	// Clean up whitespace
	result = strings.TrimSpace(result)
	return result
}

// findAudioFiles returns sorted absolute paths to .m4a/.m4b files in a directory.
func findAudioFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".m4a" || ext == ".m4b" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files
}

// IncrementalSync synchronizes discovered books with the database.
// New books are inserted, existing books are updated, and books no longer found are marked "removed".
func IncrementalSync(database *sql.DB, discovered []DiscoveredBook, metadataFn func(string) (*BookMetadata, error)) (*ScanResult, error) {
	result := &ScanResult{}

	// Get existing books from DB
	existing, err := db.ListBooks(database)
	if err != nil {
		return nil, fmt.Errorf("list existing books: %w", err)
	}

	// Build map of existing books by ASIN
	existingMap := make(map[string]db.Book)
	for _, b := range existing {
		existingMap[b.ASIN] = b
	}

	// Track which ASINs we see in the scan
	seenASINs := make(map[string]bool)

	// Process discovered books
	for _, d := range discovered {
		seenASINs[d.ASIN] = true

		// Get metadata
		var meta *BookMetadata
		if metadataFn != nil {
			meta, err = metadataFn(d.LocalPath)
			if err != nil {
				slog.Warn("metadata extraction failed, using folder info", "asin", d.ASIN, "error", err)
				meta = &BookMetadata{Source: "folder"}
			}
		} else {
			meta = &BookMetadata{Source: "folder"}
		}

		// Build the book record, preferring metadata over folder-parsed values
		title := d.Title
		if meta.Title != "" {
			title = meta.Title
		}
		author := d.Author
		if meta.Author != "" {
			author = meta.Author
		}

		book := db.Book{
			ASIN:           d.ASIN,
			Title:          title,
			Author:         author,
			Narrator:       meta.Narrator,
			Genre:          meta.Genre,
			Year:           meta.Year,
			Series:         meta.Series,
			HasCover:       meta.HasCover,
			Duration:       meta.Duration,
			ChapterCount:   meta.ChapterCount,
			MetadataSource: meta.Source,
			FileCount:      len(d.AudioFiles),
			Status:         "scanned",
			LocalPath:      d.LocalPath,
		}

		_, existed := existingMap[d.ASIN]
		if err := db.UpsertBook(database, book); err != nil {
			return nil, fmt.Errorf("upsert book %s: %w", d.ASIN, err)
		}

		if existed {
			result.Updated++
		} else {
			result.Added++
		}
	}

	// Mark books not seen in scan as "removed"
	for _, b := range existing {
		if !seenASINs[b.ASIN] && b.Status != "removed" {
			if err := db.UpdateBookStatus(database, b.ASIN, "removed"); err != nil {
				return nil, fmt.Errorf("mark book %s as removed: %w", b.ASIN, err)
			}
			result.Removed++
		}
	}

	return result, nil
}
