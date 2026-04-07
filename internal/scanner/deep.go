package scanner

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lovettbarron/earworm/internal/db"
)

// DeepScanResult holds the counts from a deep library scan.
type DeepScanResult struct {
	TotalDirs   int
	WithASIN    int
	WithoutASIN int
	IssuesFound int
	IssueCounts map[IssueType]int
}

// DeepScanLibrary walks all directories under root (not just ASIN-bearing),
// populates library_items, runs issue detection, and persists results.
// metadataFn is optional; when nil, metadata extraction is skipped.
func DeepScanLibrary(root string, database *sql.DB, metadataFn func(string) (*BookMetadata, error)) (*DeepScanResult, error) {
	runID := time.Now().Format("20060102T150405")

	// Clear all previous scan issues before fresh scan
	if err := db.ClearScanIssues(database); err != nil {
		return nil, fmt.Errorf("clear scan issues: %w", err)
	}

	result := &DeepScanResult{
		IssueCounts: make(map[IssueType]int),
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				slog.Warn("permission denied, skipping", "path", path)
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}

		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			if os.IsPermission(readErr) {
				slog.Warn("permission denied reading directory", "path", path)
				return nil
			}
			slog.Warn("cannot read directory", "path", path, "error", readErr)
			return nil
		}

		// Classify directory
		folderName := filepath.Base(path)
		asin, hasASIN := ExtractASIN(folderName)

		// Count audio files
		audioFiles := findAudioFilesInEntries(entries)

		// Calculate total size of files
		var totalSize int64
		for _, e := range entries {
			if !e.IsDir() {
				if info, infoErr := e.Info(); infoErr == nil {
					totalSize += info.Size()
				}
			}
		}

		// Determine item type
		itemType := "unknown"
		if len(audioFiles) > 0 {
			itemType = "audiobook"
		}

		// Extract metadata (only if audio present, to save time)
		var meta *BookMetadata
		if len(audioFiles) > 0 && metadataFn != nil {
			meta, _ = metadataFn(path) // ignore error -- metadata is optional
		}

		// Build LibraryItem
		item := db.LibraryItem{
			Path:           db.NormalizePath(path),
			ItemType:       itemType,
			ASIN:           asin,
			FolderName:     folderName,
			FileCount:      len(entries),
			TotalSizeBytes: totalSize,
		}
		if meta != nil {
			item.Title = meta.Title
			item.Author = meta.Author
			item.HasCover = meta.HasCover
			item.MetadataSource = meta.Source
		}

		now := time.Now()
		item.LastScannedAt = &now

		if upsertErr := db.UpsertLibraryItem(database, item); upsertErr != nil {
			slog.Warn("failed to upsert library item", "path", path, "error", upsertErr)
		}

		// Detect issues
		issues := DetectIssues(path, entries, meta, root)
		for _, issue := range issues {
			scanIssue := db.ScanIssue{
				Path:            db.NormalizePath(issue.Path),
				IssueType:       string(issue.IssueType),
				Severity:        string(issue.Severity),
				Message:         issue.Message,
				SuggestedAction: issue.SuggestedAction,
				ScanRunID:       runID,
			}
			if insertErr := db.InsertScanIssue(database, scanIssue); insertErr != nil {
				slog.Warn("failed to insert scan issue", "path", path, "error", insertErr)
			}
			result.IssuesFound++
			result.IssueCounts[issue.IssueType]++
		}

		result.TotalDirs++
		if hasASIN {
			result.WithASIN++
		} else {
			result.WithoutASIN++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("deep scan: %w", err)
	}

	return result, nil
}

// findAudioFilesInEntries checks dir entries for audio files without reading subdirectories.
func findAudioFilesInEntries(entries []os.DirEntry) []string {
	var audio []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".m4a" || ext == ".m4b" {
			audio = append(audio, e.Name())
		}
	}
	return audio
}
