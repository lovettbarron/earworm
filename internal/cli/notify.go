package cli

import (
	"encoding/json"
	"fmt"

	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var notifyJSON bool

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Trigger Audiobookshelf library scan",
	Long: `Trigger a library scan on your Audiobookshelf server.
Requires audiobookshelf.url, audiobookshelf.token, and audiobookshelf.library_id
to be configured.`,
	RunE: runNotify,
}

func init() {
	notifyCmd.Flags().BoolVar(&notifyJSON, "json", false, "output in JSON format")
	rootCmd.AddCommand(notifyCmd)
}

func runNotify(cmd *cobra.Command, args []string) error {
	absURL := viper.GetString("audiobookshelf.url")
	if absURL == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Audiobookshelf not configured. Set audiobookshelf.url, audiobookshelf.token, and audiobookshelf.library_id in config.")
		return nil
	}

	abs := audiobookshelf.NewClient(
		absURL,
		viper.GetString("audiobookshelf.token"),
		viper.GetString("audiobookshelf.library_id"),
	)

	if err := abs.ScanLibrary(); err != nil {
		return fmt.Errorf("audiobookshelf scan failed: %w", err)
	}

	if notifyJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(map[string]string{
			"status":  "ok",
			"message": "Library scan triggered",
		})
	}

	if !quiet {
		fmt.Fprintln(cmd.OutOrStdout(), "Audiobookshelf library scan triggered.")
	}

	return nil
}
