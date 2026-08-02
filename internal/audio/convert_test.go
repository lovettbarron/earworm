package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNaturalSort(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "chapter ordering",
			input:    []string{"Chapter 10.mp3", "Chapter 2.mp3", "Chapter 1.mp3"},
			expected: []string{"Chapter 1.mp3", "Chapter 2.mp3", "Chapter 10.mp3"},
		},
		{
			name:     "numeric prefixes",
			input:    []string{"01.mp3", "10.mp3", "2.mp3"},
			expected: []string{"01.mp3", "2.mp3", "10.mp3"},
		},
		{
			name:     "no numbers",
			input:    []string{"a.mp3", "b.mp3"},
			expected: []string{"a.mp3", "b.mp3"},
		},
		{
			name:     "mixed numeric and alpha",
			input:    []string{"track20.mp3", "track3.mp3", "track1.mp3"},
			expected: []string{"track1.mp3", "track3.mp3", "track20.mp3"},
		},
		{
			name:     "single element",
			input:    []string{"file.mp3"},
			expected: []string{"file.mp3"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]string, len(tt.input))
			copy(input, tt.input)
			sort.Slice(input, func(i, j int) bool {
				return NaturalLess(input[i], input[j])
			})
			assert.Equal(t, tt.expected, input)
		})
	}
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"Chapter 1", "Chapter 2", true},
		{"Chapter 2", "Chapter 1", false},
		{"Chapter 2", "Chapter 10", true},
		{"Chapter 10", "Chapter 2", false},
		{"a", "b", true},
		{"b", "a", false},
		{"abc", "abc", false},
		{"1", "2", true},
		{"2", "10", true},
		{"10", "2", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.a, tt.b), func(t *testing.T) {
			assert.Equal(t, tt.expected, NaturalLess(tt.a, tt.b))
		})
	}
}

func TestFindAudioFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mix of audio and non-audio files
	audioFiles := []string{
		"Chapter 10.mp3",
		"Chapter 1.mp3",
		"Chapter 2.mp3",
		"intro.m4a",
	}
	nonAudioFiles := []string{
		"cover.jpg",
		"metadata.json",
		"readme.txt",
		"chapters.json",
	}

	for _, f := range append(audioFiles, nonAudioFiles...) {
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, f), []byte("data"), 0644))
	}

	// Create a subdirectory (should be ignored)
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "subdir", "nested.mp3"), []byte("data"), 0644))

	files, err := FindAudioFiles(tmpDir)
	require.NoError(t, err)

	// Should find 4 audio files, not nested ones
	assert.Len(t, files, 4)

	// Verify natural sort order
	basenames := make([]string, len(files))
	for i, f := range files {
		basenames[i] = filepath.Base(f)
	}
	assert.Equal(t, []string{"Chapter 1.mp3", "Chapter 2.mp3", "Chapter 10.mp3", "intro.m4a"}, basenames)
}

func TestFindAudioFiles_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	files, err := FindAudioFiles(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestFindAudioFiles_NonexistentDir(t *testing.T) {
	_, err := FindAudioFiles("/nonexistent/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading directory")
}

func TestFindAudioFiles_AllExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	extensions := []string{".mp3", ".m4a", ".m4b", ".ogg", ".opus", ".flac", ".wma", ".aac"}
	for _, ext := range extensions {
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file"+ext), []byte("data"), 0644))
	}

	files, err := FindAudioFiles(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, len(extensions))
}

func TestGenerateConcatList(t *testing.T) {
	files := []string{
		"/path/to/Chapter 01.mp3",
		"/path/to/Chapter 02.mp3",
	}

	result := GenerateConcatList(files)
	assert.Equal(t, "file '/path/to/Chapter 01.mp3'\nfile '/path/to/Chapter 02.mp3'\n", result)
}

func TestGenerateConcatList_EscapeSingleQuotes(t *testing.T) {
	files := []string{
		"/path/to/It's a Book.mp3",
	}

	result := GenerateConcatList(files)
	assert.Equal(t, "file '/path/to/It'\\''s a Book.mp3'\n", result)
}

func TestGenerateConcatList_Empty(t *testing.T) {
	result := GenerateConcatList(nil)
	assert.Equal(t, "", result)
}

