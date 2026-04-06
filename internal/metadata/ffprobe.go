package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Test seams for subprocess mocking.
var lookPathFn = exec.LookPath
var execCommandCtx = exec.CommandContext

// ffprobeOutput represents the JSON output from ffprobe.
type ffprobeOutput struct {
	Format   ffprobeFormat    `json:"format"`
	Chapters []ffprobeChapter `json:"chapters"`
}

type ffprobeFormat struct {
	Duration string            `json:"duration"`
	Tags     map[string]string `json:"tags"`
}

type ffprobeChapter struct {
	ID int `json:"id"`
}

// extractWithFFprobe extracts metadata from an M4A file using ffprobe subprocess.
func extractWithFFprobe(filePath string) (*BookMetadata, error) {
	// Check if ffprobe is available
	ffprobePath, err := lookPathFn("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := execCommandCtx(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_chapters",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed on %s: %w", filePath, err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	meta := &BookMetadata{
		Source:       SourceFFprobe,
		ChapterCount: len(probe.Chapters),
	}

	// Parse duration
	if probe.Format.Duration != "" {
		if dur, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			meta.Duration = int(math.Round(dur))
		}
	}

	// Map tags (case insensitive)
	tags := normalizeTagKeys(probe.Format.Tags)
	meta.Title = tags["title"]
	if v := tags["album_artist"]; v != "" {
		meta.Author = v
	} else {
		meta.Author = tags["artist"]
	}
	meta.Narrator = tags["artist"]
	meta.Genre = tags["genre"]

	// Parse year from date tag
	if dateStr := tags["date"]; dateStr != "" {
		if len(dateStr) >= 4 {
			if y, err := strconv.Atoi(dateStr[:4]); err == nil {
				meta.Year = y
			}
		}
	}

	return meta, nil
}

// normalizeTagKeys lowercases all keys in a tag map for case-insensitive lookup.
func normalizeTagKeys(tags map[string]string) map[string]string {
	normalized := make(map[string]string, len(tags))
	for k, v := range tags {
		normalized[strings.ToLower(k)] = v
	}
	return normalized
}
