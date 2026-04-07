package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lovettbarron/earworm/internal/metadata"
)

// IssueType identifies the kind of issue detected in a book directory.
type IssueType string

const (
	IssueNoASIN          IssueType = "no_asin"
	IssueNestedAudio     IssueType = "nested_audio"
	IssueMultiBook       IssueType = "multi_book"
	IssueMissingMetadata IssueType = "missing_metadata"
	IssueWrongStructure  IssueType = "wrong_structure"
	IssueOrphanFiles     IssueType = "orphan_files"
	IssueEmptyDir        IssueType = "empty_dir"
	IssueCoverMissing    IssueType = "cover_missing"
)

// Severity indicates how critical a detected issue is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// DetectedIssue represents a problem found in a book directory during scanning.
type DetectedIssue struct {
	Path            string
	IssueType       IssueType
	Severity        Severity
	Message         string
	SuggestedAction string
}

// DetectIssues runs all 8 issue detectors against a book directory and returns
// all detected issues. Each detector is a pure function that can be tested in isolation.
func DetectIssues(dirPath string, entries []os.DirEntry, meta *BookMetadata, libraryRoot string) []DetectedIssue {
	var issues []DetectedIssue
	issues = append(issues, detectEmptyDir(dirPath, entries)...)
	issues = append(issues, detectNoASIN(dirPath, entries)...)
	issues = append(issues, detectNestedAudio(dirPath, entries)...)
	issues = append(issues, detectOrphanFiles(dirPath, entries)...)
	issues = append(issues, detectCoverMissing(dirPath, entries, meta)...)
	issues = append(issues, detectMissingMetadata(dirPath, entries, meta)...)
	issues = append(issues, detectWrongStructure(dirPath, entries, libraryRoot)...)
	issues = append(issues, detectMultiBook(dirPath, entries)...)
	return issues
}

// detectEmptyDir checks if a directory has no entries at all.
func detectEmptyDir(dirPath string, entries []os.DirEntry) []DetectedIssue {
	if len(entries) == 0 {
		return []DetectedIssue{{
			Path:            dirPath,
			IssueType:       IssueEmptyDir,
			Severity:        SeverityWarning,
			Message:         "Directory is empty",
			SuggestedAction: "Delete empty directory",
		}}
	}
	return nil
}

// detectNoASIN checks for audio files in a directory whose folder name lacks an ASIN.
func detectNoASIN(dirPath string, entries []os.DirEntry) []DetectedIssue {
	if !hasAudioFiles(entries) {
		return nil
	}
	if _, ok := ExtractASIN(filepath.Base(dirPath)); ok {
		return nil
	}
	return []DetectedIssue{{
		Path:            dirPath,
		IssueType:       IssueNoASIN,
		Severity:        SeverityWarning,
		Message:         "Audio files present but no ASIN in folder name",
		SuggestedAction: "Add ASIN to folder name: rename to 'Title [ASIN]'",
	}}
}

// detectNestedAudio checks if any subdirectory contains audio files.
// This is the only detector that reads from the filesystem beyond the provided entries.
func detectNestedAudio(dirPath string, entries []os.DirEntry) []DetectedIssue {
	var nestedDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subPath := filepath.Join(dirPath, entry.Name())
		audioFiles := metadata.FindAudioFiles(subPath)
		if len(audioFiles) > 0 {
			nestedDirs = append(nestedDirs, entry.Name())
		}
	}
	if len(nestedDirs) == 0 {
		return nil
	}
	return []DetectedIssue{{
		Path:            dirPath,
		IssueType:       IssueNestedAudio,
		Severity:        SeverityWarning,
		Message:         fmt.Sprintf("Audio files found in subdirectories: %s", strings.Join(nestedDirs, ", ")),
		SuggestedAction: "Flatten: move audio files up to book directory",
	}}
}

// knownExtensions lists file extensions that are expected in a book directory.
var knownExtensions = map[string]bool{
	".m4a":  true,
	".m4b":  true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".json": true,
	".nfo":  true,
	".cue":  true,
	".txt":  true,
	".log":  true,
	".xml":  true,
}

// detectOrphanFiles checks for files with unexpected extensions.
func detectOrphanFiles(dirPath string, entries []os.DirEntry) []DetectedIssue {
	var orphans []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == "" || !knownExtensions[ext] {
			orphans = append(orphans, entry.Name())
		}
	}
	if len(orphans) == 0 {
		return nil
	}
	return []DetectedIssue{{
		Path:            dirPath,
		IssueType:       IssueOrphanFiles,
		Severity:        SeverityInfo,
		Message:         fmt.Sprintf("Unexpected files found: %s", strings.Join(orphans, ", ")),
		SuggestedAction: "Review and remove or relocate orphan files",
	}}
}

// detectCoverMissing checks for the absence of cover art when audio files are present.
func detectCoverMissing(dirPath string, entries []os.DirEntry, meta *BookMetadata) []DetectedIssue {
	if !hasAudioFiles(entries) {
		return nil
	}
	// Check if metadata reports embedded cover
	if meta != nil && meta.HasCover {
		return nil
	}
	// Check for image files in directory
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			return nil
		}
	}
	return []DetectedIssue{{
		Path:            dirPath,
		IssueType:       IssueCoverMissing,
		Severity:        SeverityInfo,
		Message:         "No cover image found",
		SuggestedAction: "Add cover image to book directory",
	}}
}

