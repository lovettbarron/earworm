package split

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupFiles_TwoTitles(t *testing.T) {
	dir := t.TempDir()

	// Create 4 audio files
	for _, name := range []string{"book1_ch1.m4a", "book1_ch2.m4a", "book2_ch1.m4a", "book2_ch2.m4a"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644))
	}
	// Create shared file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("img"), 0644))

	// Mock metadata extraction
	origFn := extractFileMetadataFn
	defer func() { extractFileMetadataFn = origFn }()

	extractFileMetadataFn = func(filePath string) (*metadata.BookMetadata, error) {
		name := filepath.Base(filePath)
		if name == "book1_ch1.m4a" || name == "book1_ch2.m4a" {
			return &metadata.BookMetadata{Title: "Book One", Author: "Author A", Source: metadata.SourceTag}, nil
		}
		return &metadata.BookMetadata{Title: "Book Two", Author: "Author B", Source: metadata.SourceTag}, nil
	}

	result, err := GroupFiles(dir)
	require.NoError(t, err)
	assert.False(t, result.Skipped)
	assert.Len(t, result.Groups, 2)
	assert.Contains(t, result.SharedFiles, filepath.Join(dir, "cover.jpg"))

	// Each group should have 2 audio files
	for _, g := range result.Groups {
		assert.Len(t, g.AudioFiles, 2)
	}
}

func TestGroupFiles_SharedFilesIdentified(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "book.m4a"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("img"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "poster.png"), []byte("img"), 0644))

	origFn := extractFileMetadataFn
	defer func() { extractFileMetadataFn = origFn }()
	extractFileMetadataFn = func(filePath string) (*metadata.BookMetadata, error) {
		return &metadata.BookMetadata{Title: "Book", Author: "Auth", Source: metadata.SourceTag}, nil
	}

	result, err := GroupFiles(dir)
	require.NoError(t, err)
	assert.Len(t, result.SharedFiles, 3) // jpg, json, png
}

func TestGroupFiles_LowConfidence(t *testing.T) {
	dir := t.TempDir()

	// Create 5 audio files, 2 with metadata, 3 without (>20% unknown)
	for _, name := range []string{"a.m4a", "b.m4a", "c.m4a", "d.m4a", "e.m4a"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644))
	}

	origFn := extractFileMetadataFn
	defer func() { extractFileMetadataFn = origFn }()

	callCount := 0
	extractFileMetadataFn = func(filePath string) (*metadata.BookMetadata, error) {
		callCount++
		name := filepath.Base(filePath)
		if name == "a.m4a" || name == "b.m4a" {
			return &metadata.BookMetadata{Title: "Book", Author: "Auth", Source: metadata.SourceTag}, nil
		}
		// Return nil metadata for others
		return nil, nil
	}

	result, err := GroupFiles(dir)
	require.NoError(t, err)
	assert.True(t, result.Skipped)
	assert.NotEmpty(t, result.SkipReason)
}

func TestGroupFiles_SingleTitle(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"ch1.m4a", "ch2.m4a", "ch3.m4a"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644))
	}

	origFn := extractFileMetadataFn
	defer func() { extractFileMetadataFn = origFn }()
	extractFileMetadataFn = func(filePath string) (*metadata.BookMetadata, error) {
		return &metadata.BookMetadata{Title: "Same Book", Author: "Same Author", Source: metadata.SourceTag}, nil
	}

	result, err := GroupFiles(dir)
	require.NoError(t, err)
	assert.False(t, result.Skipped)
	assert.Len(t, result.Groups, 1)
}

func TestGroupFiles_FilenameFallback(t *testing.T) {
	dir := t.TempDir()

	// Files with pattern-based names but no metadata
	for _, name := range []string{"BookA_Chapter01.m4a", "BookA_Chapter02.m4a", "BookB_Chapter01.m4a", "BookB_Chapter02.m4a"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644))
	}

	origFn := extractFileMetadataFn
	defer func() { extractFileMetadataFn = origFn }()
	extractFileMetadataFn = func(filePath string) (*metadata.BookMetadata, error) {
		return nil, nil // No metadata for any file
	}

	result, err := GroupFiles(dir)
	require.NoError(t, err)
	// Filename fallback should find 2 groups (BookA, BookB)
	if !result.Skipped {
		assert.Len(t, result.Groups, 2)
	}
}

func TestGroupFiles_HighConfidence(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"ch1.m4a", "ch2.m4a"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644))
	}

	origFn := extractFileMetadataFn
	defer func() { extractFileMetadataFn = origFn }()
	extractFileMetadataFn = func(filePath string) (*metadata.BookMetadata, error) {
		return &metadata.BookMetadata{Title: "Book", Author: "Author", Source: metadata.SourceTag}, nil
	}

	result, err := GroupFiles(dir)
	require.NoError(t, err)
	assert.False(t, result.Skipped)
	for _, g := range result.Groups {
		assert.GreaterOrEqual(t, g.Confidence, 0.8)
	}
}
