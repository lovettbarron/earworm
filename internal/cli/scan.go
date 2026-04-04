package cli

import (
	"fmt"
	"os"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/metadata"
	"github.com/lovettbarron/earworm/internal/scanner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var scanRecursive bool

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan local library for audiobooks",
	Long: `Scan a Libation-compatible audiobook library directory and index all
discovered books into the local database. The library is expected to follow
the structure: Author Name/Book Title [ASIN]/book.m4a

Use --recursive to search deeply nested directory structures.`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().BoolVarP(&scanRecursive, "recursive", "r", false, "recursively scan nested directories")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	// Get library path from config
	libPath := viper.GetString("library_path")
	if libPath == "" {
		return fmt.Errorf("library path not configured\n\nRun 'earworm config set library_path /path/to/library' to set your library location")
	}

	// Validate library path exists and is a directory
	info, err := os.Stat(libPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("library path %q does not exist or is not a directory\n\nCheck that your library path is correct and accessible: earworm config show", libPath)
	}

	// Open database
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w\n\nTry removing %s and running again", err, dbPath)
	}
	defer database.Close()

	// Start spinner for progress feedback on stderr
	spin := NewSpinner(cmd.ErrOrStderr(), "Scanning")
	if !quiet {
		spin.Start()
	}

	// Scan library directory
	discovered, skipped, err := scanner.ScanLibrary(libPath, scanRecursive)
	if err != nil {
		if !quiet {
			spin.Stop()
		}
		return fmt.Errorf("scan failed: %w\n\nCheck that the library path is accessible and you have read permissions", err)
	}

	// Update spinner count and stop
	for range discovered {
		spin.Increment()
	}
	if !quiet {
		spin.Stop()
		fmt.Fprintf(cmd.ErrOrStderr(), "Found %d books, extracting metadata...\n", len(discovered))
	}

	// Adapter: convert metadata.ExtractMetadata to scanner.BookMetadata
	metadataFn := func(bookDir string) (*scanner.BookMetadata, error) {
		meta, err := metadata.ExtractMetadata(bookDir)
		if err != nil {
			return nil, err
		}
		return &scanner.BookMetadata{
			Title:        meta.Title,
			Author:       meta.Author,
			Narrator:     meta.Narrator,
			Genre:        meta.Genre,
			Year:         meta.Year,
			Series:       meta.Series,
			HasCover:     meta.HasCover,
			Duration:     meta.Duration,
			ChapterCount: meta.ChapterCount,
			FileCount:    meta.FileCount,
			Source:       string(meta.Source),
		}, nil
	}

	// Incremental sync to database
	result, err := scanner.IncrementalSync(database, discovered, metadataFn)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Print summary to stdout
	fmt.Fprintf(cmd.OutOrStdout(), "Scan complete:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Added:   %d\n", result.Added)
	fmt.Fprintf(cmd.OutOrStdout(), "  Updated: %d\n", result.Updated)
	fmt.Fprintf(cmd.OutOrStdout(), "  Removed: %d\n", result.Removed)
	fmt.Fprintf(cmd.OutOrStdout(), "  Skipped: %d\n", result.Skipped)

	// Print skipped folders if any and not quiet
	if len(skipped) > 0 && !quiet {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nSkipped %d folders:\n", len(skipped))
		limit := 10
		if len(skipped) < limit {
			limit = len(skipped)
		}
		for _, s := range skipped[:limit] {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s (%s)\n", s.Path, s.Reason)
		}
		if len(skipped) > 10 {
			fmt.Fprintf(cmd.ErrOrStderr(), "  ... and %d more\n", len(skipped)-10)
		}
	}

	return nil
}