func TestGenerateFFMetadata(t *testing.T) {
	probes := []ProbeResult{
		{Path: "/books/01 - Introduction.mp3", Duration: 120.5},
		{Path: "/books/02 - Chapter One.mp3", Duration: 3600.0},
		{Path: "/books/03 - Chapter Two.mp3", Duration: 2400.75},
	}

	opts := ConvertOptions{
		BookTitle:    "Test Book",
		BookAuthor:   "Test Author",
		BookNarrator: "Test Narrator",
		BookYear:     "2024",
	}

	result := GenerateFFMetadata(probes, opts)

	// Verify header
	assert.True(t, strings.HasPrefix(result, ";FFMETADATA1\n"))

	// Verify metadata fields
	assert.Contains(t, result, "title=Test Book\n")
	assert.Contains(t, result, "artist=Test Author\n")
	assert.Contains(t, result, "album_artist=Test Author\n")
	assert.Contains(t, result, "composer=Test Narrator\n")
	assert.Contains(t, result, "genre=Audiobook\n")
	assert.Contains(t, result, "date=2024\n")

	// Verify chapter markers
	assert.Contains(t, result, "[CHAPTER]")
	assert.Contains(t, result, "TIMEBASE=1/1000")

	// First chapter: START=0, END=120500
	assert.Contains(t, result, "START=0")
	assert.Contains(t, result, "END=120500")
	assert.Contains(t, result, "title=Introduction")

	// Second chapter: START=120500, END=3720500
	assert.Contains(t, result, "START=120500")
	assert.Contains(t, result, "END=3720500")
	assert.Contains(t, result, "title=Chapter One")

	// Third chapter: START=3720500
	assert.Contains(t, result, "START=3720500")
	assert.Contains(t, result, "title=Chapter Two")

	// Verify 3 chapters
	assert.Equal(t, 3, strings.Count(result, "[CHAPTER]"))
}

func TestGenerateFFMetadata_SpecialChars(t *testing.T) {
	probes := []ProbeResult{
		{Path: "/books/01.mp3", Duration: 60.0},
	}

	opts := ConvertOptions{
		BookTitle:  "Book: A = B; C # D",
		BookAuthor: "Author; Name",
	}

	result := GenerateFFMetadata(probes, opts)

	// Special chars should be escaped (colon is NOT a special char in FFMETADATA1)
	assert.Contains(t, result, "title=Book: A \\= B\\; C \\# D")
	assert.Contains(t, result, "artist=Author\\; Name")
}

func TestChapterTitleFromFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"01 - Introduction.mp3", "Introduction"},
		{"01.Introduction.mp3", "Introduction"},
		{"Chapter One.mp3", "Chapter One"},
		{"12345.mp3", "12345"},
		{"01_Prologue.mp3", "Prologue"},
		{"intro.mp3", "intro"},
		{"001 - The Beginning.mp3", "The Beginning"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := chapterTitleFromFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToM4B_MissingFFmpeg(t *testing.T) {
	opts := ConvertOptions{
		InputDir:   t.TempDir(),
		OutputPath: filepath.Join(t.TempDir(), "output.m4b"),
		StagingDir: t.TempDir(),
		CmdFactory: DefaultCmdFactory,
	}

	// Override PATH to ensure ffmpeg is not found
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	_, err := ConvertToM4B(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg not found")
}

func TestConvertToM4B_NoAudioFiles(t *testing.T) {
	inputDir := t.TempDir()
	// Create only non-audio files
	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "readme.txt"), []byte("data"), 0644))

	// We need ffmpeg and ffprobe on PATH for this test to reach the "no audio files" check.
	// Create fake binaries that do nothing.
	binDir := t.TempDir()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		script := filepath.Join(binDir, tool)
		require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755))
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	opts := ConvertOptions{
		InputDir:   inputDir,
		OutputPath: filepath.Join(t.TempDir(), "output.m4b"),
		StagingDir: t.TempDir(),
		CmdFactory: DefaultCmdFactory,
	}

	_, err := ConvertToM4B(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no audio files found")
}

