package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print earworm version and build information",
	RunE: func(cmd *cobra.Command, args []string) error {
		if quiet {
			fmt.Fprintln(cmd.OutOrStdout(), buildVersion)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "earworm version %s (commit: %s, built: %s)\n",
			buildVersion, buildCommit, buildDate)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
