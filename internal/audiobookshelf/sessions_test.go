package audiobookshelf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLenientSecondsAcceptsNumberStringAndNull(t *testing.T) {
	// The server declares these as integers but has been observed sending
	// floats, and its own code tolerates strings. A strict int64 field would
	// fail the entire page decode on one such row.
	var payload struct {
		A LenientSeconds `json:"a"`
		B LenientSeconds `json:"b"`
		C LenientSeconds `json:"c"`
		D LenientSeconds `json:"d"`
	}
	raw := `{"a": 2723.0009765625, "b": "1800", "c": null, "d": 42}`
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))

	assert.InDelta(t, 2723.0009765625, payload.A.Seconds(), 0.0001)
	assert.Equal(t, 2723, payload.A.Int())
	assert.Equal(t, 1800, payload.B.Int())
	assert.Equal(t, 0, payload.C.Int())
	assert.Equal(t, 42, payload.D.Int())
}

func TestLenientSecondsRejectsNonNumeric(t *testing.T) {
	var v LenientSeconds
	require.Error(t, json.Unmarshal([]byte(`"abc"`), &v))
}

func TestEpochMillisZeroIsNotTheEpoch(t *testing.T) {
	assert.True(t, EpochMillis(0).Time().IsZero(),
		"a missing timestamp must read as absent, not as 1970")
	assert.Equal(t, 2026, EpochMillis(1789830388367).Time().Year())
}

func TestStatusNeedsNoToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"app":"audiobookshelf","serverVersion":"2.36.0","isInit":true}`)
	}))
	defer server.Close()

	c := NewClient(server.URL, "", "")
	st, err := c.Status(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "2.36.0", st.ServerVersion)
	assert.Empty(t, gotAuth, "status is unauthenticated so it can distinguish unreachable from unauthorized")
}

func TestMeReturnsUserAndProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/me", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"id":"user-1","username":"tester","type":"root",
			"mediaProgress":[{"libraryItemId":"item-1","duration":3600,"progress":0.5,
			"currentTime":1800,"isFinished":false,"startedAt":1789830388367,"finishedAt":0}]}`)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token", "")
	user, progress, err := c.Me(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "user-1", user.ID)
	assert.Equal(t, "tester", user.Username)
	require.Len(t, progress, 1)
	assert.Equal(t, "item-1", progress[0].LibraryItemID)
	assert.True(t, progress[0].FinishedAt.Time().IsZero())
}

func TestMeRequiresToken(t *testing.T) {
	c := NewClient("http://example.invalid", "", "")
	_, _, err := c.Me(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token is not configured")
}

// sessionServer serves a paginated session list.
func sessionServer(t *testing.T, total int, requests *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			*requests = append(*requests, r.URL.String())
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("itemsPerPage"))
		if perPage == 0 {
			perPage = SessionPageSize
		}

		start := page * perPage
		numPages := (total + perPage - 1) / perPage

		var sessions []map[string]any
		for i := start; i < start+perPage && i < total; i++ {
			sessions = append(sessions, map[string]any{
				"id":            fmt.Sprintf("session-%04d", i),
				"userId":        "user-1",
				"libraryItemId": fmt.Sprintf("item-%02d", i%5),
				"mediaType":     "book",
				"displayTitle":  "A Test Title",
				"displayAuthor": "An Author",
				"timeListening": 600.5,
				"duration":      3600,
				"startedAt":     time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour).UnixMilli(),
				"updatedAt":     time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour).UnixMilli(),
				"mediaMetadata": map[string]any{"title": "A Test Title", "asin": ""},
			})
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"total": total, "numPages": numPages, "page": page,
			"itemsPerPage": perPage, "sessions": sessions,
		})
	}))
}

func TestListSessionsPagesThroughAll(t *testing.T) {
	var requests []string
	server := sessionServer(t, 450, &requests)
	defer server.Close()

	c := NewClient(server.URL, "test-token", "")
	got, err := c.ListSessions(context.Background(), SessionsOptions{UserID: "user-1"})

	require.NoError(t, err)
	assert.Len(t, got, 450)
	assert.GreaterOrEqual(t, len(requests), 3, "450 sessions at 200 per page needs 3 pages")

	// The admin endpoint must be used and scoped to the user.
	assert.Contains(t, requests[0], "/api/sessions")
	assert.Contains(t, requests[0], "user=user-1")
}

func TestListSessionsDecodesFloatTimeListening(t *testing.T) {
	server := sessionServer(t, 1, nil)
	defer server.Close()

	c := NewClient(server.URL, "test-token", "")
	got, err := c.ListSessions(context.Background(), SessionsOptions{UserID: "user-1"})

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 601, got[0].TimeListening.Int(), "600.5 rounds to 601")
}

