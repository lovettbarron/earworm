// Package split provides multi-book folder detection and split plan generation.
// It analyzes audio file metadata to identify multiple books co-located in a
// single directory and creates plan operations to separate them.
package split

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovettbarron/earworm/internal/metadata"
)

// extractFileMetadataFn is a test seam for metadata extraction.
var extractFileMetadataFn = metadata.ExtractFileMetadata

// BookGroup represents a detected group of audio files belonging to a single book.
type BookGroup struct {
	Title      string
	Author     string
	Narrator   string
	ASIN       string
	AudioFiles []string // absolute paths
	Confidence float64  // 0.0-1.0
}

// GroupResult contains the outcome of analyzing a directory for multi-book content.
type GroupResult struct {
	SourceDir   string
	Groups      []BookGroup
	SharedFiles []string // covers, metadata.json, etc.
	Skipped     bool
	SkipReason  string
}

// fileInfo pairs a file path with its extracted metadata (may be nil).
type fileInfo struct {
	path string
	meta *metadata.BookMetadata
}

// sharedFileExts defines non-audio file extensions to treat as shared files.
var sharedFileExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".json": true,
}

// GroupFiles analyzes a directory and clusters audio files by metadata.
// It uses per-file metadata extraction (tag -> ffprobe) and falls back to
// filename pattern analysis when metadata is sparse.
func GroupFiles(dirPath string) (*GroupResult, error) {
	audioFiles := metadata.FindAudioFiles(dirPath)
	if len(audioFiles) == 0 {
		return nil, fmt.Errorf("no audio files found in %s", dirPath)
	}

	result := &GroupResult{
		SourceDir: dirPath,
	}

	// Collect shared (non-audio) files
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", dirPath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if sharedFileExts[ext] {
			result.SharedFiles = append(result.SharedFiles, filepath.Join(dirPath, entry.Name()))
		}
	}

	// Extract metadata for each audio file
	var infos []fileInfo
	var nilMetaCount int

	for _, f := range audioFiles {
		meta, err := extractFileMetadataFn(f)
		if err != nil || meta == nil {
			infos = append(infos, fileInfo{path: f, meta: nil})
			nilMetaCount++
		} else {
			infos = append(infos, fileInfo{path: f, meta: meta})
		}
	}

	// If ALL files have nil metadata, try filename fallback
	if nilMetaCount == len(infos) {
		return groupByFilename(result, infos)
	}

	// Group by normalized (title + "|" + author) key
	groups := make(map[string]*BookGroup)
	var unknownFiles []string

	for _, info := range infos {
		if info.meta == nil || (info.meta.Title == "" && info.meta.Author == "") {
			unknownFiles = append(unknownFiles, info.path)
			continue
		}

		key := strings.ToLower(info.meta.Title) + "|" + strings.ToLower(info.meta.Author)
		g, ok := groups[key]
		if !ok {
			g = &BookGroup{
				Title:    info.meta.Title,
				Author:   info.meta.Author,
				Narrator: info.meta.Narrator,
			}
			groups[key] = g
		}
		g.AudioFiles = append(g.AudioFiles, info.path)
	}

	// Check unknown ratio — too many unidentified files means low confidence
	if len(unknownFiles) > 0 {
		unknownRatio := float64(len(unknownFiles)) / float64(len(infos))
		if unknownRatio > 0.2 {
			result.Skipped = true
			result.SkipReason = "too many files without metadata for confident grouping"
			return result, nil
		}
	}

	// If only 1 group, merge unknowns into it (unknowns are <=20% at this point)
	if len(groups) == 1 {
		for _, g := range groups {
			g.AudioFiles = append(g.AudioFiles, unknownFiles...)
			g.Confidence = 1.0
			result.Groups = []BookGroup{*g}
		}
		return result, nil
	}

	// Calculate per-group confidence
	minConfidence := 1.0
	for _, g := range groups {
		// All files in a group matched the same key, so confidence = 1.0
		// unless we want to account for title variations
		g.Confidence = 1.0
		if g.Confidence < minConfidence {
			minConfidence = g.Confidence
		}
	}

	if minConfidence < 0.7 {
		result.Skipped = true
		result.SkipReason = fmt.Sprintf("confidence below threshold: %.2f", minConfidence)
		return result, nil
	}

	// Build result groups
	for _, g := range groups {
		result.Groups = append(result.Groups, *g)
	}

	return result, nil
}

// groupByFilename attempts to group files by common filename prefixes when
// metadata is unavailable. Looks for patterns like "BookA_Chapter01.m4a".
func groupByFilename(result *GroupResult, infos []fileInfo) (*GroupResult, error) {
	// Try to find common prefixes by splitting on common delimiters
	prefixGroups := make(map[string][]string)

	for _, info := range infos {
		name := filepath.Base(info.path)
		name = strings.TrimSuffix(name, filepath.Ext(name))

		// Try splitting by common delimiters: _, -, space
		prefix := extractPrefix(name)
		if prefix == "" {
			prefix = "_unknown"
		}
		prefixGroups[prefix] = append(prefixGroups[prefix], info.path)
	}

	if len(prefixGroups) <= 1 {
		// Can't split — either all same prefix or no pattern found
		result.Skipped = true
		result.SkipReason = "unable to determine file groupings from filenames"
		return result, nil
	}

	// Only accept filename grouping if we found at least 2 distinct groups
	// and no single "unknown" bucket dominates
	for prefix, files := range prefixGroups {
		result.Groups = append(result.Groups, BookGroup{
			Title:      prefix,
			Author:     "",
			AudioFiles: files,
			Confidence: 0.5, // Lower confidence for filename-based grouping
		})
	}

	return result, nil
}

// extractPrefix extracts a title-like prefix from a filename by splitting
// on common delimiters (underscore, dash, space followed by digits).
func extractPrefix(name string) string {
	// Try underscore delimiter: "BookA_Chapter01" -> "BookA"
	if idx := strings.Index(name, "_"); idx > 0 {
		return name[:idx]
	}
	// Try dash delimiter: "BookA-Chapter01" -> "BookA"
	if idx := strings.Index(name, "-"); idx > 0 {
		return name[:idx]
	}
	// Try space+digit: "Book A 01" -> "Book A"
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] >= '0' && name[i] <= '9' {
			continue
		}
		if name[i] == ' ' && i < len(name)-1 {
			return strings.TrimSpace(name[:i])
		}
		break
	}
	return ""
}
