package audiobookshelf

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanLibrary_SendsCorrectRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.URL, "my-secret-token", "lib-123")
	err := c.ScanLibrary()

	require.NoError(t, err)
	assert.Equal(t, "POST", gotMethod)
	assert.Equal(t, "/api/libraries/lib-123/scan", gotPath)
	assert.Equal(t, "Bearer my-secret-token", gotAuth)
}

func TestScanLibrary_Returns403Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := NewClient(server.URL, "bad-token", "lib-123")
	err := c.ScanLibrary()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 403")
}

func TestScanLibrary_Returns500Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", "lib-123")
	err := c.ScanLibrary()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestScanLibrary_UnreachableServer(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "token", "lib-123")
	err := c.ScanLibrary()

	require.Error(t, err)
}

func TestScanLibrary_EmptyBaseURL_SilentSkip(t *testing.T) {
	c := NewClient("", "token", "lib-123")
	err := c.ScanLibrary()

	assert.NoError(t, err)
}
