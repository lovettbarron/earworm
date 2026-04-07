package fileops

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FlattenResult holds the outcome of flattening a book directory.
type FlattenResult struct {
	BookDir     string
	FilesMoved  []FileMoveResult
	DirsRemoved []string
	Errors      []error
}

// FileMoveResult records the outcome of a single file move during flattening.
type FileMoveResult struct {
	SourcePath string
	DestPath   string
	SHA256     string
	Success    bool
	Error      string
}

// FlattenDir moves all nested audio files (.m4a/.m4b) up to the book folder
// root. It handles filename collisions with numeric suffixes, verifies each
// move with SHA-256 hashing, and cleans up empty subdirectories bottom-up.
// Per-file errors do not abort the remaining files.
func FlattenDir(bookDir string) (*FlattenResult, error) {
	result := &FlattenResult{BookDir: bookDir}

	// Collect nested audio files
	var nestedFiles []string
	err := filepath.WalkDir(bookDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip files already at root level
		if filepath.Dir(path) == bookDir {
			return nil
		}
		// Match audio extensions case-insensitively
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".m4a" || ext == ".m4b" {
			nestedFiles = append(nestedFiles, path)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("walking %s: %w", bookDir, err)
	}

	// Move each nested audio file to root
	for _, src := range nestedFiles {
		dst := uniquePath(filepath.Join(bookDir, filepath.Base(src)))

		moveErr := VerifiedMove(src, dst)
		if moveErr != nil {
			result.FilesMoved = append(result.FilesMoved, FileMoveResult{
				SourcePath: src,
				DestPath:   dst,
				Success:    false,
				Error:      moveErr.Error(),
			})
			result.Errors = append(result.Errors, moveErr)
			continue
		}

		// Get the hash of the moved file for the result record
		hash, _ := HashFile(dst)
		result.FilesMoved = append(result.FilesMoved, FileMoveResult{
			SourcePath: src,
			DestPath:   dst,
			SHA256:     hash,
			Success:    true,
		})
	}

	// Clean up empty subdirectories bottom-up
	result.DirsRemoved = removeEmptyDirs(bookDir)

	return result, nil
}

// uniquePath returns path if it does not exist. Otherwise it appends a numeric
// suffix (_1, _2, ...) before the extension until a non-existing path is found.
func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	for i := 1; i <= 999; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}

	// Fallback: return the 999th path (will likely cause a move error)
	return fmt.Sprintf("%s_%d%s", base, 999, ext)
}

// removeEmptyDirs walks the directory tree and removes empty subdirectories
// bottom-up (deepest first). The root directory itself is never removed.
func removeEmptyDirs(root string) []string {
	var dirs []string

	// Collect all subdirectories
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})

	// Sort by depth descending (deepest first)
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) >
			strings.Count(dirs[j], string(filepath.Separator))
	})

	var removed []string
	for _, dir := range dirs {
		// os.Remove only succeeds on empty directories
		if err := os.Remove(dir); err == nil {
			removed = append(removed, dir)
		}
	}

	return removed
}
