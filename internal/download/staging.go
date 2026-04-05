package download

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dhowden/tag"
)

// sanitizeFolderName removes characters illegal in file/folder names across platforms.
func sanitizeFolderName(name string) string {
	// Replace characters illegal on Windows/macOS/Linux
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", " -", "*", "", "?", "", "\"", "'",
		"<", "", ">", "", "|", "-",
	)
	result := replacer.Replace(name)
	result = strings.TrimSpace(result)
	// Collapse multiple spaces
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return result
}

// asinPattern matches ASIN-like directory names: 10 alphanumeric characters
// starting with B (standard Audible ASIN format).
var asinPattern = regexp.MustCompile(`^B[A-Z0-9]{9}$`)

// VerifyM4A checks that a file exists, is non-zero size, and has readable
// audio metadata via dhowden/tag. Per D-13.
func VerifyM4A(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file %q: %w", filePath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file %q: %w", filePath, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("file %q has zero size", filePath)
	}

	if _, err := tag.ReadFrom(f); err != nil {
		return fmt.Errorf("reading metadata from %q: %w", filePath, err)
	}

	return nil
}

// MoveToLibrary moves all files from stagingDir/asin/ to libraryDir/Title [ASIN]/.
// Uses os.Rename with copy+delete fallback for cross-filesystem moves (Pitfall 1).
// Creates destination directory if needed. Title is used for the Libation-compatible
// folder name; if empty, falls back to bare ASIN.
func MoveToLibrary(stagingDir, libraryDir, asin, title string) error {
	src := filepath.Join(stagingDir, asin)
	folderName := asin
	if title != "" {
		folderName = fmt.Sprintf("%s [%s]", sanitizeFolderName(title), asin)
	}
	dst := filepath.Join(libraryDir, folderName)

	// Ensure destination directory exists
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("creating library directory %q: %w", dst, err)
	}

	// Walk source directory and move each file
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("reading staging directory %q: %w", src, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // skip subdirectories for now
		}
		srcFile := filepath.Join(src, entry.Name())
		dstFile := filepath.Join(dst, entry.Name())

		if err := moveFile(srcFile, dstFile); err != nil {
			return fmt.Errorf("moving %q to %q: %w", srcFile, dstFile, err)
		}
	}

	// Remove the now-empty staging ASIN directory
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("removing staging directory %q: %w", src, err)
	}

	return nil
}

// moveFile moves a file from src to dst. Tries os.Rename first, falls back
// to copy+delete for cross-filesystem moves.
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Fallback: copy + delete for cross-filesystem moves
	return copyAndDelete(src, dst)
}

// copyAndDelete copies src to dst then removes src.
func copyAndDelete(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source %q: %w", src, err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("creating destination %q: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copying %q to %q: %w", src, dst, err)
	}

	// Close both before deleting source
	srcFile.Close()
	dstFile.Close()

	if err := os.Remove(src); err != nil {
		return fmt.Errorf("removing source %q after copy: %w", src, err)
	}

	return nil
}

// CleanOrphans removes ASIN-named subdirectories from staging that don't have
// matching "downloaded" status. Per D-07.
// Only operates on directories that look like ASINs (10 alphanumeric chars starting with B).
// Never deletes the staging root itself.
func CleanOrphans(stagingDir string, downloadedASINs map[string]bool) error {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return fmt.Errorf("reading staging directory %q: %w", stagingDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Only remove directories that look like ASINs
		if !asinPattern.MatchString(name) {
			continue
		}

		// Keep directories for ASINs that have been downloaded
		if downloadedASINs[name] {
			continue
		}

		// Remove orphaned ASIN directory
		dirPath := filepath.Join(stagingDir, name)
		if err := os.RemoveAll(dirPath); err != nil {
			return fmt.Errorf("removing orphan directory %q: %w", dirPath, err)
		}
	}

	return nil
}
