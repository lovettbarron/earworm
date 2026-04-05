package cli

import (
	"fmt"
	"os"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/goodreads"
	"github.com/spf13/cobra"
)

var goodreadsOutput string

var goodreadsCmd = &cobra.Command{
	Use:   "goodreads",
	Short: "Export library to Goodreads CSV",
	Long: `Export your audiobook library as a CSV file compatible with Goodreads
import. Books are placed on the 'read' and 'audiobook' shelves.

By default, output goes to stdout. Use --output to write to a file.`,
	RunE: runGoodreads,
}

func init() {
	goodreadsCmd.Flags().StringVarP(&goodreadsOutput, "output", "o", "", "output file path (default: stdout)")
	rootCmd.AddCommand(goodreadsCmd)
}

func runGoodreads(cmd *cobra.Command, args []string) error {
	// Open database.
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Get all books.
	books, err := db.ListBooks(database)
	if err != nil {
		return fmt.Errorf("failed to list books: %w", err)
	}

	// Determine output writer.
	writer := cmd.OutOrStdout()
	if goodreadsOutput != "" {
		f, err := os.Create(goodreadsOutput)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		writer = f
	}

	if err := goodreads.ExportCSV(writer, books); err != nil {
		return fmt.Errorf("CSV export failed: %w", err)
	}

	if !quiet && goodreadsOutput != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Exported %d books to %s\n", len(books), goodreadsOutput)
	}

	return nil
}
