package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigDir returns ~/.config/earworm/ (hardcoded XDG path per D-04).
// Does NOT use os.UserConfigDir() which returns ~/Library/Application Support on macOS.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "earworm"), nil
}

// ConfigFilePath returns ~/.config/earworm/config.yaml
func ConfigFilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DBPath returns ~/.config/earworm/earworm.db
func DBPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "earworm.db"), nil
}

// DefaultStagingPath returns ~/.config/earworm/staging
func DefaultStagingPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "staging"), nil
}
