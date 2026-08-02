package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovettbarron/earworm/internal/audio"
	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/organize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	convertBitrate       string
	convertSampleRate    int
	convertMono          bool
	convertKeepOriginals bool
	convertDryRun        bool
	convertAll           bool
	convertJSON          bool
)

var convertCmd = &cobra.Command{
	Use:   "convert [ASIN...]",
	Short: "Convert audiobooks to M4B format",
	Long: `Convert multi-file audiobooks (MP3, etc.) into single M4B files with chapters.

Each source audio file becomes a chapter in the output M4B. The original files
are removed after successful conversion unless --keep-originals is specified.

Requires ffmpeg and ffprobe to be installed.`,
	RunE: runConvert,
}

func init() {
	convertCmd.Flags().StringVar(&convertBitrate, "bitrate", "64k", "audio bitrate (e.g. 64k, 128k)")
	convertCmd.Flags().IntVar(&convertSampleRate, "sample-rate", 44100, "sample rate in Hz")
	convertCmd.Flags().BoolVar(&convertMono, "mono", true, "mono output")
	convertCmd.Flags().BoolVar(&convertKeepOriginals, "keep-originals", false, "keep original files after conversion")
	convertCmd.Flags().BoolVar(&convertDryRun, "dry-run", false, "show what would be converted without doing it")
	convertCmd.Flags().BoolVar(&convertAll, "all", false, "convert all multi-file books in library")
	convertCmd.Flags().BoolVar(&convertJSON, "json", false, "output results in JSON format")
	rootCmd.AddCommand(convertCmd)
}