// detectMissingMetadata checks for books where metadata extraction failed and no sidecar exists.
func detectMissingMetadata(dirPath string, entries []os.DirEntry, meta *BookMetadata) []DetectedIssue {
	if meta != nil {
		return nil
	}
	if !hasAudioFiles(entries) {
		return nil
	}
	// Check for metadata.json sidecar
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() == "metadata.json" {
			return nil
		}
	}
	return []DetectedIssue{{
		Path:            dirPath,
		IssueType:       IssueMissingMetadata,
		Severity:        SeverityInfo,
		Message:         "No metadata extractable from audio files",
		SuggestedAction: "Write metadata.json sidecar with known information",
	}}
}

// detectWrongStructure checks if a directory is nested too deeply from the library root.
// Expected structure is Author/Title [ASIN] (depth 2 from root). Deeper nesting is flagged.
func detectWrongStructure(dirPath string, entries []os.DirEntry, libraryRoot string) []DetectedIssue {
	if !hasAudioFiles(entries) {
		return nil
	}
	rel, err := filepath.Rel(libraryRoot, dirPath)
	if err != nil {
		return nil
	}
	// Count path separators to determine depth
	depth := strings.Count(rel, string(filepath.Separator)) + 1
	if depth <= 2 {
		return nil
	}
	return []DetectedIssue{{
		Path:            dirPath,
		IssueType:       IssueWrongStructure,
		Severity:        SeverityInfo,
		Message:         fmt.Sprintf("Directory is nested too deeply (%d levels from library root)", depth),
		SuggestedAction: "Restructure: move to Author/Title [ASIN] format",
	}}
}

// stripNumericPrefixSuffix removes common numbering patterns from filenames for title grouping.
// Strips patterns like "01 - ", "Track 01 ", " Part 1", " - Chapter 1", etc.
var numericPrefixPattern = regexp.MustCompile(`^(?:\d+\s*[-.:]\s*|(?:Track|Disc|Part|Chapter)\s*\d+\s*[-.:]*\s*)`)
var numericSuffixPattern = regexp.MustCompile(`\s*[-.:]\s*(?:Track|Disc|Part|Chapter)?\s*\d+\s*$`)

// titleSeparatorPattern matches " - " used to separate title from chapter/track info.
var titleSeparatorPattern = regexp.MustCompile(`\s*-\s*`)

// detectMultiBook uses a conservative heuristic to detect multiple distinct books in one directory.
// It extracts the title portion (text before " - ") from each audio filename, strips numeric
// prefixes/suffixes, and checks if there are genuinely different title groups. Files that are
// just numbered chapters of the same book (e.g., "01 - Chapter One", "02 - Chapter Two") are NOT flagged.
func detectMultiBook(dirPath string, entries []os.DirEntry) []DetectedIssue {
	var audioNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".m4a" || ext == ".m4b" {
			base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			audioNames = append(audioNames, base)
		}
	}

	if len(audioNames) < 2 {
		return nil
	}

	// Extract title portion: for "Title - Chapter N" patterns, take the part before " - "
	// For purely numbered files like "01 - Chapter One", strip numeric prefix first
	titles := make(map[string]bool)
	for _, name := range audioNames {
		title := extractTitle(name)
		titles[strings.ToLower(title)] = true
	}

	if len(titles) < 2 {
		return nil
	}

	return []DetectedIssue{{
		Path:            dirPath,
		IssueType:       IssueMultiBook,
		Severity:        SeverityWarning,
		Message:         "Multiple distinct titles detected in single directory",
		SuggestedAction: "Split: separate into individual book directories",
	}}
}

// extractTitle extracts a representative title from an audio filename for grouping.
// It handles patterns like:
//   - "The Hobbit - Chapter 1" -> "the hobbit"
//   - "01 - Chapter One" -> "" (purely numeric prefix, treated as generic chapter)
//   - "Lord of the Rings - Chapter 1" -> "lord of the rings"
// purelyNumeric matches strings that are only digits (e.g., "01", "123").
var purelyNumeric = regexp.MustCompile(`^\d+$`)

func extractTitle(name string) string {
	// Split on " - " separator
	parts := titleSeparatorPattern.Split(name, 2)
	if len(parts) >= 2 {
		candidate := strings.TrimSpace(parts[0])
		// If the part before " - " is purely numeric, this is a numbered track
		// (e.g., "01 - Chapter One") — return empty to group them together
		if purelyNumeric.MatchString(candidate) {
			return "" // purely numeric prefix, all such files group together
		}
		stripped := numericPrefixPattern.ReplaceAllString(candidate, "")
		if stripped == "" {
			return "" // purely numeric prefix after stripping
		}
		return stripped
	}
	// No separator — strip numeric prefix/suffix and use remaining
	stripped := numericPrefixPattern.ReplaceAllString(name, "")
	stripped = numericSuffixPattern.ReplaceAllString(stripped, "")
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return name
	}
	return stripped
}

// hasAudioFiles checks if any entry has an audio file extension (.m4a, .m4b).
func hasAudioFiles(entries []os.DirEntry) bool {
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".m4a" || ext == ".m4b" {
			return true
		}
	}
	return false
}