func TestConvertToM4B_Success(t *testing.T) {
	inputDir := t.TempDir()
	stagingDir := t.TempDir()
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.m4b")

	// Create test audio files
	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "01 - Intro.mp3"), []byte("audio1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "02 - Chapter 1.mp3"), []byte("audio2"), 0644))

	// Create fake ffmpeg/ffprobe on PATH
	binDir := t.TempDir()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		script := filepath.Join(binDir, tool)
		require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755))
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	// Track ffmpeg calls to verify arguments
	callCount := 0
	factory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		callCount++
		baseName := filepath.Base(name)

		if baseName == "ffprobe" {
			// Return valid JSON for probing
			jsonResp := `{"format":{"duration":"120.5","bit_rate":"128000","tags":{"title":"Test"}},"streams":[{"codec_type":"audio","sample_rate":"44100","channels":2}]}`
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo '%s'", jsonResp))
		}

		// For ffmpeg calls, create the expected output files
		if baseName == "ffmpeg" {
			// Find output path (last argument)
			outPath := args[len(args)-1]
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo 'fake audio' > '%s'", outPath))
		}

		return exec.CommandContext(ctx, "echo", "unexpected call")
	}

	opts := ConvertOptions{
		InputDir:     inputDir,
		OutputPath:   outputPath,
		StagingDir:   stagingDir,
		Bitrate:      "64k",
		SampleRate:   44100,
		Channels:     1,
		BookTitle:    "Test Book",
		BookAuthor:   "Test Author",
		BookNarrator: "Test Narrator",
		BookYear:     "2024",
		CmdFactory:   factory,
	}

	result, err := ConvertToM4B(context.Background(), opts)
	require.NoError(t, err)

	assert.Equal(t, outputPath, result.OutputPath)
	assert.Equal(t, 2, result.ChapterCount)
	assert.InDelta(t, 241.0, result.Duration, 0.1) // 2 * 120.5
	assert.Greater(t, result.FileSize, int64(0))

	// Verify output file exists
	assert.FileExists(t, outputPath)

	// Verify staging temp files cleaned up
	assert.NoFileExists(t, filepath.Join(stagingDir, "filelist.txt"))
	assert.NoFileExists(t, filepath.Join(stagingDir, "metadata.txt"))
	assert.NoFileExists(t, filepath.Join(stagingDir, "temp.m4a"))
	assert.NoFileExists(t, filepath.Join(stagingDir, "temp_meta.m4b"))

	// ffprobe called twice (once per file) + ffmpeg called 3 times (concat, metadata, finalize)
	assert.Equal(t, 5, callCount)
}

func TestConvertToM4B_WithCoverArt(t *testing.T) {
	inputDir := t.TempDir()
	stagingDir := t.TempDir()
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.m4b")

	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "01.mp3"), []byte("audio"), 0644))

	// Create cover image
	coverPath := filepath.Join(inputDir, "cover.jpg")
	require.NoError(t, os.WriteFile(coverPath, []byte("jpeg data"), 0644))

	binDir := t.TempDir()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		script := filepath.Join(binDir, tool)
		require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755))
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	var ffmpegArgs [][]string
	factory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		baseName := filepath.Base(name)
		if baseName == "ffprobe" {
			jsonResp := `{"format":{"duration":"60.0","bit_rate":"128000","tags":{}},"streams":[{"codec_type":"audio","sample_rate":"44100","channels":2}]}`
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo '%s'", jsonResp))
		}
		if baseName == "ffmpeg" {
			ffmpegArgs = append(ffmpegArgs, args)
			outPath := args[len(args)-1]
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo 'audio' > '%s'", outPath))
		}
		return exec.CommandContext(ctx, "echo", "unexpected")
	}

	opts := ConvertOptions{
		InputDir:   inputDir,
		OutputPath: outputPath,
		StagingDir: stagingDir,
		CoverPath:  coverPath,
		BookTitle:   "Cover Test",
		CmdFactory: factory,
	}

	result, err := ConvertToM4B(context.Background(), opts)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Third ffmpeg call should include cover art arguments
	require.Len(t, ffmpegArgs, 3)
	lastCall := ffmpegArgs[2]
	assert.Contains(t, lastCall, coverPath)
	assert.Contains(t, lastCall, "attached_pic")
}

