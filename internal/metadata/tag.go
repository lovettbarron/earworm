package metadata

import (
	"fmt"
	"os"

	"github.com/dhowden/tag"
)

// extractWithTag extracts metadata from an M4A file using dhowden/tag.
func extractWithTag(filePath string) (*BookMetadata, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", filePath, err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("read tags from %s: %w", filePath, err)
	}

	meta := &BookMetadata{
		Title:  m.Title(),
		Genre:  m.Genre(),
		Year:   m.Year(),
		Source: SourceTag,
	}

	// Author: prefer AlbumArtist, fall back to Artist
	meta.Author = m.AlbumArtist()
	if meta.Author == "" {
		meta.Author = m.Artist()
	}

	// HasCover: check if picture data exists
	meta.HasCover = m.Picture() != nil

	// Narrator: try raw tags for narrator-specific fields
	raw := m.Raw()
	if raw != nil {
		// Try common narrator atom keys
		if nrt, ok := raw["\u00a9nrt"]; ok {
			meta.Narrator = fmt.Sprintf("%v", nrt)
		}
		if meta.Narrator == "" {
			if nrt, ok := raw["narrator"]; ok {
				meta.Narrator = fmt.Sprintf("%v", nrt)
			}
		}

		// If no explicit narrator, Audible often puts narrator in artist field
		if meta.Narrator == "" && m.AlbumArtist() != "" {
			meta.Narrator = m.Artist()
		}

		// Series: try iTunes-style series atom
		if series, ok := raw["----:com.apple.iTunes:SERIES"]; ok {
			meta.Series = fmt.Sprintf("%v", series)
		}
		if meta.Series == "" {
			if series, ok := raw["tvsh"]; ok {
				meta.Series = fmt.Sprintf("%v", series)
			}
		}
	}

	return meta, nil
}
