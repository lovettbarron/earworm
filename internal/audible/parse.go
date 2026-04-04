package audible

import (
	"encoding/json"
	"fmt"
)

// LibraryItem represents a single book from audible-cli library export.
// Pointer types are used where null values are possible from audible-cli JSON output.
type LibraryItem struct {
	ASIN             string  `json:"asin"`
	Title            string  `json:"title"`
	Subtitle         string  `json:"subtitle"`
	Authors          string  `json:"authors"`
	Narrators        string  `json:"narrators"`
	SeriesTitle      string  `json:"series_title"`
	SeriesSequence   string  `json:"series_sequence"`
	RuntimeLengthMin *int    `json:"runtime_length_min"` // pointer: may be null for podcasts
	PurchaseDate     string  `json:"purchase_date"`
	ReleaseDate      string  `json:"release_date"`
	IsFinished       bool    `json:"is_finished"`
	PercentComplete  float64 `json:"percent_complete"`
	Genres           string  `json:"genres"`
	Rating           string  `json:"rating"`
	NumRatings       *int    `json:"num_ratings"` // pointer: may be null
	CoverURL         string  `json:"cover_url"`
}

// RuntimeMinutes returns the runtime in minutes, defaulting to 0 if nil.
func (li *LibraryItem) RuntimeMinutes() int {
	if li.RuntimeLengthMin == nil {
		return 0
	}
	return *li.RuntimeLengthMin
}

// AudibleStatus derives the status string from IsFinished and PercentComplete.
func (li *LibraryItem) AudibleStatus() string {
	if li.IsFinished {
		return "finished"
	}
	if li.PercentComplete > 0 {
		return "in_progress"
	}
	return "new"
}

// ParseLibraryExport parses audible-cli JSON export output into a LibraryItem slice.
func ParseLibraryExport(data []byte) ([]LibraryItem, error) {
	var items []LibraryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse library export: %w", err)
	}
	return items, nil
}
