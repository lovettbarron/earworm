package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// CmdFactory creates exec.Cmd instances. Allows test injection.
type CmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

// DefaultCmdFactory uses os/exec to create commands.
func DefaultCmdFactory(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// ProbeResult holds ffprobe output for a single audio file.
type ProbeResult struct {
	Path       string
	Duration   float64 // seconds
	SampleRate int
	Channels   int
	Bitrate    int // kbps
	Format     string
	Tags       map[string]string
}

// ConvertOptions configures the conversion.
type ConvertOptions struct {
	InputDir     string   // directory containing source audio files
	OutputPath   string   // full path for output .m4b file
	StagingDir   string   // temp working directory
	Bitrate      string   // e.g. "64k" (default)
	SampleRate   int      // e.g. 44100 (default)
	Channels     int      // e.g. 1 for mono (default)
	BookTitle    string
	BookAuthor   string
	BookNarrator string
	BookYear     string
	CoverPath    string // path to cover image, empty if none
	CmdFactory   CmdFactory
}

// ConvertResult holds the outcome.
type ConvertResult struct {
	OutputPath   string
	Duration     float64 // total duration in seconds
	ChapterCount int
	FileSize     int64
}

// audioExtensions lists recognized audio file extensions.
var audioExtensions = map[string]bool{
	".mp3":  true,
	".m4a":  true,
	".m4b":  true,
	".ogg":  true,
	".opus": true,
	".flac": true,
	".wma":  true,
	".aac":  true,
}

// naturalChunkRe splits a string into alternating text and numeric chunks.
var naturalChunkRe = regexp.MustCompile(`(\d+|\D+)`)

// NaturalLess returns true if a should sort before b using natural sort order.
// Numeric substrings are compared as integers, so "Chapter 2" < "Chapter 10".
func NaturalLess(a, b string) bool {
	chunksA := naturalChunkRe.FindAllString(strings.ToLower(a), -1)
	chunksB := naturalChunkRe.FindAllString(strings.ToLower(b), -1)

	for i := 0; i < len(chunksA) && i < len(chunksB); i++ {
		ca, cb := chunksA[i], chunksB[i]
		if ca == cb {
			continue
		}

		// Try to compare as numbers
		na, errA := strconv.Atoi(ca)
		nb, errB := strconv.Atoi(cb)
		if errA == nil && errB == nil {
			if na != nb {
				return na < nb
			}
			continue
		}

		// Fall back to string comparison
		return ca < cb
	}

	return len(chunksA) < len(chunksB)
}

// FindAudioFiles finds all audio files in a directory, sorted with natural sort order.
// It does not recurse into subdirectories.
func FindAudioFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %q: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if audioExtensions[ext] {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return NaturalLess(filepath.Base(files[i]), filepath.Base(files[j]))
	})

	return files, nil
}

// ffprobeStreamOutput represents the JSON output from ffprobe.
type ffprobeStreamOutput struct {
	Format  ffprobeFormatInfo  `json:"format"`
	Streams []ffprobeStreamDet `json:"streams"`
}

type ffprobeFormatInfo struct {
	Duration string            `json:"duration"`
	BitRate  string            `json:"bit_rate"`
	Tags     map[string]string `json:"tags"`
}

