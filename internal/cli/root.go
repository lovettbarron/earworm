package cli

import (
	"github.com/lovettbarron/earworm/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	quiet   bool

	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "earworm",
	Short: "Audiobook library manager for Audible",
	Long: `Earworm is a CLI-driven audiobook library manager for Audible.
It tracks your local audiobook library, downloads new books via audible-cli,
and organizes them in a Libation-compatible file structure.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.InitConfig(cfgFile)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/earworm/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-essential output")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// SetVersion sets the build version information displayed by the version command.
func SetVersion(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date
}
