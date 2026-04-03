package metadata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindM4AFiles(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "book.m4a"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "UPPER.M4A"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("fake"), 0644))

	files := FindM4AFiles(dir)
	assert.Len(t, files, 2)

	// Verify paths are absolute and end with .m4a/.M4A
	for _, f := range files {
		assert.True(t, filepath.IsAbs(f))
		ext := filepath.Ext(f)
		assert.True(t, ext == ".m4a" || ext == ".M4A", "unexpected extension: %s", ext)
	}
}

func TestFindM4AFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	files := FindM4AFiles(dir)
	assert.Empty(t, files)
}

func TestFindM4AFilesNonexistent(t *testing.T) {
	files := FindM4AFiles("/nonexistent/path")
	assert.Nil(t, files)
}

func TestExtractFromFolderName(t *testing.T) {
	tests := []struct {
		name       string
		bookDir    string
		wantTitle  string
		wantAuthor string
	}{
		{
			name:       "standard Libation format with brackets",
			bookDir:    "/library/Andy Weir/Project Hail Mary [B08C6YJ1LS]",
			wantTitle:  "Project Hail Mary",
			wantAuthor: "Andy Weir",
		},
		{
			name:       "ASIN in parens",
			bookDir:    "/library/Brandon Sanderson/Mistborn (B09ABCDEF1)",
			wantTitle:  "Mistborn",
			wantAuthor: "Brandon Sanderson",
		},
		{
			name:       "standalone ASIN",
			bookDir:    "/library/Author/Title B08C6YJ1LS",
			wantTitle:  "Title",
			wantAuthor: "Author",
		},
		{
			name:       "no ASIN in folder",
			bookDir:    "/library/Author/Just A Title",
			wantTitle:  "Just A Title",
			wantAuthor: "Author",
		},
		{
			name:       "ASIN only folder name",
			bookDir:    "/library/Author/B08C6YJ1LS",
			wantTitle:  "B08C6YJ1LS", // falls back to original when stripped is empty
			wantAuthor: "Author",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := extractFromFolderName(tt.bookDir)
			assert.Equal(t, tt.wantTitle, meta.Title)
			assert.Equal(t, tt.wantAuthor, meta.Author)
			assert.Equal(t, SourceFolder, meta.Source)
		})
	}
}

func TestExtractMetadataNoM4A(t *testing.T) {
	// Create a temp directory structure: Author/Title [ASIN]/
	root := t.TempDir()
	bookDir := filepath.Join(root, "Andy Weir", "Project Hail Mary [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	meta, err := ExtractMetadata(bookDir)
	require.NoError(t, err)
	assert.Equal(t, SourceFolder, meta.Source)
	assert.Equal(t, "Project Hail Mary", meta.Title)
	assert.Equal(t, "Andy Weir", meta.Author)
	assert.Equal(t, 0, meta.FileCount)
}

func TestExtractMetadataInvalidM4A(t *testing.T) {
	// Create a directory with an invalid M4A file (not a real MP4 container)
	root := t.TempDir()
	bookDir := filepath.Join(root, "Author Name", "Book Title [B08C6YJ1LS]")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), []byte("not a real m4a"), 0644))

	meta, err := ExtractMetadata(bookDir)
	require.NoError(t, err)

	// tag.ReadFrom will fail on invalid file, should fall through to ffprobe or folder
	// Either way, we get metadata back (not an error)
	assert.NotNil(t, meta)
	assert.Equal(t, 1, meta.FileCount)
	// Source will be either ffprobe or folder depending on availability
	assert.Contains(t, []MetadataSource{SourceFFprobe, SourceFolder}, meta.Source)
}

func TestExtractWithFFprobeNotAvailable(t *testing.T) {
	// Save current PATH and set to empty
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	meta, err := extractWithFFprobe("/nonexistent/file.m4a")
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "ffprobe not found")
}

func TestNormalizeTagKeys(t *testing.T) {
	tags := map[string]string{
		"Title":       "My Book",
		"ALBUM_ARTIST": "Author Name",
		"genre":       "Fiction",
	}

	normalized := normalizeTagKeys(tags)
	assert.Equal(t, "My Book", normalized["title"])
	assert.Equal(t, "Author Name", normalized["album_artist"])
	assert.Equal(t, "Fiction", normalized["genre"])
}