// convertResult holds the outcome of converting a single book.
type convertResult struct {
	ASIN    string `json:"asin"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
	Files   int    `json:"files,omitempty"`
}

// jsonConvertOutput is the JSON output structure for the convert command.
type jsonConvertOutput struct {
	Converted int             `json:"converted"`
	Errors    int             `json:"errors"`
	Skipped   int             `json:"skipped"`
	Results   []convertResult `json:"results"`
}

func runConvert(cmd *cobra.Command, args []string) error {
	libraryPath := viper.GetString("library_path")
	if libraryPath == "" {
		return fmt.Errorf("library_path not configured\n\nRun: earworm config set library_path /path/to/audiobooks")
	}

	stagingPath := viper.GetString("staging_path")
	if stagingPath == "" {
		configDir, err := config.ConfigDir()
		if err != nil {
			return fmt.Errorf("failed to determine config directory: %w", err)
		}
		stagingPath = filepath.Join(configDir, "staging")
	}

	// Open database
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Determine which books to convert
	var books []db.Book
	if convertAll {
		allBooks, err := db.ListBooks(database)
		if err != nil {
			return fmt.Errorf("list books: %w", err)
		}
		for _, b := range allBooks {
			if b.LocalPath != "" && (b.Status == "organized" || b.Status == "scanned") {
				books = append(books, b)
			}
		}
	} else if len(args) > 0 {
		for _, asin := range args {
			book, err := db.GetBook(database, asin)
			if err != nil {
				return fmt.Errorf("get book %s: %w", asin, err)
			}
			if book == nil {
				return fmt.Errorf("book %s not found", asin)
			}
			books = append(books, *book)
		}
	} else {
		return fmt.Errorf("specify ASINs or use --all to convert all multi-file books")
	}

	channels := 2
	if convertMono {
		channels = 1
	}

	var results []convertResult
	var successCount, errorCount, skipCount int

	for _, book := range books {
		result := convertResult{
			ASIN:   book.ASIN,
			Title:  book.Title,
			Author: book.Author,
		}

		// Verify book exists at LocalPath
		bookDir := book.LocalPath
		if bookDir == "" {
			result.Error = "no local path set"
			result.Success = false
			errorCount++
			results = append(results, result)
			continue
		}

		if _, err := os.Stat(bookDir); os.IsNotExist(err) {
			result.Error = fmt.Sprintf("directory not found: %s", bookDir)
			result.Success = false
			errorCount++
			results = append(results, result)
			continue
		}

		// Find audio files in book directory
		audioFiles, err := audio.FindAudioFiles(bookDir)
		if err != nil {
			result.Error = fmt.Sprintf("find audio files: %v", err)
			result.Success = false
			errorCount++
			results = append(results, result)
			continue
		}

		// Skip if already a single M4B file
		if len(audioFiles) == 1 {
			ext := strings.ToLower(filepath.Ext(audioFiles[0]))
			if ext == ".m4b" {
				skipCount++
				continue
			}
		}

		// Skip if no multi-file conversion needed (single file that's not M4B could still convert)
		if len(audioFiles) == 0 {
			skipCount++
			continue
		}

		result.Files = len(audioFiles)

		if convertDryRun {
			result.DryRun = true
			result.Success = true
			if !convertJSON && !quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "Would convert: %s - %s (%d files)\n",
					book.Author, book.Title, len(audioFiles))
			}
			results = append(results, result)
			continue
		}

		// Create staging dir for this conversion
		convertStagingDir := filepath.Join(stagingPath, "convert", book.ASIN)
		if err := os.MkdirAll(convertStagingDir, 0755); err != nil {
			result.Error = fmt.Sprintf("create staging dir: %v", err)
			result.Success = false
			errorCount++
			results = append(results, result)
			continue
		}

		// Find cover image in book directory
		coverPath := findCoverImage(bookDir)

		// Output M4B goes to staging first
		outputFilename := organize.RenameAudioFile(book.Title, ".m4b")
		stagingOutputPath := filepath.Join(convertStagingDir, outputFilename)

		opts := audio.ConvertOptions{
			InputDir:     bookDir,
			OutputPath:   stagingOutputPath,
			StagingDir:   convertStagingDir,
			Bitrate:      convertBitrate,
			SampleRate:   convertSampleRate,
			Channels:     channels,
			BookTitle:    book.Title,
			BookAuthor:   book.Author,
			BookNarrator: book.Narrator,
			BookYear:     fmt.Sprintf("%d", book.Year),
			CoverPath:    coverPath,
			CmdFactory:   audio.DefaultCmdFactory,
		}

		convertResult, err := audio.ConvertToM4B(cmd.Context(), opts)
		if err != nil {
			result.Error = fmt.Sprintf("convert: %v", err)
			result.Success = false
			errorCount++
			results = append(results, result)
			// Clean up staging on failure
			os.RemoveAll(convertStagingDir)
			continue
		}

		// Move verified M4B from staging to library directory
		finalPath := filepath.Join(bookDir, outputFilename)
		if err := organize.MoveFile(stagingOutputPath, finalPath); err != nil {
			result.Error = fmt.Sprintf("move to library: %v", err)
			result.Success = false
			errorCount++
			results = append(results, result)
			os.RemoveAll(convertStagingDir)
			continue
		}

		// Remove original audio files (but not cover/chapters/other files)
		if !convertKeepOriginals {
			for _, f := range audioFiles {
				os.Remove(f)
			}
		}

		// Update database
		_, dbErr := database.Exec(
			`UPDATE books SET file_count = 1, duration = ?, chapter_count = ?, updated_at = CURRENT_TIMESTAMP WHERE asin = ?`,
			int(convertResult.Duration), convertResult.ChapterCount, book.ASIN,
		)
		if dbErr != nil {
			// Non-fatal: conversion succeeded, just log warning
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update database for %s: %v\n", book.ASIN, dbErr)
		}

		// Clean up staging directory
		os.RemoveAll(convertStagingDir)

		result.Success = true
		successCount++
		results = append(results, result)

		if !quiet && !convertJSON {
			fmt.Fprintf(cmd.OutOrStdout(), "Converted: %s - %s (%d files -> 1 M4B, %d chapters)\n",
				book.Author, book.Title, len(audioFiles), convertResult.ChapterCount)
		}
	}

	if results == nil {
		results = []convertResult{}
	}

	// JSON output
	if convertJSON {
		output := jsonConvertOutput{
			Converted: successCount,
			Errors:    errorCount,
			Skipped:   skipCount,
			Results:   results,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Summary
	if !convertDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Converted %d books, %d errors, %d skipped\n",
			successCount, errorCount, skipCount)
	}

	// Trigger Audiobookshelf library scan after successful conversions
	absConfigured := viper.GetString("audiobookshelf.url") != ""
	if successCount > 0 && absConfigured {
		abs := audiobookshelf.NewClient(
			viper.GetString("audiobookshelf.url"),
			viper.GetString("audiobookshelf.token"),
			viper.GetString("audiobookshelf.library_id"),
		)
		if scanErr := abs.ScanLibrary(); scanErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: Audiobookshelf scan failed: %v\n", scanErr)
		} else if !quiet {
			fmt.Fprintln(cmd.OutOrStdout(), "Audiobookshelf library scan triggered.")
		}
	}

	// Hints
	if successCount > 0 && !absConfigured {
		hint(cmd.ErrOrStderr(), "earworm notify            # trigger Audiobookshelf library scan")
	} else if errorCount > 0 && successCount == 0 {
		hint(cmd.ErrOrStderr(), "earworm status --status error  # inspect failed books")
	}

	return nil
}

// findCoverImage looks for a cover image in the given directory.
// Returns the path to the first found cover image, or empty string.
func findCoverImage(dir string) string {
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.png"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
