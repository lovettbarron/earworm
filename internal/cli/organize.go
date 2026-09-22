package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/organize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var organizeJSON bool
var organizeRetry bool

var organizeCmd = &cobra.Command{
	Use:   "organize",
	Short: "Organize downloaded books into library folder structure",
	Long: `Move downloaded audiobooks from the staging directory into the library.

The folder structure is controlled by library.layout config:
  flat:          Title [ASIN]/ (default)
  author-title:  Author/Title [ASIN]/

Operates on all books with 'downloaded' status. Books missing required
metadata (author, title) are marked as errors.`,
	RunE: runOrganize,
}

func init() {
	organizeCmd.Flags().BoolVar(&organizeJSON, "json", false, "output results in JSON format")
	organizeCmd.Flags().BoolVar(&organizeRetry, "retry", false, "reset previously failed books and retry organizing them")
	rootCmd.AddCommand(organizeCmd)
}

// jsonOrganizeOutput is the JSON output structure for the organize command.
type jsonOrganizeOutput struct {
	Organized int                      `json:"organized"`
	Errors    int                      `json:"errors"`
	Results   []organize.OrganizeResult `json:"results"`
}

func runOrganize(cmd *cobra.Command, args []string) error {
	// The library lives on a network mount that can go away. Probing first
	// turns an indefinite hang into an immediate, explanatory failure.
	if err := ensureLibraryAvailable(cmd.Context(), cmd.OutOrStdout()); err != nil {
		return err
	}

	// Validate required config
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

	// Reset error'd books back to "downloaded" if --retry is set.
	if organizeRetry {
		reset, err := db.ResetOrganizeErrors(database, stagingPath)
		if err != nil {
			return fmt.Errorf("failed to reset errors: %w", err)
		}
		if reset > 0 && !quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "Reset %d failed books for retry\n", reset)
		}
	}

	// Validate library path is reachable and writable before attempting moves.
	if err := organize.ValidateLibraryPath(libraryPath); err != nil {
		// Count how many books are waiting.
		staged, _ := db.CountByStatus(database, "downloaded")
		if staged > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %d books in staging awaiting transfer\n", staged)
		}
		return fmt.Errorf("library not accessible: %w\n\nBooks remain in staging and will be organized on next successful run", err)
	}

	// Run organization
	layout := viper.GetString("library.layout")
	results, err := organize.OrganizeAll(database, stagingPath, libraryPath, layout)
	if err != nil {
		return fmt.Errorf("organize failed: %w", err)
	}

	// Count successes, retryable failures, and permanent errors
	var successCount, retryCount, errorCount int
	for _, r := range results {
		if r.Success {
			successCount++
		} else if r.Retryable {
			retryCount++
		} else {
			errorCount++
		}
	}

	// JSON output
	if organizeJSON {
		output := jsonOrganizeOutput{
			Organized: successCount,
			Errors:    errorCount + retryCount,
			Results:   results,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Text output
	if !quiet {
		for _, r := range results {
			if r.Success {
				fmt.Fprintf(cmd.OutOrStdout(), "Organized: %s - %s -> %s\n", r.Author, r.Title, r.LibPath)
			} else if r.Retryable {
				fmt.Fprintf(cmd.OutOrStdout(), "Retry: %s - %s: %s (will retry next run)\n", r.Author, r.Title, r.Error)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: %s - %s: %s\n", r.Author, r.Title, r.Error)
			}
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Organized %d books", successCount)
	if retryCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), ", %d pending retry", retryCount)
	}
	if errorCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), ", %d errors", errorCount)
	}
	fmt.Fprintln(cmd.OutOrStdout())

	// Trigger Audiobookshelf library scan after successful organization.
	// Silent skip if unconfigured. Warn and continue on failure.
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

	if successCount > 0 && !absConfigured {
		hint(cmd.ErrOrStderr(), "earworm notify            # trigger Audiobookshelf library scan")
	} else if errorCount > 0 && successCount == 0 {
		hint(cmd.ErrOrStderr(), "earworm status --status error  # inspect failed books")
	}

	return nil
}
