package audiobookshelf

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

// SessionPageSize is how many playback sessions are requested per page.
const SessionPageSize = 200

// itemBatchSize bounds how many library items are enriched per request.
const itemBatchSize = 100

// LenientSeconds decodes a duration in seconds that the server may send as a
// JSON number or as a string.
//
// Audiobookshelf declares these columns as integers but has been observed
// returning fractional values, and its own code defensively handles string
// values. A plain int64 field would fail the whole page decode on one such row.
type LenientSeconds float64

// UnmarshalJSON accepts numbers, quoted numbers, and null.
func (s *LenientSeconds) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "null" || trimmed == `""` || trimmed == "" {
		*s = 0
		return nil
	}
	trimmed = strings.Trim(trimmed, `"`)
	f, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", trimmed, err)
	}
	*s = LenientSeconds(f)
	return nil
}

// Seconds returns the value as a float.
func (s LenientSeconds) Seconds() float64 { return float64(s) }

// Int returns the value rounded to whole seconds.
func (s LenientSeconds) Int() int { return int(float64(s) + 0.5) }

// EpochMillis is a timestamp the server sends as epoch milliseconds.
type EpochMillis int64

// Time converts to a time.Time in UTC. A zero value yields the zero time so
// callers can distinguish "absent" from "the epoch".
func (e EpochMillis) Time() time.Time {
	if e == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(e)).UTC()
}

// SessionMetadata is the snapshot of book metadata taken when a session was
// recorded. Its genres and series are frequently empty even when the library
// item has them, so it is not a substitute for fetching the item.
type SessionMetadata struct {
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	ASIN        string   `json:"asin"`
	ISBN        string   `json:"isbn"`
	Genres      []string `json:"genres"`
	AuthorName  string   `json:"authorName"`
	PublishYear string   `json:"publishedYear"`
}

// PlaybackSession is one recorded listening session.
type PlaybackSession struct {
	ID            string          `json:"id"`
	UserID        string          `json:"userId"`
	LibraryItemID string          `json:"libraryItemId"`
	BookID        string          `json:"bookId"`
	EpisodeID     string          `json:"episodeId"`
	MediaType     string          `json:"mediaType"`
	DisplayTitle  string          `json:"displayTitle"`
	DisplayAuthor string          `json:"displayAuthor"`
	Metadata      SessionMetadata `json:"mediaMetadata"`
	Duration      LenientSeconds  `json:"duration"`
	TimeListening LenientSeconds  `json:"timeListening"`
	StartTime     LenientSeconds  `json:"startTime"`
	CurrentTime   LenientSeconds  `json:"currentTime"`
	StartedAt     EpochMillis     `json:"startedAt"`
	UpdatedAt     EpochMillis     `json:"updatedAt"`
	DeviceInfo    *struct {
		DeviceType  string `json:"deviceType"`
		ClientName  string `json:"clientName"`
		BrowserName string `json:"browserName"`
		OSName      string `json:"osName"`
	} `json:"deviceInfo"`
	// ServerDate is the day string the server computed in its own timezone.
	// Recorded for reference only: day bucketing derives from StartedAt so it
	// does not depend on where the server happens to run.
	ServerDate string `json:"date"`
}

// DeviceDescription renders a short human-readable device label.
func (s PlaybackSession) DeviceDescription() string {
	if s.DeviceInfo == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, p := range []string{s.DeviceInfo.ClientName, s.DeviceInfo.OSName, s.DeviceInfo.BrowserName} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " / ")
}

// User is the authenticated account.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Type     string `json:"type"`
}

// MediaProgress is the current progress state for one item.
type MediaProgress struct {
	LibraryItemID string         `json:"libraryItemId"`
	EpisodeID     string         `json:"episodeId"`
	Duration      LenientSeconds `json:"duration"`
	Progress      float64        `json:"progress"`
	CurrentTime   LenientSeconds `json:"currentTime"`
	IsFinished    bool           `json:"isFinished"`
	LastUpdate    EpochMillis    `json:"lastUpdate"`
	StartedAt     EpochMillis    `json:"startedAt"`
	FinishedAt    EpochMillis    `json:"finishedAt"`
}