func TestListSessionsStopsAtSinceWatermark(t *testing.T) {
	var requests []string
	server := sessionServer(t, 450, &requests)
	defer server.Close()

	// Sessions are generated one hour apart descending, so a recent cutoff
	// should stop paging early rather than walking the whole history.
	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	c := NewClient(server.URL, "test-token", "")
	got, err := c.ListSessions(context.Background(), SessionsOptions{UserID: "user-1", Since: since})

	require.NoError(t, err)
	assert.Less(t, len(got), 450, "a watermark should limit what is returned")
	assert.Less(t, len(requests), 3, "paging should stop once a page yields nothing new")
	for _, s := range got {
		assert.False(t, s.UpdatedAt.Time().Before(since))
	}
}

func TestListSessionsDeduplicatesByID(t *testing.T) {
	// A server that returns the same page forever must not produce duplicates
	// or loop indefinitely.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total": 1000, "numPages": 10, "page": 0, "itemsPerPage": SessionPageSize,
			"sessions": []map[string]any{{
				"id": "same-session", "timeListening": 60,
				"startedAt": time.Now().UnixMilli(), "updatedAt": time.Now().UnixMilli(),
			}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token", "")
	got, err := c.ListSessions(context.Background(), SessionsOptions{UserID: "user-1", MaxPages: 5})

	require.NoError(t, err)
	assert.Len(t, got, 1, "a repeated session id must be stored once")
}

func TestListSessionsRequiresURL(t *testing.T) {
	c := NewClient("", "token", "")
	_, err := c.ListSessions(context.Background(), SessionsOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "URL is not configured")
}

// Audiobookshelf answers 404 rather than 403 for admin endpoints a user cannot
// reach, which otherwise sends people hunting for a path typo.
func TestListSessionsExplainsAdminOnly404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token", "")
	_, err := c.ListSessions(context.Background(), SessionsOptions{UserID: "user-1"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin account")
}

func TestListSessionsReportsRejectedToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewClient(server.URL, "bad-token", "")
	_, err := c.ListSessions(context.Background(), SessionsOptions{UserID: "user-1"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rejected the API token")
}

func TestGetItemsBatchesAndMaps(t *testing.T) {
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/items/batch/get", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var body struct {
			IDs []string `json:"libraryItemIds"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		batchSizes = append(batchSizes, len(body.IDs))

		var items []map[string]any
		for _, id := range body.IDs {
			items = append(items, map[string]any{
				"id": id,
				"media": map[string]any{
					"duration": 3600,
					"metadata": map[string]any{
						"title": "A Test Title", "asin": "SYNTH00001",
						"authorName": "An Author", "genres": []string{"Fantasy"},
						"series": []map[string]any{{"name": "A Series", "sequence": "2"}},
					},
				},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"libraryItems": items})
	}))
	defer server.Close()

	ids := make([]string, 250)
	for i := range ids {
		ids[i] = fmt.Sprintf("item-%03d", i)
	}

	c := NewClient(server.URL, "test-token", "")
	got, err := c.GetItems(context.Background(), ids)

	require.NoError(t, err)
	assert.Len(t, got, 250)
	assert.Len(t, batchSizes, 3, "250 items should batch into 3 requests")
	for _, n := range batchSizes {
		assert.LessOrEqual(t, n, itemBatchSize)
	}

	item := got["item-000"]
	assert.Equal(t, "A Test Title", item.Media.Metadata.Title)
	assert.Equal(t, "An Author", item.AuthorDisplay())
	assert.Equal(t, []string{"Fantasy"}, item.Media.Metadata.Genres)
}

func TestGetItemsEmptyIsNoop(t *testing.T) {
	c := NewClient("http://example.invalid", "token", "")
	got, err := c.GetItems(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// A session can outlive the item it refers to; a missing item must not fail
// the whole enrichment pass.
func TestGetItemsToleratesMissingItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"libraryItems": []map[string]any{
			{"id": "item-001", "media": map[string]any{"metadata": map[string]any{"title": "Present"}}},
		}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token", "")
	got, err := c.GetItems(context.Background(), []string{"item-001", "item-gone"})

	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.NotContains(t, got, "item-gone")
}

func TestDeviceDescription(t *testing.T) {
	var s PlaybackSession
	assert.Empty(t, s.DeviceDescription(), "a session without device info yields no label")

	raw := `{"deviceInfo":{"clientName":"Abs Web","osName":"macOS","browserName":"Safari"}}`
	require.NoError(t, json.Unmarshal([]byte(raw), &s))
	assert.Equal(t, "Abs Web / macOS / Safari", s.DeviceDescription())
}

func TestNarratorDisplayPrefersNameField(t *testing.T) {
	var it LibraryItem
	raw := `{"media":{"metadata":{"narratorName":"A Narrator","narrators":["Ignored"]}}}`
	require.NoError(t, json.Unmarshal([]byte(raw), &it))
	assert.Equal(t, "A Narrator", it.NarratorDisplay())

	var it2 LibraryItem
	raw2 := `{"media":{"metadata":{"narrators":["One","Two"]}}}`
	require.NoError(t, json.Unmarshal([]byte(raw2), &it2))
	assert.Equal(t, "One, Two", it2.NarratorDisplay())
}
