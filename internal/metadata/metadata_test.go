package metadata

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindAudioFiles(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "book.m4a"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "UPPER.M4A"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "audiobook.m4b"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("fake"), 0644))

	files := FindAudioFiles(dir)
	assert.Len(t, files, 3)

	// Verify paths are absolute and end with audio extensions
	for _, f := range files {
		assert.True(t, filepath.IsAbs(f))
		ext := strings.ToLower(filepath.Ext(f))
		assert.True(t, ext == ".m4a" || ext == ".m4b", "unexpected extension: %s", ext)
	}
}

func TestFindAudioFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	files := FindAudioFiles(dir)
	assert.Empty(t, files)
}

func TestFindAudioFilesNonexistent(t *testing.T) {
	files := FindAudioFiles("/nonexistent/path")
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
	origLookPath := lookPathFn
	lookPathFn = func(file string) (string, error) {
		return "", fmt.Errorf("exec: %q: executable file not found in $PATH", file)
	}
	defer func() { lookPathFn = origLookPath }()

	meta, err := extractWithFFprobe("/nonexistent/file.m4a")
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "ffprobe not found")
}

func TestNormalizeTagKeys(t *testing.T) {
	tags := map[string]string{
		"Title":        "My Book",
		"ALBUM_ARTIST": "Author Name",
		"genre":        "Fiction",
	}

	normalized := normalizeTagKeys(tags)
	assert.Equal(t, "My Book", normalized["title"])
	assert.Equal(t, "Author Name", normalized["album_artist"])
	assert.Equal(t, "Fiction", normalized["genre"])
}

// TestHelperProcess is used by fakeExecCommand to simulate subprocesses.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("GO_HELPER_OUTPUT"))
	code, _ := strconv.Atoi(os.Getenv("GO_HELPER_EXIT_CODE"))
	os.Exit(code)
}

func fakeExecCommand(output string, exitCode int) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			fmt.Sprintf("GO_HELPER_OUTPUT=%s", output),
			fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", exitCode),
		)
		return cmd
	}
}

func TestExtractWithFFprobe_Success(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffprobe", nil
	}

	ffprobeJSON := `{"format":{"duration":"36000.5","tags":{"title":"Test Book","album_artist":"Test Author","artist":"Test Narrator","genre":"Fiction","date":"2024-01-15"}},"chapters":[{"id":0},{"id":1},{"id":2}]}`
	execCommandCtx = fakeExecCommand(ffprobeJSON, 0)

	meta, err := extractWithFFprobe("/fake/book.m4a")
	require.NoError(t, err)
	assert.Equal(t, "Test Book", meta.Title)
	assert.Equal(t, "Test Author", meta.Author)
	assert.Equal(t, "Test Narrator", meta.Narrator)
	assert.Equal(t, "Fiction", meta.Genre)
	assert.Equal(t, 2024, meta.Year)
	assert.Equal(t, 36001, meta.Duration)
	assert.Equal(t, 3, meta.ChapterCount)
	assert.Equal(t, SourceFFprobe, meta.Source)
}

func TestExtractWithFFprobe_CommandFails(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffprobe", nil
	}
	execCommandCtx = fakeExecCommand("", 1)

	meta, err := extractWithFFprobe("/fake/book.m4a")
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "ffprobe failed")
}

func TestExtractWithFFprobe_InvalidJSON(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffprobe", nil
	}
	execCommandCtx = fakeExecCommand("not json at all", 0)

	meta, err := extractWithFFprobe("/fake/book.m4a")
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "parse ffprobe output")
}

func TestExtractWithTag_OpenError(t *testing.T) {
	meta, err := extractWithTag("/nonexistent/file.m4a")
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "open file")
}

func TestExtractMetadata_TagFailsFfprobeFallback(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	// Create a temp dir with fake .m4a file (invalid content so tag fails)
	root := t.TempDir()
	bookDir := filepath.Join(root, "Author", "Book Title")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), []byte("not real m4a"), 0644))

	// Override ffprobe to return valid metadata
	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffprobe", nil
	}
	ffprobeJSON := `{"format":{"duration":"3600","tags":{"title":"FFprobe Title","artist":"FFprobe Artist"}},"chapters":[]}`
	execCommandCtx = fakeExecCommand(ffprobeJSON, 0)

	meta, err := ExtractMetadata(bookDir)
	require.NoError(t, err)
	assert.Equal(t, SourceFFprobe, meta.Source)
	assert.Equal(t, "FFprobe Title", meta.Title)
	assert.Equal(t, 1, meta.FileCount)
}

