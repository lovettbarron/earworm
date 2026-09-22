package organize

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovettbarron/earworm/internal/db"
)

// OrganizeResult holds the outcome of organizing a single book.
type OrganizeResult struct {
	ASIN      string `json:"asin"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	LibPath   string `json:"lib_path,omitempty"` // final library path
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Retryable bool   `json:"retryable,omitempty"` // true if files remain in staging for retry
}

// ValidateLibraryPath checks that the library directory exists and is writable.
// Call before OrganizeAll to fail fast when the NAS/mount is unavailable.
func ValidateLibraryPath(libraryDir string) error {
	info, err := os.Stat(libraryDir)
	if err != nil {
		return fmt.Errorf("library path unreachable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("library path is not a directory: %s", libraryDir)
	}
	tmp := filepath.Join(libraryDir, ".earworm-write-test")
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("library path not writable: %w", err)
	}
	f.Close()
	os.Remove(tmp)
	return nil
}

// OrganizeBook moves a book's files from the staging directory into the library.
// The layout parameter controls directory structure: "flat" produces Title [ASIN]/,
// "author-title" produces Author/Title [ASIN]/. The M4A file is renamed to
// Title.m4a, cover images to cover.jpg, and chapter metadata to chapters.json.
func OrganizeBook(book db.Book, stagingDir, libraryDir, layout string) (string, error) {
	relPath, err := BuildBookPath(book.Author, book.Title, book.ASIN, layout)
	if err != nil {
		return "", fmt.Errorf("build book path: %w", err)
	}

	srcDir := filepath.Join(stagingDir, book.ASIN)
	destDir := filepath.Join(libraryDir, relPath)

	// Create destination directory hierarchy (D-13: may already exist)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create destination directory: %w", err)
	}

	// List all files in staging ASIN directory
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return "", fmt.Errorf("read staging directory %s: %w", srcDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		srcFile := filepath.Join(srcDir, entry.Name())
		dstName := destinationFilename(entry.Name(), book.Title)
		if dstName == "" {
			continue // skip files that should not be organized (e.g., decrypt artifacts)
		}
		dstFile := filepath.Join(destDir, dstName)

		if err := MoveFile(srcFile, dstFile); err != nil {
			return "", fmt.Errorf("move %s: %w", entry.Name(), err)
		}
	}

	// Remove empty staging directory (ignore error if not empty)
	os.Remove(srcDir)

	return destDir, nil
}

// destinationFilename determines the correct destination filename based on
// the source filename. M4A files are renamed to Title.m4a, cover images to
// cover.jpg, chapter JSON to chapters.json, and everything else keeps its name.
func destinationFilename(name, title string) string {
	ext := strings.ToLower(filepath.Ext(name))

	switch ext {
	case ".m4b":
		return RenameAudioFile(title, ".m4b")
	case ".m4a":
		return RenameAudioFile(title, ".m4a")
	case ".jpg", ".jpeg", ".png":
		return "cover.jpg"
	case ".json":
		return "chapters.json"
	case ".voucher", ".aaxc":
		return "" // skip decrypt artifacts (should already be removed)
	default:
		return name
	}
}

// OrganizeAll processes all books with 'downloaded' status, moving their files
// from staging into the library and updating the database. The layout parameter
// controls directory structure ("flat" or "author-title"). It returns results
// for all books (both successes and failures). Individual book failures do not
// stop processing of remaining books.
func OrganizeAll(database *sql.DB, stagingDir, libraryDir, layout string) ([]OrganizeResult, error) {
	books, err := db.ListOrganizable(database)
	if err != nil {
		return nil, fmt.Errorf("list organizable books: %w", err)
	}

	var results []OrganizeResult

	for _, book := range books {
		result := OrganizeResult{
			ASIN:   book.ASIN,
			Title:  book.Title,
			Author: book.Author,
		}

		destDir, err := OrganizeBook(book, stagingDir, libraryDir, layout)
		if err != nil {
			result.Success = false
			result.Error = err.Error()

			// Distinguish retryable filesystem errors from permanent metadata errors.
			// If the error is a transfer/filesystem issue AND staging files still exist,
			// keep as "downloaded" so the book is retried on next organize run.
			isTransferError := !strings.Contains(err.Error(), "build book path:")
			asinStaging := filepath.Join(stagingDir, book.ASIN)
			hasStaged := false
			if entries, statErr := os.ReadDir(asinStaging); statErr == nil && len(entries) > 0 {
				hasStaged = true
			}

			if isTransferError && hasStaged {
				result.Retryable = true
			} else {
				if dbErr := db.UpdateOrganizeResult(database, book.ASIN, "error", "", err.Error()); dbErr != nil {
					result.Error = fmt.Sprintf("%s (db update also failed: %s)", result.Error, dbErr.Error())
				}
			}
		} else {
			result.Success = true
			result.LibPath = destDir
			// Mark as organized in DB
			if dbErr := db.UpdateOrganizeResult(database, book.ASIN, "organized", destDir, ""); dbErr != nil {
				result.Success = false
				result.Error = fmt.Sprintf("organized files but db update failed: %s", dbErr.Error())
			}
		}

		results = append(results, result)
	}

	if results == nil {
		results = []OrganizeResult{}
	}

	return results, nil
}
