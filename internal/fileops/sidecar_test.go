package fileops

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestWriteMetadataSidecar_WritesJSON(t *testing.T) {
	dir := t.TempDir()

	meta := ABSMetadata{
		Title:         "Test Book",
		Authors:       []string{"Test Author"},
		Narrators:     []string{"Test Narrator"},
		Genres:        []string{"Sci-Fi"},
		PublishedYear: "2024",
		ASIN:          "B012345678",
		Tags:          []string{},
		Chapters:      []ABSChapter{},
		Series:        []string{},
	}

	err := WriteMetadataSidecar(dir, meta)
	require.NoError(t, err)

	path := filepath.Join(dir, "metadata.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var result ABSMetadata
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "Test Book", result.Title)
	assert.Equal(t, []string{"Test Author"}, result.Authors)
	assert.Equal(t, "2024", result.PublishedYear)
	assert.Equal(t, "B012345678", result.ASIN)
}

func TestWriteMetadataSidecar_PrettyPrinted(t *testing.T) {
	dir := t.TempDir()

	meta := ABSMetadata{
		Title:    "Pretty Test",
		Authors:  []string{},
		Narrators: []string{},
		Series:   []string{},
		Genres:   []string{},
		Tags:     []string{},
		Chapters: []ABSChapter{},
	}

	err := WriteMetadataSidecar(dir, meta)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, "\n  ", "JSON should be 2-space indented")
}

func TestWriteMetadataSidecar_Overwrite(t *testing.T) {
	dir := t.TempDir()

	meta1 := ABSMetadata{
		Title: "First Title", Authors: []string{}, Narrators: []string{},
		Series: []string{}, Genres: []string{}, Tags: []string{}, Chapters: []ABSChapter{},
	}
	meta2 := ABSMetadata{
		Title: "Second Title", Authors: []string{}, Narrators: []string{},
		Series: []string{}, Genres: []string{}, Tags: []string{}, Chapters: []ABSChapter{},
	}

	require.NoError(t, WriteMetadataSidecar(dir, meta1))
	require.NoError(t, WriteMetadataSidecar(dir, meta2))

	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	require.NoError(t, err)

	var result ABSMetadata
	require.NoError(t, json.Unmarshal(data, &result))
	assert.Equal(t, "Second Title", result.Title, "second write should overwrite first")
}

func TestWriteMetadataSidecar_InvalidDir(t *testing.T) {
	err := WriteMetadataSidecar("/nonexistent/path/that/does/not/exist", ABSMetadata{
		Authors: []string{}, Narrators: []string{}, Series: []string{},
		Genres: []string{}, Tags: []string{}, Chapters: []ABSChapter{},
	})
	assert.Error(t, err)
}

func TestSidecarNoAudioModification(t *testing.T) {
	dir := t.TempDir()

	// Create a dummy .m4a file with known content
	m4aPath := filepath.Join(dir, "test.m4a")
	content := []byte("fake audio content for hash verification")
	require.NoError(t, os.WriteFile(m4aPath, content, 0644))

	// Hash before
	hashBefore := sha256File(t, m4aPath)

	// Write sidecar
	meta := ABSMetadata{
		Title: "Audio Test", Authors: []string{"Writer"}, Narrators: []string{},
		Series: []string{}, Genres: []string{}, Tags: []string{}, Chapters: []ABSChapter{},
	}
	require.NoError(t, WriteMetadataSidecar(dir, meta))

	// Hash after
	hashAfter := sha256File(t, m4aPath)

	assert.Equal(t, hashBefore, hashAfter, "audio file must not be modified by sidecar write")
}

func TestWriteMetadataSidecar_JSONFormat(t *testing.T) {
	dir := t.TempDir()

	meta := ABSMetadata{
		Title: "Format Test", Authors: []string{}, Narrators: []string{},
		Series: []string{}, Genres: []string{}, Tags: []string{}, Chapters: []ABSChapter{},
		PublishedYear: "2023",
	}
	require.NoError(t, WriteMetadataSidecar(dir, meta))

	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	require.NoError(t, err)

	raw := string(data)
	for _, key := range []string{
		"publishedYear", "authors", "narrators", "series", "genres", "chapters", "tags",
	} {
		assert.True(t, strings.Contains(raw, `"`+key+`"`), "JSON should contain key %q", key)
	}
}

// sha256File computes the SHA-256 hex digest of a file.
func sha256File(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	h := sha256.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	return fmt.Sprintf("%x", h.Sum(nil))
}
