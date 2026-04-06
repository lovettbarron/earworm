package download

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dhowden/tag"
)

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
