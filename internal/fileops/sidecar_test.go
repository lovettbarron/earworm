package fileops

import (
	"encoding/json"
	"testing"

	"github.com/lovettbarron/earworm/internal/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildABSMetadata_FullFields(t *testing.T) {
	bookMeta := &metadata.BookMetadata{
		Title:        "The Great Book",
		Author:       "Jane Author",
		Narrator:     "Bob Reader",
		Genre:        "Fiction",
		Year:         2023,
		Series:       "My Series",
		HasCover:     true,
		Duration:     3600,
		ChapterCount: 10,
		FileCount:    3,
	}

	abs := BuildABSMetadata(bookMeta, "B0XXXXXXXX")

	assert.Equal(t, "The Great Book", abs.Title)
	assert.Equal(t, []string{"Jane Author"}, abs.Authors)
	assert.Equal(t, []string{"Bob Reader"}, abs.Narrators)
	assert.Equal(t, []string{"Fiction"}, abs.Genres)
	assert.Equal(t, "2023", abs.PublishedYear)
	assert.Equal(t, []string{"My Series"}, abs.Series)
	assert.Equal(t, "B0XXXXXXXX", abs.ASIN)
}

func TestBuildABSMetadata_EmptyFields(t *testing.T) {
	bookMeta := &metadata.BookMetadata{}

	abs := BuildABSMetadata(bookMeta, "")

	assert.Equal(t, "", abs.Title)
	assert.Equal(t, "", abs.PublishedYear)
	assert.Equal(t, "", abs.ASIN)
	assert.NotNil(t, abs.Authors)
	assert.NotNil(t, abs.Narrators)
	assert.NotNil(t, abs.Series)
	assert.NotNil(t, abs.Genres)
	assert.NotNil(t, abs.Tags)
	assert.NotNil(t, abs.Chapters)
	assert.Empty(t, abs.Authors)
	assert.Empty(t, abs.Narrators)
	assert.Empty(t, abs.Series)
	assert.Empty(t, abs.Genres)
	assert.Empty(t, abs.Tags)
	assert.Empty(t, abs.Chapters)
}

func TestBuildABSMetadata_ZeroYear(t *testing.T) {
	bookMeta := &metadata.BookMetadata{Year: 0}

	abs := BuildABSMetadata(bookMeta, "")

	assert.Equal(t, "", abs.PublishedYear, "zero year should produce empty string, not '0'")
}

func TestBuildABSMetadata_ArraysNeverNil(t *testing.T) {
	bookMeta := &metadata.BookMetadata{}

	abs := BuildABSMetadata(bookMeta, "")

	data, err := json.Marshal(abs)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"tags":[]`)
	assert.Contains(t, jsonStr, `"chapters":[]`)
	assert.Contains(t, jsonStr, `"authors":[]`)
	assert.Contains(t, jsonStr, `"narrators":[]`)
	assert.Contains(t, jsonStr, `"series":[]`)
	assert.Contains(t, jsonStr, `"genres":[]`)
	assert.NotContains(t, jsonStr, "null")
}
