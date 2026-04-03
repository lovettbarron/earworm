package metadata

import (
	"path/filepath"
	"regexp"
	"strings"
)

// asinPattern matches ASINs for stripping from folder names.
var asinStripPattern = regexp.MustCompile(`\s*[\[\(]?B[0-9A-Z]{9}[\]\)]?\s*`)

// extractFromFolderName parses metadata from the folder name convention Author/Title [ASIN].
func extractFromFolderName(bookDir string) *BookMetadata {
	titleDir := filepath.Base(bookDir)
	authorDir := filepath.Base(filepath.Dir(bookDir))

	// Strip ASIN and surrounding brackets/parens from title
	title := asinStripPattern.ReplaceAllString(titleDir, "")
	title = strings.TrimSpace(title)

	// If title ended up empty after stripping, use the original
	if title == "" {
		title = titleDir
	}

	return &BookMetadata{
		Title:  title,
		Author: authorDir,
		Source: SourceFolder,
	}
}
