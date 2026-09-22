// Package komga reads reading progress from a Komga comic, manga and ebook
// server.
//
// Komga stores one progress record per book, overwritten in place, with no
// history of reading events. That is the same shape as Audible rather than
// Audiobookshelf. It matters much less here: a book in Komga is a single
// volume or chapter, so a series read through produces one dated completion
// per volume, which is already a dense timeline without any reconstruction.
package komga

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PageSize is how many books are requested per page.
const PageSize = 500

// requestTimeout bounds a single API call.
const requestTimeout = 60 * time.Second

// Client talks to a Komga server.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a Komga client. Komga authenticates the REST API with an
// API key in X-API-Key, generated from the account settings page.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: requestTimeout},
	}
}

// ReadProgress is Komga's per-book progress record.
//
// Created and ReadDate bracket the reading: Created is when progress was first
// written, ReadDate when it was last written. For a volume read in one sitting
// the two are minutes apart. They are frequently identical, because a reader
// that only reports completion writes progress exactly once.
type ReadProgress struct {
	Page         int       `json:"page"`
	Completed    bool      `json:"completed"`
	ReadDate     time.Time `json:"readDate"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	DeviceID     string    `json:"deviceId"`
	DeviceName   string    `json:"deviceName"`
}

// Book is one volume, chapter or ebook, with the caller's progress on it.
type Book struct {
	ID          string        `json:"id"`
	SeriesID    string        `json:"seriesId"`
	SeriesTitle string        `json:"seriesTitle"`
	Name        string        `json:"name"`
	Number      float64       `json:"number"`
	Oneshot     bool          `json:"oneshot"`
	Progress    *ReadProgress `json:"readProgress"`
	Metadata    struct {
		Title      string  `json:"title"`
		Number     string  `json:"number"`
		NumberSort float64 `json:"numberSort"`
		Summary    string  `json:"summary"`
		ISBN       string  `json:"isbn"`
		Authors    []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"authors"`
		Tags        []string `json:"tags"`
		ReleaseDate string   `json:"releaseDate"`
	} `json:"metadata"`
	Media struct {
		MediaType  string `json:"mediaType"`
		PagesCount int    `json:"pagesCount"`
		Profile    string `json:"mediaProfile"`
	} `json:"media"`
}

// DisplayTitle returns the most useful title available.
func (b Book) DisplayTitle() string {
	if b.Metadata.Title != "" {
		return b.Metadata.Title
	}
	return b.Name
}

// AuthorDisplay joins the credited authors, preferring writers where the role
// is given, since a comic credits artists and letterers alongside the writer.
func (b Book) AuthorDisplay() string {
	var writers, others []string
	for _, a := range b.Metadata.Authors {
		if a.Name == "" {
			continue
		}
		if strings.EqualFold(a.Role, "writer") {
			writers = append(writers, a.Name)
		} else {
			others = append(others, a.Name)
		}
	}
	if len(writers) > 0 {
		return strings.Join(writers, ", ")
	}
	return strings.Join(others, ", ")
}

// VolumeNumber renders the volume or chapter number as stored.
func (b Book) VolumeNumber() string {
	if b.Metadata.Number != "" {
		return b.Metadata.Number
	}
	if b.Number > 0 {
		return strconv.FormatFloat(b.Number, 'f', -1, 64)
	}
	return ""
}

// CompletedAt returns the best available completion timestamp.
func (b Book) CompletedAt() time.Time {
	if b.Progress == nil {
		return time.Time{}
	}
	if !b.Progress.ReadDate.IsZero() {
		return b.Progress.ReadDate
	}
	return b.Progress.LastModified
}

// ReadingSpan returns how long the book was open, from the first progress
// write to the last.
//
// This is not reading time. It is zero whenever a reader reports progress only
// once, which is the common case, so it is recorded rather than relied upon.
func (b Book) ReadingSpan() time.Duration {
	if b.Progress == nil || b.Progress.Created.IsZero() || b.Progress.ReadDate.IsZero() {
		return 0
	}
	d := b.Progress.ReadDate.Sub(b.Progress.Created)
	if d < 0 {
		return 0
	}
	return d
}

