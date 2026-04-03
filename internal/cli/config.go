package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage earworm configuration",
	Long:  "View, initialize, and modify earworm configuration settings.",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, err := config.ConfigFilePath()
		if err != nil {
			return err
		}

		// Check if file already exists -- do NOT overwrite.
		if _, err := os.Stat(cfgPath); err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Configuration file already exists at %s\n", cfgPath)
			return nil
		}

		if err := config.WriteDefaultConfig(cfgPath); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Configuration file created at %s\n", cfgPath)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		if quiet {
			return nil
		}

		settings := viper.AllSettings()
		out, err := yaml.Marshal(settings)
		if err != nil {
			return fmt.Errorf("marshalling config: %w", err)
		}

		fmt.Fprint(cmd.OutOrStdout(), string(out))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		// Validate key against known keys.
		validKeys := config.ValidKeys()
		found := false
		for _, k := range validKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown config key %q\nValid keys: %s", key, strings.Join(validKeys, ", "))
		}

		viper.Set(key, value)

		// Ensure config file exists before writing.
		cfgPath, err := config.ConfigFilePath()
		if err != nil {
			return err
		}

		if _, err := os.Stat(cfgPath); err != nil {
			// Config file doesn't exist yet -- create it.
			if writeErr := config.WriteDefaultConfig(cfgPath); writeErr != nil {
				return writeErr
			}
		}

		if err := viper.WriteConfigAs(cfgPath); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, value)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
