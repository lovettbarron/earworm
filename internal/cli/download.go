package cli

import (
	"encoding/json"
	"fmt"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/spf13/cobra"
)

var (
	dryRun       bool
	downloadJSON bool
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download new audiobooks from Audible",
	Long: `Download audiobooks that are in your Audible library but not yet
downloaded locally. Use --dry-run to preview what would be downloaded.`,
	RunE: runDownload,
}

func init() {
	downloadCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview downloads without downloading")
	downloadCmd.Flags().BoolVar(&downloadJSON, "json", false, "output in JSON format")
	rootCmd.AddCommand(downloadCmd)
}

// dryRunBook is the JSON output structure for dry-run mode.
type dryRunBook struct {
	ASIN           string `json:"asin"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	Narrators      string `json:"narrators"`
	RuntimeMinutes int    `json:"runtime_minutes"`
	SeriesName     string `json:"series_name,omitempty"`
	SeriesPosition string `json:"series_position,omitempty"`
}

func runDownload(cmd *cobra.Command, args []string) error {
	if !dryRun {
		return fmt.Errorf("download not yet implemented\n\nUse --dry-run to preview what would be downloaded")
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

	// Get new books
	books, err := db.ListNewBooks(database)
	if err != nil {
		return fmt.Errorf("failed to list new books: %w", err)
	}

	// JSON output
	if downloadJSON {
		var items []dryRunBook
		for _, b := range books {
			items = append(items, dryRunBook{
				ASIN:           b.ASIN,
				Title:          b.Title,
				Author:         b.Author,
				Narrators:      b.Narrators,
				RuntimeMinutes: b.RuntimeMinutes,
				SeriesName:     b.SeriesName,
				SeriesPosition: b.SeriesPosition,
			})
		}
		if items == nil {
			items = []dryRunBook{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Empty
	if len(books) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No new books to download.")
		return nil
	}

	// Per D-08: Author - Title [ASIN] (runtime)
	for _, b := range books {
		runtime := formatRuntime(b.RuntimeMinutes)
		fmt.Fprintf(cmd.OutOrStdout(), "%s - %s [%s] (%s)\n", b.Author, b.Title, b.ASIN, runtime)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%d books to download\n", len(books))
	return nil
}

// formatRuntime formats minutes into "Xh Ym" display string.
func formatRuntime(minutes int) string {
	if minutes == 0 {
		return "unknown"
	}
	h := minutes / 60
	m := minutes % 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