// ServerStatus is the unauthenticated server description.
type ServerStatus struct {
	App           string `json:"app"`
	ServerVersion string `json:"serverVersion"`
	IsInit        bool   `json:"isInit"`
}

// Status returns the server's version banner. It requires no authentication,
// so it distinguishes "server unreachable" from "credentials rejected".
func (c *Client) Status(ctx context.Context) (ServerStatus, error) {
	var out ServerStatus
	body, err := c.get(ctx, "/status", nil, false)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("decode server status: %w", err)
	}
	return out, nil
}

// Me returns the authenticated user together with its media progress.
func (c *Client) Me(ctx context.Context) (User, []MediaProgress, error) {
	body, err := c.get(ctx, "/api/me", nil, true)
	if err != nil {
		return User{}, nil, err
	}
	var resp struct {
		User
		MediaProgress []MediaProgress `json:"mediaProgress"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return User{}, nil, fmt.Errorf("decode user: %w", err)
	}
	return resp.User, resp.MediaProgress, nil
}

// sessionPage is one page of the paginated sessions response.
type sessionPage struct {
	Total        int               `json:"total"`
	NumPages     int               `json:"numPages"`
	Page         int               `json:"page"`
	ItemsPerPage int               `json:"itemsPerPage"`
	Sessions     []PlaybackSession `json:"sessions"`
}

// SessionsOptions controls a session fetch.
type SessionsOptions struct {
	// UserID scopes the admin endpoint to one user. Required for that endpoint.
	UserID string
	// Since stops paging once sessions older than this are reached. The zero
	// value fetches everything.
	Since time.Time
	// MaxPages bounds the walk. Zero means unbounded.
	MaxPages int
}

// ListSessions returns playback sessions newest-first.
//
// It uses the admin /api/sessions endpoint, which paginates server-side. The
// per-user endpoint loads the account's entire session table into memory on
// every page request, which makes a backfill quadratic on a large history.
//
// When Since is set, paging stops at the first page whose sessions are all
// older, rather than at the first old session: a page is ordered by update
// time, but a session updated long after it started can sit among older ones.
func (c *Client) ListSessions(ctx context.Context, opts SessionsOptions) ([]PlaybackSession, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("audiobookshelf URL is not configured")
	}

	var all []PlaybackSession
	seen := make(map[string]bool)

	for page := 0; opts.MaxPages == 0 || page < opts.MaxPages; page++ {
		q := url.Values{}
		q.Set("itemsPerPage", strconv.Itoa(SessionPageSize))
		q.Set("page", strconv.Itoa(page))
		q.Set("sort", "updatedAt")
		q.Set("desc", "1")
		if opts.UserID != "" {
			q.Set("user", opts.UserID)
		}

		body, err := c.get(ctx, "/api/sessions", q, true)
		if err != nil {
			return nil, err
		}

		var p sessionPage
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, fmt.Errorf("decode sessions page %d: %w", page, err)
		}
		if len(p.Sessions) == 0 {
			break
		}

		pageHasNew := false
		for _, s := range p.Sessions {
			if s.ID == "" || seen[s.ID] {
				continue
			}
			if !opts.Since.IsZero() && s.UpdatedAt.Time().Before(opts.Since) {
				continue
			}
			seen[s.ID] = true
			all = append(all, s)
			pageHasNew = true
		}

		if !opts.Since.IsZero() && !pageHasNew {
			break
		}
		if p.NumPages > 0 && page+1 >= p.NumPages {
			break
		}
		if len(p.Sessions) < SessionPageSize {
			break
		}
	}

	return all, nil
}

// LibraryItem is the subset of an expanded library item this package consumes.
type LibraryItem struct {
	ID    string `json:"id"`
	Media struct {
		Metadata struct {
			Title      string `json:"title"`
			ASIN       string `json:"asin"`
			AuthorName string `json:"authorName"`
			Authors    []struct {
				Name string `json:"name"`
			} `json:"authors"`
			NarratorName string   `json:"narratorName"`
			Narrators    []string `json:"narrators"`
			Genres       []string `json:"genres"`
			Series       []struct {
				Name     string `json:"name"`
				Sequence string `json:"sequence"`
			} `json:"series"`
			PublishedYear string `json:"publishedYear"`
		} `json:"metadata"`
		Duration LenientSeconds `json:"duration"`
	} `json:"media"`
}

// AuthorDisplay returns the best available author string.
func (i LibraryItem) AuthorDisplay() string {
	if i.Media.Metadata.AuthorName != "" {
		return i.Media.Metadata.AuthorName
	}
	names := make([]string, 0, len(i.Media.Metadata.Authors))
	for _, a := range i.Media.Metadata.Authors {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// NarratorDisplay returns the best available narrator string.
func (i LibraryItem) NarratorDisplay() string {
	if i.Media.Metadata.NarratorName != "" {
		return i.Media.Metadata.NarratorName
	}
	return strings.Join(i.Media.Metadata.Narrators, ", ")
}

// GetItems fetches library items in batches.
//
// Session metadata snapshots routinely omit genres and series, so enrichment
// has to come from the items themselves. Items that no longer exist are
// skipped rather than failing the batch: a session can outlive its item.
func (c *Client) GetItems(ctx context.Context, ids []string) (map[string]LibraryItem, error) {
	out := make(map[string]LibraryItem, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	for start := 0; start < len(ids); start += itemBatchSize {
		end := start + itemBatchSize
		if end > len(ids) {
			end = len(ids)
		}

		payload, err := json.Marshal(map[string][]string{"libraryItemIds": ids[start:end]})
		if err != nil {
			return nil, fmt.Errorf("encode item batch: %w", err)
		}

		body, err := c.post(ctx, "/api/items/batch/get", payload)
		if err != nil {
			return nil, err
		}

		var resp struct {
			LibraryItems []LibraryItem `json:"libraryItems"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode item batch: %w", err)
		}
		for _, it := range resp.LibraryItems {
			if it.ID != "" {
				out[it.ID] = it
			}
		}
	}
	return out, nil
}