func TestExtractWithFFprobe_NoDuration(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffprobe", nil
	}
	ffprobeJSON := `{"format":{"tags":{"title":"No Duration Book"}},"chapters":[]}`
	execCommandCtx = fakeExecCommand(ffprobeJSON, 0)

	meta, err := extractWithFFprobe("/fake/book.m4a")
	require.NoError(t, err)
	assert.Equal(t, 0, meta.Duration)
	assert.Equal(t, "No Duration Book", meta.Title)
}

// buildMinimalMP4 creates a minimal MP4 file with metadata atoms that dhowden/tag can parse.
// It constructs: ftyp box + moov box containing udta/meta/ilst atoms with title, artist, etc.
func buildMinimalMP4(title, artist, albumArtist, genre string, year int) []byte {
	// Helper to write a big-endian uint32
	be32 := func(v uint32) []byte {
		return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
	// Helper to build a box: [size][type][payload]
	box := func(typ string, payload []byte) []byte {
		size := uint32(8 + len(payload))
		b := be32(size)
		b = append(b, []byte(typ)...)
		b = append(b, payload...)
		return b
	}
	// Helper to build an ilst data atom: [size][type][data box]
	dataAtom := func(atomType string, text string) []byte {
		// data box: [size]["data"][flags 4 bytes][locale 4 bytes][text]
		dataPayload := make([]byte, 8) // 4 flags + 4 locale (all zero = UTF-8 text)
		dataPayload[3] = 1             // data type 1 = UTF-8 text
		dataPayload = append(dataPayload, []byte(text)...)
		dataBox := box("data", dataPayload)
		return box(atomType, dataBox)
	}

	// ftyp box
	ftypPayload := []byte("M4A \x00\x00\x00\x00M4A mp42isom")
	ftyp := box("ftyp", ftypPayload)

	// Build ilst (item list) with metadata
	var ilst []byte
	if title != "" {
		ilst = append(ilst, dataAtom("\xa9nam", title)...)
	}
	if artist != "" {
		ilst = append(ilst, dataAtom("\xa9ART", artist)...)
	}
	if albumArtist != "" {
		ilst = append(ilst, dataAtom("aART", albumArtist)...)
	}
	if genre != "" {
		ilst = append(ilst, dataAtom("\xa9gen", genre)...)
	}
	if year > 0 {
		ilst = append(ilst, dataAtom("\xa9day", fmt.Sprintf("%d", year))...)
	}

	ilstBox := box("ilst", ilst)

	// meta box has a 4-byte version/flags field after the type
	metaPayload := make([]byte, 4) // version + flags
	// hdlr box for meta
	hdlrPayload := make([]byte, 4) // version + flags
	hdlrPayload = append(hdlrPayload, []byte("\x00\x00\x00\x00")...) // pre-defined
	hdlrPayload = append(hdlrPayload, []byte("mdir")...)              // handler type
	hdlrPayload = append(hdlrPayload, make([]byte, 12)...)            // reserved
	hdlrPayload = append(hdlrPayload, []byte("appl\x00")...)          // name
	hdlrBox := box("hdlr", hdlrPayload)
	metaPayload = append(metaPayload, hdlrBox...)
	metaPayload = append(metaPayload, ilstBox...)
	metaBox := box("meta", metaPayload)

	udtaBox := box("udta", metaBox)
	moovBox := box("moov", udtaBox)

	var mp4 []byte
	mp4 = append(mp4, ftyp...)
	mp4 = append(mp4, moovBox...)
	return mp4
}

func TestExtractWithTag_Success(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "book.m4a")
	mp4Data := buildMinimalMP4("My Great Book", "Narrator Name", "Author Name", "Fiction", 2023)
	require.NoError(t, os.WriteFile(f, mp4Data, 0644))

	meta, err := extractWithTag(f)
	require.NoError(t, err)
	assert.Equal(t, SourceTag, meta.Source)
	assert.Equal(t, "My Great Book", meta.Title)
	assert.Equal(t, "Author Name", meta.Author) // AlbumArtist preferred over Artist
	assert.Equal(t, "Fiction", meta.Genre)
	assert.Equal(t, 2023, meta.Year)
}

func TestExtractWithTag_ArtistFallbackNoAlbumArtist(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "book.m4a")
	// No album artist, should fall back to artist
	mp4Data := buildMinimalMP4("Title Only", "The Artist", "", "Drama", 2020)
	require.NoError(t, os.WriteFile(f, mp4Data, 0644))

	meta, err := extractWithTag(f)
	require.NoError(t, err)
	assert.Equal(t, "The Artist", meta.Author)
}