type bookPage struct {
	Content       []Book `json:"content"`
	TotalElements int    `json:"totalElements"`
	Last          bool   `json:"last"`
	Number        int    `json:"number"`
}

// ReadStatus values accepted by the books endpoint.
const (
	StatusRead       = "READ"
	StatusInProgress = "IN_PROGRESS"
	StatusUnread     = "UNREAD"
)

// BooksWithProgress returns every book the account has read or started.
//
// Unread books are deliberately excluded: a library holds thousands of them
// and none carry a reading record.
func (c *Client) BooksWithProgress(ctx context.Context) ([]Book, error) {
	var all []Book
	seen := make(map[string]bool)

	for _, status := range []string{StatusRead, StatusInProgress} {
		books, err := c.booksByStatus(ctx, status)
		if err != nil {
			return nil, err
		}
		for _, b := range books {
			// A book can in principle appear under both filters between pages;
			// keying by id keeps it single.
			if b.ID == "" || seen[b.ID] {
				continue
			}
			seen[b.ID] = true
			all = append(all, b)
		}
	}
	return all, nil
}

func (c *Client) booksByStatus(ctx context.Context, status string) ([]Book, error) {
	var out []Book

	for page := 0; ; page++ {
		q := url.Values{}
		q.Set("read_status", status)
		q.Set("size", strconv.Itoa(PageSize))
		q.Set("page", strconv.Itoa(page))

		body, err := c.get(ctx, "/api/v1/books", q)
		if err != nil {
			return nil, err
		}

		var p bookPage
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, fmt.Errorf("decode komga books page %d: %w", page, err)
		}
		out = append(out, p.Content...)

		if p.Last || len(p.Content) == 0 {
			break
		}
		// Defensive stop: a server that never sets last would page forever.
		if page > 1000 {
			break
		}
	}
	return out, nil
}

// ServerInfo is a minimal reachability probe result.
type ServerInfo struct {
	Reachable     bool
	Authenticated bool
	TotalBooks    int
}

// Check reports whether the server answers and the API key is accepted.
//
// Reachability and authentication are reported separately so a wrong address
// and a wrong key are distinguishable.
func (c *Client) Check(ctx context.Context) (ServerInfo, error) {
	var info ServerInfo
	if c.BaseURL == "" {
		return info, fmt.Errorf("komga.url is not configured")
	}

	body, err := c.get(ctx, "/api/v1/books", url.Values{"size": []string{"1"}})
	if err != nil {
		var authErr *AuthError
		if ok := asAuthError(err, &authErr); ok {
			info.Reachable = true
			return info, err
		}
		return info, err
	}

	var p bookPage
	if err := json.Unmarshal(body, &p); err != nil {
		return info, fmt.Errorf("decode komga check response: %w", err)
	}
	info.Reachable = true
	info.Authenticated = true
	info.TotalBooks = p.TotalElements
	return info, nil
}

// AuthError indicates the server answered but rejected the credentials.
type AuthError struct {
	StatusCode int
	Path       string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("komga rejected the API key (%d) for %s", e.StatusCode, e.Path)
}

func asAuthError(err error, target **AuthError) bool {
	if ae, ok := err.(*AuthError); ok {
		*target = ae
		return true
	}
	return false
}

func (c *Client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("komga.url is not configured")
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("komga.api_key is not configured")
	}

	endpoint := c.BaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("komga create request: %w", err)
	}
	req.Header.Set("X-API-Key", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("komga request %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("komga read response %s: %w", path, err)
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return body, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &AuthError{StatusCode: resp.StatusCode, Path: path}
	default:
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("komga %s returned status %d: %s", path, resp.StatusCode, snippet)
	}
}
