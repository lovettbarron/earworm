package cli

import (
	"fmt"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/spf13/cobra"
)

var skipCmd = &cobra.Command{
	Use:   "skip [asin...]",
	Short: "Skip books so they won't be downloaded",
	Long: `Mark one or more books as skipped so they are excluded from future downloads.
Use this for subscription books you no longer have access to, or books you don't want.

Use --undo to un-skip books and make them downloadable again.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSkip,
}

var undoSkip bool

func init() {
	skipCmd.Flags().BoolVar(&undoSkip, "undo", false, "un-skip books (mark as unknown again)")
	rootCmd.AddCommand(skipCmd)
}

func runSkip(cmd *cobra.Command, args []string) error {
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	newStatus := "skipped"
	if undoSkip {
		newStatus = "unknown"
	}

	for _, asin := range args {
		book, err := db.GetBook(database, asin)
		if err != nil {
			return fmt.Errorf("looking up %s: %w", asin, err)
		}
		if book == nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: ASIN %s not found in database, skipping\n", asin)
			continue
		}

		if err := db.UpdateBookStatus(database, asin, newStatus); err != nil {
			return fmt.Errorf("updating %s: %w", asin, err)
		}

		if undoSkip {
			fmt.Fprintf(cmd.OutOrStdout(), "Un-skipped: %s - %s [%s]\n", book.Author, book.Title, asin)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Skipped: %s - %s [%s]\n", book.Author, book.Title, asin)
		}
	}

	return nil
}