type ffprobeStreamDet struct {
	CodecType  string `json:"codec_type"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
	BitRate    string `json:"bit_rate"`
}

// ProbeFile calls ffprobe on a single file and returns its metadata.
func ProbeFile(ctx context.Context, path string, cmdFactory CmdFactory) (*ProbeResult, error) {
	if cmdFactory == nil {
		cmdFactory = DefaultCmdFactory
	}

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe not found: %w", err)
	}

	cmd := cmdFactory(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed on %q: %w", path, err)
	}

	var probe ffprobeStreamOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("parse ffprobe output for %q: %w", path, err)
	}

	result := &ProbeResult{
		Path: path,
		Tags: probe.Format.Tags,
	}

	// Parse duration
	if probe.Format.Duration != "" {
		if dur, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			result.Duration = dur
		}
	}

	// Parse format bitrate (kbps)
	if probe.Format.BitRate != "" {
		if br, err := strconv.Atoi(probe.Format.BitRate); err == nil {
			result.Bitrate = br / 1000
		}
	}

	// Extract format from extension
	result.Format = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")

	// Find audio stream for sample rate and channels
	for _, s := range probe.Streams {
		if s.CodecType == "audio" {
			if s.SampleRate != "" {
				if sr, err := strconv.Atoi(s.SampleRate); err == nil {
					result.SampleRate = sr
				}
			}
			result.Channels = s.Channels
			if result.Bitrate == 0 && s.BitRate != "" {
				if br, err := strconv.Atoi(s.BitRate); err == nil {
					result.Bitrate = br / 1000
				}
			}
			break
		}
	}

	return result, nil
}

// ProbeFiles probes all files and returns results in the same order.
// Returns an error if any file fails to probe.
func ProbeFiles(ctx context.Context, files []string, cmdFactory CmdFactory) ([]ProbeResult, error) {
	results := make([]ProbeResult, 0, len(files))
	for _, f := range files {
		r, err := ProbeFile(ctx, f, cmdFactory)
		if err != nil {
			return nil, fmt.Errorf("probing %q: %w", f, err)
		}
		results = append(results, *r)
	}
	return results, nil
}

// GenerateConcatList generates ffmpeg concat demuxer format content.
// Paths are properly escaped for the concat format.
func GenerateConcatList(files []string) string {
	var sb strings.Builder
	for _, f := range files {
		// Escape single quotes in filenames by replacing ' with '\''
		escaped := strings.ReplaceAll(f, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("file '%s'\n", escaped))
	}
	return sb.String()
}

// chapterTitleFromFilename extracts a chapter title from a filename.
// Strips extension, then strips leading numbers, dots, dashes, and spaces.
func chapterTitleFromFilename(name string) string {
	// Remove extension
	name = strings.TrimSuffix(name, filepath.Ext(name))

	// Strip leading numbers, dots, dashes, underscores, and spaces
	trimmed := strings.TrimLeftFunc(name, func(r rune) bool {
		return unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' || r == ' '
	})

	if trimmed == "" {
		return name // fallback to original (minus extension) if all stripped
	}
	return trimmed
}

// GenerateFFMetadata generates FFMETADATA1 content with book metadata and
// chapter markers. Each input file becomes a chapter, with timing derived from
// file durations. Timestamps use TIMEBASE=1/1000 (milliseconds).
func GenerateFFMetadata(probes []ProbeResult, opts ConvertOptions) string {
	var sb strings.Builder
	sb.WriteString(";FFMETADATA1\n")

	if opts.BookTitle != "" {
		sb.WriteString(fmt.Sprintf("title=%s\n", escapeFFMetaValue(opts.BookTitle)))
	}
	if opts.BookAuthor != "" {
		sb.WriteString(fmt.Sprintf("artist=%s\n", escapeFFMetaValue(opts.BookAuthor)))
		sb.WriteString(fmt.Sprintf("album_artist=%s\n", escapeFFMetaValue(opts.BookAuthor)))
		sb.WriteString(fmt.Sprintf("album=%s\n", escapeFFMetaValue(opts.BookTitle)))
	}
	if opts.BookNarrator != "" {
		sb.WriteString(fmt.Sprintf("composer=%s\n", escapeFFMetaValue(opts.BookNarrator)))
	}
	sb.WriteString("genre=Audiobook\n")
	if opts.BookYear != "" {
		sb.WriteString(fmt.Sprintf("date=%s\n", escapeFFMetaValue(opts.BookYear)))
	}

	// Generate chapter markers
	var startMs int64
	for _, p := range probes {
		durationMs := int64(math.Round(p.Duration * 1000))
		endMs := startMs + durationMs

		chapterTitle := chapterTitleFromFilename(filepath.Base(p.Path))

		sb.WriteString("\n[CHAPTER]\n")
		sb.WriteString("TIMEBASE=1/1000\n")
		sb.WriteString(fmt.Sprintf("START=%d\n", startMs))
		sb.WriteString(fmt.Sprintf("END=%d\n", endMs))
		sb.WriteString(fmt.Sprintf("title=%s\n", escapeFFMetaValue(chapterTitle)))

		startMs = endMs
	}

	return sb.String()
}

// escapeFFMetaValue escapes special characters in ffmetadata values.
// The format requires escaping =, ;, #, and \
func escapeFFMetaValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "=", "\\=")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, "#", "\\#")
	s = strings.ReplaceAll(s, "\n", "\\\n")
	return s
}

// ConvertToM4B runs the full conversion pipeline: probe, concat, transcode,
// apply metadata, optionally embed cover art, verify output, and clean up.
func ConvertToM4B(ctx context.Context, opts ConvertOptions) (*ConvertResult, error) {
	if opts.CmdFactory == nil {
		opts.CmdFactory = DefaultCmdFactory
	}
	if opts.Bitrate == "" {
		opts.Bitrate = "64k"
	}
	if opts.SampleRate == 0 {
		opts.SampleRate = 44100
	}
	if opts.Channels == 0 {
		opts.Channels = 1
	}

	// Validate: check ffmpeg and ffprobe are available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %w", err)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return nil, fmt.Errorf("ffprobe not found: %w", err)
	}

	// Find audio files
	files, err := FindAudioFiles(opts.InputDir)
	if err != nil {
		return nil, fmt.Errorf("find audio files: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no audio files found in %q", opts.InputDir)
	}

	// Probe all files
	probes, err := ProbeFiles(ctx, files, opts.CmdFactory)
	if err != nil {
		return nil, fmt.Errorf("probe files: %w", err)
	}

	// Validate sample rates (warn if inconsistent, conversion will normalize)
	sampleRates := make(map[int]bool)
	for _, p := range probes {
		if p.SampleRate > 0 {
			sampleRates[p.SampleRate] = true
		}
	}
	// No error on inconsistent rates -- ffmpeg normalizes via -ar flag

	// Create staging dir if needed
	if err := os.MkdirAll(opts.StagingDir, 0755); err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}

	// Write concat list
	concatPath := filepath.Join(opts.StagingDir, "filelist.txt")
	concatContent := GenerateConcatList(files)
	if err := os.WriteFile(concatPath, []byte(concatContent), 0644); err != nil {
		return nil, fmt.Errorf("write concat list: %w", err)
	}

	// Write ffmetadata
	metadataPath := filepath.Join(opts.StagingDir, "metadata.txt")
	metadataContent := GenerateFFMetadata(probes, opts)
	if err := os.WriteFile(metadataPath, []byte(metadataContent), 0644); err != nil {
		return nil, fmt.Errorf("write ffmetadata: %w", err)
	}

	// Temp file paths
	tempM4A := filepath.Join(opts.StagingDir, "temp.m4a")
	tempMeta := filepath.Join(opts.StagingDir, "temp_meta.m4b")

	// Cleanup on failure or after success
	tempFiles := []string{concatPath, metadataPath, tempM4A, tempMeta}
	defer func() {
		for _, f := range tempFiles {
			os.Remove(f)
		}
	}()

	// Step 1: Concat + transcode
	cmd := opts.CmdFactory(ctx, "ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", concatPath,
		"-vn",
		"-c:a", "aac",
		"-b:a", opts.Bitrate,
		"-ar", strconv.Itoa(opts.SampleRate),
		"-ac", strconv.Itoa(opts.Channels),
		tempM4A,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg concat+transcode: %w\noutput: %s", err, string(output))
	}

	// Step 2: Apply metadata
	cmd = opts.CmdFactory(ctx, "ffmpeg",
		"-i", tempM4A,
		"-i", metadataPath,
		"-map_metadata", "1",
		"-c", "copy",
		tempMeta,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg apply metadata: %w\noutput: %s", err, string(output))
	}

	// Step 3: Embed cover art or finalize
	outputPath := opts.OutputPath
	if opts.CoverPath != "" {
		if _, err := os.Stat(opts.CoverPath); err == nil {
			cmd = opts.CmdFactory(ctx, "ffmpeg",
				"-i", tempMeta,
				"-i", opts.CoverPath,
				"-map", "0:a",
				"-map", "1:v",
				"-c", "copy",
				"-disposition:v:0", "attached_pic",
				"-movflags", "+faststart",
				outputPath,
			)
		} else {
			// Cover path specified but file not found -- skip cover
			cmd = opts.CmdFactory(ctx, "ffmpeg",
				"-i", tempMeta,
				"-c", "copy",
				"-movflags", "+faststart",
				outputPath,
			)
		}
	} else {
		cmd = opts.CmdFactory(ctx, "ffmpeg",
			"-i", tempMeta,
			"-c", "copy",
			"-movflags", "+faststart",
			outputPath,
		)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		// Clean up partial output on failure
		os.Remove(outputPath)
		return nil, fmt.Errorf("ffmpeg finalize: %w\noutput: %s", err, string(output))
	}

	// Verify output file
	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("output file not found at %q: %w", outputPath, err)
	}
	if info.Size() == 0 {
		os.Remove(outputPath)
		return nil, fmt.Errorf("output file %q has zero size", outputPath)
	}

	// Calculate total duration
	var totalDuration float64
	for _, p := range probes {
		totalDuration += p.Duration
	}

	return &ConvertResult{
		OutputPath:   outputPath,
		Duration:     totalDuration,
		ChapterCount: len(probes),
		FileSize:     info.Size(),
	}, nil
}
