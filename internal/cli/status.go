package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/spf13/cobra"
)

var (
	jsonOutput   bool
	filterAuthor string
	filterStatus string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show library contents and status",
	Long: `Display the current state of your audiobook library including book
metadata and download status. Use --json for machine-readable output.

Filter by author with --author or by status with --status.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	statusCmd.Flags().StringVar(&filterAuthor, "author", "", "filter by author (substring match)")
	statusCmd.Flags().StringVar(&filterStatus, "status", "", "filter by status (exact match)")
	rootCmd.AddCommand(statusCmd)
}

// statusIndicator maps book status to a short display indicator.
func statusIndicator(status string) string {
	switch status {
	case "scanned":
		return "OK"
	case "downloaded":
		return "DL"
	case "organized":
		return "OK"
	case "error":
		return "ERR"
	case "removed":
		return "GONE"
	default:
		return "?"
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Open database
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w\n\nRun 'earworm scan' first to initialize your library", err)
	}
	defer database.Close()

	// List all books
	books, err := db.ListBooks(database)
	if err != nil {
		return fmt.Errorf("failed to list books: %w", err)
	}

	// Apply filters
	if filterAuthor != "" || filterStatus != "" {
		var filtered []db.Book
		for _, b := range books {
			if filterAuthor != "" && !strings.Contains(strings.ToLower(b.Author), strings.ToLower(filterAuthor)) {
				continue
			}
			if filterStatus != "" && b.Status != filterStatus {
				continue
			}
			filtered = append(filtered, b)
		}
		books = filtered
	}

	// JSON output
	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(books)
	}

	// Empty library
	if len(books) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No books in library. Run 'earworm scan' to index your library.")
		return nil
	}

	// One-line-per-book format
	for _, b := range books {
		fmt.Fprintf(cmd.OutOrStdout(), "%s - %s [%s] (%s)\n",
			b.Author, b.Title, b.ASIN, statusIndicator(b.Status))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%d books total\n", len(books))

	return nil
}
