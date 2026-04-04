package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/viper"
)

// SetDefaults configures all default values for the earworm configuration.
func SetDefaults() {
	viper.SetDefault("library_path", "")
	viper.SetDefault("staging_path", "")
	viper.SetDefault("audible_cli_path", "audible")
	viper.SetDefault("audiobookshelf.url", "")
	viper.SetDefault("audiobookshelf.token", "")
	viper.SetDefault("audiobookshelf.library_id", "")
	viper.SetDefault("download.rate_limit_seconds", 5)
	viper.SetDefault("download.max_retries", 3)
	viper.SetDefault("download.backoff_multiplier", 2.0)
	viper.SetDefault("scan.recursive", false)
}

// InitConfig initializes the configuration system. If cfgFile is provided, it
// is used directly. Otherwise, the default config directory is searched for
// config.yaml. Missing config files are not an error (defaults apply).
func InitConfig(cfgFile string) error {
	SetDefaults()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		configDir, err := ConfigDir()
		if err != nil {
			return fmt.Errorf("resolving config directory: %w", err)
		}
		viper.SetConfigType("yaml")
		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
	}

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("reading config file: %w", err)
		}
		// Config file not found is fine -- defaults are used.
	}

	return nil
}

// Validate checks the current configuration for errors.
func Validate() error {
	libPath := viper.GetString("library_path")
	if libPath != "" {
		info, err := os.Stat(libPath)
		if err != nil {
			return fmt.Errorf("library_path %q does not exist: %w", libPath, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("library_path %q is not a directory", libPath)
		}
	}

	if viper.GetInt("download.rate_limit_seconds") <= 0 {
		return fmt.Errorf("download.rate_limit_seconds must be > 0, got %d", viper.GetInt("download.rate_limit_seconds"))
	}

	if viper.GetInt("download.max_retries") < 0 {
		return fmt.Errorf("download.max_retries must be >= 0, got %d", viper.GetInt("download.max_retries"))
	}

	return nil
}

// ValidKeys returns a sorted list of all recognized configuration keys.
func ValidKeys() []string {
	keys := []string{
		"audible_cli_path",
		"audiobookshelf.library_id",
		"audiobookshelf.token",
		"audiobookshelf.url",
		"download.backoff_multiplier",
		"download.max_retries",
		"download.rate_limit_seconds",
		"library_path",
		"scan.recursive",
		"staging_path",
	}
	sort.Strings(keys)
	return keys
}

// WriteDefaultConfig creates a default configuration file at the given path.
// The parent directory is created if it does not exist.
func WriteDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory %q: %w", dir, err)
	}

	SetDefaults()

	if err := viper.WriteConfigAs(path); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}
