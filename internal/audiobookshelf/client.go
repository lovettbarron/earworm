package audiobookshelf

import (
	"fmt"
	"net/http"
	"time"
)

// Client communicates with the Audiobookshelf REST API.
type Client struct {
	BaseURL    string
	Token      string
	LibraryID  string
	HTTPClient *http.Client
}

// NewClient creates a new Audiobookshelf API client.
func NewClient(baseURL, token, libraryID string) *Client {
	return &Client{
		BaseURL:   baseURL,
		Token:     token,
		LibraryID: libraryID,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ScanLibrary triggers a library scan on the configured Audiobookshelf instance.
// If BaseURL is empty, it returns nil immediately (silent skip for unconfigured instances).
func (c *Client) ScanLibrary() error {
	if c.BaseURL == "" {
		return nil
	}

	url := fmt.Sprintf("%s/api/libraries/%s/scan", c.BaseURL, c.LibraryID)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("audiobookshelf create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("audiobookshelf scan request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("audiobookshelf scan returned status %d", resp.StatusCode)
	}

	return nil
}
