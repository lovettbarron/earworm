package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifyCommand_Success(t *testing.T) {
	var gotRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequest = true
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/api/libraries/")
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgContent := "audiobookshelf:\n  url: " + server.URL + "\n  token: test-token\n  library_id: lib1\n"
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

	out, err := executeCommand(t, "--config", cfgPath, "notify")
	require.NoError(t, err)
	assert.True(t, gotRequest, "should have made request to mock server")
	assert.Contains(t, out, "scan triggered")
}

func TestNotifyCommand_JSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgContent := "audiobookshelf:\n  url: " + server.URL + "\n  token: test-token\n  library_id: lib1\n"
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

	out, err := executeCommand(t, "--config", cfgPath, "notify", "--json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"ok"`)
}

func TestNotifyCommand_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cfgDir := filepath.Join(tmpHome, ".config", "earworm")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))

	cfgContent := "audiobookshelf:\n  url: " + server.URL + "\n  token: test-token\n  library_id: lib1\n"
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

	_, err := executeCommand(t, "--config", cfgPath, "notify")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan")
}
