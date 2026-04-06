package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })
}

func TestSetDefaults(t *testing.T) {
	resetViper(t)
	SetDefaults()
	assert.Equal(t, "audible", viper.GetString("audible_cli_path"))
	assert.Equal(t, "", viper.GetString("library_path"))
	assert.Contains(t, viper.GetString("staging_path"), filepath.Join(".config", "earworm", "staging"))
}

func TestSetDefaultsRateLimit(t *testing.T) {
	resetViper(t)
	SetDefaults()
	assert.Equal(t, 5, viper.GetInt("download.rate_limit_seconds"))
}

func TestSetDefaultsMaxRetries(t *testing.T) {
	resetViper(t)
	SetDefaults()
	assert.Equal(t, 3, viper.GetInt("download.max_retries"))
}

func TestSetDefaultsBackoff(t *testing.T) {
	resetViper(t)
	SetDefaults()
	assert.Equal(t, 2.0, viper.GetFloat64("download.backoff_multiplier"))
}

func TestConfigDir(t *testing.T) {
	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(dir))
	assert.Contains(t, dir, filepath.Join(".config", "earworm"))
}

func TestDBPath(t *testing.T) {
	p, err := DBPath()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(p))
	assert.Contains(t, p, filepath.Join(".config", "earworm", "earworm.db"))
}

func TestConfigFilePath(t *testing.T) {
	p, err := ConfigFilePath()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(p))
	assert.Contains(t, p, filepath.Join(".config", "earworm", "config.yaml"))
}

func TestValidKeys(t *testing.T) {
	keys := ValidKeys()
	assert.Contains(t, keys, "library_path")
	assert.Contains(t, keys, "staging_path")
	assert.Contains(t, keys, "audible_cli_path")
	assert.Contains(t, keys, "download.rate_limit_seconds")
	// Verify sorted
	for i := 1; i < len(keys); i++ {
		assert.True(t, keys[i-1] <= keys[i], "keys should be sorted: %s > %s", keys[i-1], keys[i])
	}
}

func TestValidateEmptyLibraryPath(t *testing.T) {
	resetViper(t)
	SetDefaults()
	assert.NoError(t, Validate())
}

func TestValidateNonexistentLibraryPath(t *testing.T) {
	resetViper(t)
	SetDefaults()
	viper.Set("library_path", "/nonexistent/path/that/does/not/exist")
	err := Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "library_path")
}

func TestValidateValidLibraryPath(t *testing.T) {
	resetViper(t)
	SetDefaults()
	tmpDir := t.TempDir()
	viper.Set("library_path", tmpDir)
	assert.NoError(t, Validate())
}

func TestInitConfigNoFile(t *testing.T) {
	resetViper(t)
	// InitConfig with empty string and no config file on disk should succeed with defaults.
	err := InitConfig("")
	assert.NoError(t, err)
	assert.Equal(t, "audible", viper.GetString("audible_cli_path"))
}

func TestInitConfigWithFile(t *testing.T) {
	resetViper(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte("library_path: /tmp/testlib\n"), 0644)
	require.NoError(t, err)

	err = InitConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/testlib", viper.GetString("library_path"))
	// Defaults still apply for unset keys.
	assert.Equal(t, "audible", viper.GetString("audible_cli_path"))
}

func TestWriteDefaultConfig(t *testing.T) {
	resetViper(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "sub", "config.yaml")

	err := WriteDefaultConfig(cfgPath)
	require.NoError(t, err)

	// File should exist.
	_, err = os.Stat(cfgPath)
	assert.NoError(t, err)

	// File should contain expected keys.
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "library_path")
	assert.Contains(t, string(data), "audible_cli_path")
}

func TestWriteDefaultConfig_BadPath(t *testing.T) {
	resetViper(t)
	// /dev/null is not a directory, so MkdirAll should fail
	err := WriteDefaultConfig("/dev/null/sub/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating config directory")
}

func TestValidateLibraryPathNotDir(t *testing.T) {
	resetViper(t)
	SetDefaults()
	// Create a regular file and set it as library_path
	tmpDir := t.TempDir()
	fPath := filepath.Join(tmpDir, "not-a-dir")
	require.NoError(t, os.WriteFile(fPath, []byte("data"), 0644))
	viper.Set("library_path", fPath)
	err := Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not a directory")
}

func TestValidateNegativeRateLimit(t *testing.T) {
	resetViper(t)
	SetDefaults()
	viper.Set("download.rate_limit_seconds", -1)
	err := Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate_limit_seconds")
}

func TestValidateNegativeMaxRetries(t *testing.T) {
	resetViper(t)
	SetDefaults()
	viper.Set("download.max_retries", -1)
	err := Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max_retries")
}

func TestValidateNegativeTimeout(t *testing.T) {
	resetViper(t)
	SetDefaults()
	viper.Set("download.timeout_minutes", -1)
	err := Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout_minutes")
}

func TestInitConfigBadFile(t *testing.T) {
	resetViper(t)
	// Point to a file that doesn't exist and has invalid extension
	// Viper errors when the config file is set explicitly but can't be read
	err := InitConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestDefaultStagingPath(t *testing.T) {
	p, err := DefaultStagingPath()
	require.NoError(t, err)
	assert.Contains(t, p, filepath.Join(".config", "earworm", "staging"))
}
