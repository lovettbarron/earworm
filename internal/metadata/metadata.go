package metadata

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MetadataSource indicates where metadata was extracted from.
type MetadataSource string

const (
	SourceTag     MetadataSource = "tag"
	SourceFFprobe MetadataSource = "ffprobe"
	SourceFolder  MetadataSource = "folder"
)

// BookMetadata holds metadata extracted from audiobook files or folder names.
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
	Source       MetadataSource
}

// ExtractMetadata extracts metadata for a book directory using the fallback chain:
// dhowden/tag -> ffprobe -> folder name parsing.
func ExtractMetadata(bookDir string) (*BookMetadata, error) {
	m4aFiles := FindM4AFiles(bookDir)

	if len(m4aFiles) == 0 {
		meta := extractFromFolderName(bookDir)
		return meta, nil
	}

	// Try dhowden/tag on the first M4A file
	meta, err := extractWithTag(m4aFiles[0])
	if err == nil && meta.Title != "" {
		meta.FileCount = len(m4aFiles)
		return meta, nil
	}

	// Try ffprobe fallback
	meta, err = extractWithFFprobe(m4aFiles[0])
	if err == nil {
		meta.FileCount = len(m4aFiles)
		return meta, nil
	}

	// Fall through to folder name parsing
	meta = extractFromFolderName(bookDir)
	meta.FileCount = len(m4aFiles)
	return meta, nil
}

// FindM4AFiles returns sorted absolute paths to .m4a files in a directory.
// Case-insensitive extension matching.
func FindM4AFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".m4a") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	sort.Strings(files)
	return files
}