func TestExtractMetadata_TagSucceeds(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "Author", "Title")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	mp4Data := buildMinimalMP4("Metadata Title", "Narrator", "Author", "Sci-Fi", 2024)
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), mp4Data, 0644))

	meta, err := ExtractMetadata(bookDir)
	require.NoError(t, err)
	assert.Equal(t, SourceTag, meta.Source)
	assert.Equal(t, "Metadata Title", meta.Title)
	assert.Equal(t, 1, meta.FileCount)
}

func TestExtractWithTag_InvalidM4AContent(t *testing.T) {
	// Create a temp file with content that is a valid file but not valid MP4
	tmp := t.TempDir()
	f := filepath.Join(tmp, "book.m4a")
	require.NoError(t, os.WriteFile(f, []byte("definitely not an mp4 container"), 0644))

	meta, err := extractWithTag(f)
	assert.Error(t, err)
	assert.Nil(t, meta)
	assert.Contains(t, err.Error(), "read tags from")
}

func TestExtractMetadata_AllFallbacksToFolder(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	// Make ffprobe fail too
	lookPathFn = func(file string) (string, error) {
		return "", fmt.Errorf("not found")
	}

	root := t.TempDir()
	bookDir := filepath.Join(root, "Test Author", "Test Title")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.m4a"), []byte("not real"), 0644))

	meta, err := ExtractMetadata(bookDir)
	require.NoError(t, err)
	assert.Equal(t, SourceFolder, meta.Source)
	assert.Equal(t, "Test Title", meta.Title)
	assert.Equal(t, "Test Author", meta.Author)
	assert.Equal(t, 1, meta.FileCount)
}

func TestExtractFileMetadata_TagSuccess(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "book.m4a")
	mp4Data := buildMinimalMP4("File Title", "Narrator", "File Author", "Fiction", 2023)
	require.NoError(t, os.WriteFile(f, mp4Data, 0644))

	meta, err := ExtractFileMetadata(f)
	require.NoError(t, err)
	assert.Equal(t, "File Title", meta.Title)
	assert.Equal(t, "File Author", meta.Author)
	assert.Equal(t, SourceTag, meta.Source)
	assert.Equal(t, 1, meta.FileCount)
}

func TestExtractFileMetadata_FfprobeFallback(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	tmp := t.TempDir()
	f := filepath.Join(tmp, "book.m4a")
	// Write invalid M4A so tag fails
	require.NoError(t, os.WriteFile(f, []byte("not real m4a"), 0644))

	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffprobe", nil
	}
	ffprobeJSON := `{"format":{"duration":"3600","tags":{"title":"FFprobe File Title","artist":"FFprobe Artist"}},"chapters":[]}`
	execCommandCtx = fakeExecCommand(ffprobeJSON, 0)

	meta, err := ExtractFileMetadata(f)
	require.NoError(t, err)
	assert.Equal(t, "FFprobe File Title", meta.Title)
	assert.Equal(t, SourceFFprobe, meta.Source)
	assert.Equal(t, 1, meta.FileCount)
}

func TestExtractFileMetadata_NonexistentFile(t *testing.T) {
	_, err := ExtractFileMetadata("/nonexistent/file.m4a")
	assert.Error(t, err)
}

func TestExtractFileMetadata_NoFolderFallback(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	// Make ffprobe fail too
	lookPathFn = func(file string) (string, error) {
		return "", fmt.Errorf("not found")
	}

	tmp := t.TempDir()
	f := filepath.Join(tmp, "book.m4a")
	require.NoError(t, os.WriteFile(f, []byte("not real"), 0644))

	// Both tag and ffprobe fail — should return error, NOT folder fallback
	meta, err := ExtractFileMetadata(f)
	assert.Error(t, err)
	assert.Nil(t, meta)
}

func TestExtractWithFFprobe_ArtistFallback(t *testing.T) {
	origLookPath := lookPathFn
	origExecCmd := execCommandCtx
	defer func() {
		lookPathFn = origLookPath
		execCommandCtx = origExecCmd
	}()

	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffprobe", nil
	}
	// No album_artist, should fall back to artist for Author
	ffprobeJSON := `{"format":{"tags":{"title":"Book","artist":"Artist Only"}},"chapters":[]}`
	execCommandCtx = fakeExecCommand(ffprobeJSON, 0)

	meta, err := extractWithFFprobe("/fake/book.m4a")
	require.NoError(t, err)
	assert.Equal(t, "Artist Only", meta.Author)
}
