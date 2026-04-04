package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Audible via audible-cli",
	Long: `Run the audible-cli quickstart flow to authenticate with your Audible account.
This is an interactive process — you will be prompted for your Audible credentials
directly by audible-cli.`,
	RunE: runAuth,
}

func init() {
	rootCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) error {
	client := newAudibleClient()

	fmt.Fprintln(cmd.OutOrStdout(), "Starting Audible authentication...")
	fmt.Fprintln(cmd.OutOrStdout(), "You will be guided through the login process by audible-cli.")
	fmt.Fprintln(cmd.OutOrStdout(), "")

	if err := client.Quickstart(context.Background()); err != nil {
		return fmt.Errorf("authentication failed: %w\n\nCheck that audible-cli is installed: pip install audible-cli", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintln(cmd.OutOrStdout(), "Authentication successful! Run 'earworm sync' to sync your Audible library.")
	return nil
}
