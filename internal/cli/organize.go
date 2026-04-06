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

var organizeCmd = &cobra.Command{
	Use:   "organize",
	Short: "Organize downloaded books into library folder structure",
	Long: `Move downloaded audiobooks from the staging directory into the library
in Audiobookshelf-compatible Author/Title [ASIN]/ folder structure.

Operates on all books with 'downloaded' status. Books missing required
metadata (author, title) are marked as errors.`,
	RunE: runOrganize,
}

func init() {
	organizeCmd.Flags().BoolVar(&organizeJSON, "json", false, "output results in JSON format")
	rootCmd.AddCommand(organizeCmd)
}

// jsonOrganizeOutput is the JSON output structure for the organize command.
type jsonOrganizeOutput struct {
	Organized int                      `json:"organized"`
	Errors    int                      `json:"errors"`
	Results   []organize.OrganizeResult `json:"results"`
}

func runOrganize(cmd *cobra.Command, args []string) error {
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

	// Run organization
	results, err := organize.OrganizeAll(database, stagingPath, libraryPath)
	if err != nil {
		return fmt.Errorf("organize failed: %w", err)
	}

	// Count successes and failures
	var successCount, errorCount int
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			errorCount++
		}
	}

	// JSON output
	if organizeJSON {
		output := jsonOrganizeOutput{
			Organized: successCount,
			Errors:    errorCount,
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
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Error: %s - %s: %s\n", r.Author, r.Title, r.Error)
			}
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Organized %d books, %d errors\n", successCount, errorCount)

	// Trigger Audiobookshelf library scan after successful organization.
	// Silent skip if unconfigured. Warn and continue on failure.
	if successCount > 0 {
		if absURL := viper.GetString("audiobookshelf.url"); absURL != "" {
			abs := audiobookshelf.NewClient(
				absURL,
				viper.GetString("audiobookshelf.token"),
				viper.GetString("audiobookshelf.library_id"),
			)
			if scanErr := abs.ScanLibrary(); scanErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: Audiobookshelf scan failed: %v\n", scanErr)
			} else if !quiet {
				fmt.Fprintln(cmd.OutOrStdout(), "Audiobookshelf library scan triggered.")
			}
		}
	}

	return nil
}
