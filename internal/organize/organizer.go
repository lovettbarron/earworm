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
	ASIN    string `json:"asin"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	LibPath string `json:"lib_path,omitempty"` // final library path
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// OrganizeBook moves a book's files from the staging directory into the library
// with the correct Libation-compatible folder structure: Author/Title [ASIN]/.
// The M4A file is renamed to Title.m4a, cover images to cover.jpg, and chapter
// metadata to chapters.json. Returns the destination directory path on success.
func OrganizeBook(book db.Book, stagingDir, libraryDir string) (string, error) {
	// Build the relative library path (validates author/title)
	relPath, err := BuildBookPath(book.Author, book.Title, book.ASIN)
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
	case ".m4a":
		return RenameM4AFile(title)
	case ".jpg", ".jpeg", ".png":
		return "cover.jpg"
	case ".json":
		return "chapters.json"
	default:
		return name
	}
}

// OrganizeAll processes all books with 'downloaded' status, moving their files
// from staging into the library and updating the database. It returns results
// for all books (both successes and failures). Individual book failures do not
// stop processing of remaining books.
func OrganizeAll(database *sql.DB, stagingDir, libraryDir string) ([]OrganizeResult, error) {
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

		destDir, err := OrganizeBook(book, stagingDir, libraryDir)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			// Mark as error in DB
			if dbErr := db.UpdateOrganizeResult(database, book.ASIN, "error", "", err.Error()); dbErr != nil {
				result.Error = fmt.Sprintf("%s (db update also failed: %s)", result.Error, dbErr.Error())
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