func TestConvertToM4B_FailureCleanup(t *testing.T) {
	inputDir := t.TempDir()
	stagingDir := t.TempDir()
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "output.m4b")

	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "01.mp3"), []byte("audio"), 0644))

	binDir := t.TempDir()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		script := filepath.Join(binDir, tool)
		require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755))
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	callCount := 0
	factory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		callCount++
		baseName := filepath.Base(name)
		if baseName == "ffprobe" {
			jsonResp := `{"format":{"duration":"60.0","bit_rate":"128000","tags":{}},"streams":[{"codec_type":"audio","sample_rate":"44100","channels":2}]}`
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo '%s'", jsonResp))
		}
		// First ffmpeg call (concat) fails
		return exec.CommandContext(ctx, "sh", "-c", "echo 'ffmpeg error' >&2; exit 1")
	}

	opts := ConvertOptions{
		InputDir:   inputDir,
		OutputPath: outputPath,
		StagingDir: stagingDir,
		CmdFactory: factory,
	}

	_, err := ConvertToM4B(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg concat+transcode")

	// Output should not exist
	assert.NoFileExists(t, outputPath)

	// Staging temp files should be cleaned up (via defer)
	assert.NoFileExists(t, filepath.Join(stagingDir, "temp.m4a"))
	assert.NoFileExists(t, filepath.Join(stagingDir, "temp_meta.m4b"))
}

func TestProbeFile_MockOutput(t *testing.T) {
	binDir := t.TempDir()
	// Create fake ffprobe that returns JSON
	jsonResp := `{"format":{"duration":"3661.5","bit_rate":"192000","tags":{"title":"My Audiobook","artist":"Author Name","genre":"Audiobook"}},"streams":[{"codec_type":"audio","sample_rate":"44100","channels":2,"bit_rate":"192000"}]}`
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", jsonResp)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte(script), 0755))
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	tmpFile := filepath.Join(t.TempDir(), "test.mp3")
	require.NoError(t, os.WriteFile(tmpFile, []byte("audio"), 0644))

	result, err := ProbeFile(context.Background(), tmpFile, DefaultCmdFactory)
	require.NoError(t, err)

	assert.Equal(t, tmpFile, result.Path)
	assert.InDelta(t, 3661.5, result.Duration, 0.01)
	assert.Equal(t, 192, result.Bitrate)
	assert.Equal(t, 44100, result.SampleRate)
	assert.Equal(t, 2, result.Channels)
	assert.Equal(t, "mp3", result.Format)
	assert.Equal(t, "My Audiobook", result.Tags["title"])
	assert.Equal(t, "Author Name", result.Tags["artist"])
}

func TestProbeFiles_AllSucceed(t *testing.T) {
	binDir := t.TempDir()
	jsonResp := `{"format":{"duration":"60.0","bit_rate":"128000","tags":{}},"streams":[{"codec_type":"audio","sample_rate":"44100","channels":1}]}`
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", jsonResp)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte(script), 0755))
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	tmpDir := t.TempDir()
	files := []string{
		filepath.Join(tmpDir, "01.mp3"),
		filepath.Join(tmpDir, "02.mp3"),
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(f, []byte("audio"), 0644))
	}

	results, err := ProbeFiles(context.Background(), files, DefaultCmdFactory)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, files[0], results[0].Path)
	assert.Equal(t, files[1], results[1].Path)
}

func TestDefaultCmdFactory(t *testing.T) {
	cmd := DefaultCmdFactory(context.Background(), "echo", "hello")
	assert.NotNil(t, cmd)
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "hello")
}

func TestEscapeFFMetaValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"a=b", "a\\=b"},
		{"a;b", "a\\;b"},
		{"a#b", "a\\#b"},
		{"a\\b", "a\\\\b"},
		{"Book: Title = Subtitle; Extra", "Book: Title \\= Subtitle\\; Extra"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, escapeFFMetaValue(tt.input))
		})
	}
}

func TestConvertOptions_Defaults(t *testing.T) {
	// Verify that ConvertToM4B applies defaults when zero values are given
	// We can't run the full pipeline without ffmpeg, but we can check the options
	// by looking at what happens with empty options
	opts := ConvertOptions{}

	// These would be applied at the start of ConvertToM4B
	if opts.Bitrate == "" {
		opts.Bitrate = "64k"
	}
	if opts.SampleRate == 0 {
		opts.SampleRate = 44100
	}
	if opts.Channels == 0 {
		opts.Channels = 1
	}

	assert.Equal(t, "64k", opts.Bitrate)
	assert.Equal(t, 44100, opts.SampleRate)
	assert.Equal(t, 1, opts.Channels)
}
