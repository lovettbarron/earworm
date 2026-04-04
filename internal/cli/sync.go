package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncJSON bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Audible library metadata to local database",
	Long: `Pull your full Audible library metadata into the local database.
Each sync is a full refresh — all books are upserted. Local-only data
(download status, file paths) is preserved.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "output sync summary in JSON format")
	rootCmd.AddCommand(syncCmd)
}

// syncSummary holds the result of a sync operation for display/JSON output.
type syncSummary struct {
	TotalSynced  int `json:"total_synced"`
	NewBooks     int `json:"new_books"`
	AlreadyLocal int `json:"already_local"`
}

// newAudibleClient creates an AudibleClient from config. Extracted for testability.
var newAudibleClient = func() audible.AudibleClient {
	cliPath := viper.GetString("audible_cli_path")
	var opts []audible.ClientOption
	if profilePath := viper.GetString("audible.profile_path"); profilePath != "" {
		opts = append(opts, audible.WithProfilePath(profilePath))
	}
	return audible.NewClient(cliPath, opts...)
}

func runSync(cmd *cobra.Command, args []string) error {
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

	client := newAudibleClient()
	ctx := context.Background()

	// Pre-flight auth check (per D-11)
	if !quiet {
		fmt.Fprintln(cmd.ErrOrStderr(), "Checking authentication...")
	}
	if err := client.CheckAuth(ctx); err != nil {
		var authErr *audible.AuthError
		if errors.As(err, &authErr) {
			return fmt.Errorf("authentication expired. Run 'earworm auth' to re-authenticate")
		}
		return fmt.Errorf("auth check failed: %w", err)
	}

	// Export library (per D-06: full sync every time)
	if !quiet {
		fmt.Fprintln(cmd.ErrOrStderr(), "Syncing Audible library...")
	}
	items, err := client.LibraryExport(ctx)
	if err != nil {
		var authErr *audible.AuthError
		if errors.As(err, &authErr) {
			return fmt.Errorf("authentication expired. Run 'earworm auth' to re-authenticate")
		}
		return fmt.Errorf("library export failed: %w", err)
	}

	// Upsert all books (per D-05: remote wins on metadata, local fields preserved)
	for _, item := range items {
		book := db.Book{
			ASIN:           item.ASIN,
			Title:          item.Title,
			Author:         item.Authors,
			Narrator:       item.Narrators,
			SeriesName:     item.SeriesTitle,
			SeriesPosition: item.SeriesSequence,
			RuntimeMinutes: item.RuntimeMinutes(),
			PurchaseDate:   item.PurchaseDate,
			AudibleStatus:  item.AudibleStatus(),
			Narrators:      item.Narrators,
		}
		if err := db.SyncRemoteBook(database, book); err != nil {
			return fmt.Errorf("sync book %s: %w", item.ASIN, err)
		}
	}

	// Count new books
	newBooks, err := db.ListNewBooks(database)
	if err != nil {
		return fmt.Errorf("list new books: %w", err)
	}

	summary := syncSummary{
		TotalSynced:  len(items),
		NewBooks:     len(newBooks),
		AlreadyLocal: len(items) - len(newBooks),
	}

	// Output
	if syncJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Sync complete:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Synced:        %d books\n", summary.TotalSynced)
	fmt.Fprintf(cmd.OutOrStdout(), "  New:           %d (not yet downloaded)\n", summary.NewBooks)
	fmt.Fprintf(cmd.OutOrStdout(), "  Already local: %d\n", summary.AlreadyLocal)
	return nil
}