// get performs an authenticated GET and returns the body.
func (c *Client) get(ctx context.Context, path string, query url.Values, auth bool) ([]byte, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("audiobookshelf URL is not configured")
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("audiobookshelf create request: %w", err)
	}
	if auth {
		if c.Token == "" {
			return nil, fmt.Errorf("audiobookshelf API token is not configured")
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("audiobookshelf request %s: %w", path, err)
	}
	defer resp.Body.Close()

	return readAPIResponse(resp, path)
}

// post performs an authenticated JSON POST and returns the body.
func (c *Client) post(ctx context.Context, path string, payload []byte) ([]byte, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("audiobookshelf URL is not configured")
	}
	if c.Token == "" {
		return nil, fmt.Errorf("audiobookshelf API token is not configured")
	}

	endpoint := strings.TrimRight(c.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("audiobookshelf create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("audiobookshelf request %s: %w", path, err)
	}
	defer resp.Body.Close()

	return readAPIResponse(resp, path)
}

// readAPIResponse turns a response into a body or a descriptive error.
//
// A 404 on an admin endpoint means the account lacks permission rather than
// that the path is wrong, which is worth saying out loud: Audiobookshelf
// returns 404 instead of 403 there, and the default message sends people
// looking for a typo that does not exist.
func readAPIResponse(resp *http.Response, path string) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("audiobookshelf read response %s: %w", path, err)
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return body, nil
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("audiobookshelf rejected the API token (401) for %s", path)
	case resp.StatusCode == http.StatusNotFound && strings.HasPrefix(path, "/api/sessions"):
		return nil, fmt.Errorf(
			"audiobookshelf returned 404 for %s; this endpoint requires an admin account, "+
				"and non-admin users receive 404 rather than 403", path)
	default:
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("audiobookshelf %s returned status %d: %s", path, resp.StatusCode, snippet)
	}
}
