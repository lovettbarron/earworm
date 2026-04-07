package fileops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lovettbarron/earworm/internal/metadata"
)

// SidecarFileName is the name of the Audiobookshelf metadata sidecar file.
const SidecarFileName = "metadata.json"

// ABSMetadata represents the Audiobookshelf-compatible metadata.json schema.
type ABSMetadata struct {
	Tags          []string     `json:"tags"`
	Chapters      []ABSChapter `json:"chapters"`
	Title         string       `json:"title"`
	Subtitle      string       `json:"subtitle"`
	Authors       []string     `json:"authors"`
	Narrators     []string     `json:"narrators"`
	Series        []string     `json:"series"`
	Genres        []string     `json:"genres"`
	PublishedYear string       `json:"publishedYear"`
	PublishedDate string       `json:"publishedDate"`
	Publisher     string       `json:"publisher"`
	Description   string       `json:"description"`
	ISBN          string       `json:"isbn"`
	ASIN          string       `json:"asin"`
	Language      string       `json:"language"`
	Explicit      bool         `json:"explicit"`
	Abridged      bool         `json:"abridged"`
}

// ABSChapter represents a chapter entry in the ABS metadata.
type ABSChapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

// BuildABSMetadata converts internal BookMetadata to Audiobookshelf-compatible format.
func BuildABSMetadata(bookMeta *metadata.BookMetadata, asin string) ABSMetadata {
	abs := ABSMetadata{
		Title:    bookMeta.Title,
		ASIN:     asin,
		Authors:  toSlice(bookMeta.Author),
		Narrators: toSlice(bookMeta.Narrator),
		Series:   toSlice(bookMeta.Series),
		Genres:   toSlice(bookMeta.Genre),
		Tags:     []string{},
		Chapters: []ABSChapter{},
	}

	if bookMeta.Year > 0 {
		abs.PublishedYear = strconv.Itoa(bookMeta.Year)
	}

	return abs
}

// WriteMetadataSidecar writes an ABSMetadata struct as pretty-printed JSON
// to metadata.json in the given book directory.
func WriteMetadataSidecar(bookDir string, meta ABSMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("write metadata sidecar: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(bookDir, SidecarFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write metadata sidecar: %w", err)
	}
	return nil
}

// toSlice converts a string to a single-element slice, or an empty slice if empty.
func toSlice(s string) []string {
	if s == "" {
		return []string{}
	}
	return []string{s}
}
