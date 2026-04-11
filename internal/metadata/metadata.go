package metadata

import (
	"fmt"
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
	m4aFiles := FindAudioFiles(bookDir)

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

// ExtractFileMetadata extracts metadata for a single audio file using the fallback chain:
// dhowden/tag -> ffprobe. Unlike ExtractMetadata, this does NOT fall back to folder name parsing.
// Returns an error if both extraction methods fail.
func ExtractFileMetadata(filePath string) (*BookMetadata, error) {
	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("extract file metadata: %w", err)
	}

	// Try dhowden/tag first
	meta, err := extractWithTag(filePath)
	if err == nil && meta.Title != "" {
		meta.FileCount = 1
		return meta, nil
	}

	// Try ffprobe fallback
	meta, err = extractWithFFprobe(filePath)
	if err == nil {
		meta.FileCount = 1
		return meta, nil
	}

	// Both failed — return error (no folder fallback for per-file extraction)
	return nil, fmt.Errorf("extract file metadata: no metadata found in %s", filePath)
}

// FindAudioFiles returns sorted absolute paths to .m4a/.m4b files in a directory.
// Case-insensitive extension matching.
func FindAudioFiles(dir string) []string {
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

	sort.Strings(files)
	return files
}
