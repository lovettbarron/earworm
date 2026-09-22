package komga

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeServer serves /api/v1/books from per-status page lists.
type fakeServer struct {
	mu       sync.Mutex
	pages    map[string][]bookPage // read_status -> pages
	status   int                   // non-zero forces this response status
	body     string
	requests []*http.Request
	apiKeys  []string
}

func (f *fakeServer) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	f.apiKeys = append(f.apiKeys, r.Header.Get("X-API-Key"))

	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
		return
	}
	if r.URL.Path != "/api/v1/books" {
		http.NotFound(w, r)
		return
	}
	if f.body != "" {
		_, _ = w.Write([]byte(f.body))
		return
	}

	pages := f.pages[r.URL.Query().Get("read_status")]
	n, _ := strconv.Atoi(r.URL.Query().Get("page"))
	var p bookPage
	if n < len(pages) {
		p = pages[n]
	}
	_ = json.NewEncoder(w).Encode(p)
}

func newTestClient(t *testing.T, f *fakeServer) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL+"/", "secret-key")
}

func book(id string) Book {
	return Book{ID: id, Progress: &ReadProgress{Completed: true}}
}

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	c := NewClient("http://example.test///", "k")
	assert.Equal(t, "http://example.test", c.BaseURL)
	assert.Equal(t, "k", c.APIKey)
	require.NotNil(t, c.HTTPClient)
	assert.Equal(t, requestTimeout, c.HTTPClient.Timeout)
}

func TestBooksWithProgressPagesBothStatusesAndDedupes(t *testing.T) {
	f := &fakeServer{pages: map[string][]bookPage{
		StatusRead: {
			{Content: []Book{book("a"), book("b")}, Last: false},
			{Content: []Book{book("c")}, Last: true},
		},
		StatusInProgress: {
			// "b" also appears here and must not be returned twice; the
			// empty id is dropped.
			{Content: []Book{book("b"), book("d"), {ID: ""}}, Last: true},
		},
	}}
	c := newTestClient(t, f)

	books, err := c.BooksWithProgress(context.Background())
	require.NoError(t, err)

	var ids []string
	for _, b := range books {
		ids = append(ids, b.ID)
	}
	assert.Equal(t, []string{"a", "b", "c", "d"}, ids)

	require.Len(t, f.requests, 3)
	q := f.requests[0].URL.Query()
	assert.Equal(t, StatusRead, q.Get("read_status"))
	assert.Equal(t, strconv.Itoa(PageSize), q.Get("size"))
	assert.Equal(t, "0", q.Get("page"))
	assert.Equal(t, "1", f.requests[1].URL.Query().Get("page"))
	assert.Equal(t, StatusInProgress, f.requests[2].URL.Query().Get("read_status"))

	for _, k := range f.apiKeys {
		assert.Equal(t, "secret-key", k, "every request carries X-API-Key")
	}
}

func TestBooksWithProgressStopsOnEmptyPage(t *testing.T) {
	// No page sets last; an empty page ends the walk.
	f := &fakeServer{pages: map[string][]bookPage{
		StatusRead: {{Content: []Book{book("a")}}},
	}}
	c := newTestClient(t, f)

	books, err := c.BooksWithProgress(context.Background())
	require.NoError(t, err)
	assert.Len(t, books, 1)
	// READ: page 0 (content) + page 1 (empty). IN_PROGRESS: page 0 (empty).
	assert.Len(t, f.requests, 3)
}

func TestBooksWithProgressGuardsAgainstEndlessPaging(t *testing.T) {
	// A server that always returns content and never sets last.
	f := &fakeServer{body: `{"content":[{"id":"same"}],"last":false}`}
	c := newTestClient(t, f)

	books, err := c.BooksWithProgress(context.Background())
	require.NoError(t, err)
	assert.Len(t, books, 1, "repeated id is deduplicated")
	// Pages 0..1001 per status before the guard trips.
	assert.Len(t, f.requests, 2*1002)
}

func TestBooksWithProgressPropagatesErrors(t *testing.T) {
	t.Run("server error", func(t *testing.T) {
		f := &fakeServer{status: http.StatusInternalServerError, body: "boom"}
		_, err := newTestClient(t, f).BooksWithProgress(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
		assert.Contains(t, err.Error(), "boom")
	})
	t.Run("long error body is truncated", func(t *testing.T) {
		long := make([]byte, 500)
		for i := range long {
			long[i] = 'x'
		}
		f := &fakeServer{status: http.StatusBadGateway, body: string(long)}
		_, err := newTestClient(t, f).BooksWithProgress(context.Background())
		require.Error(t, err)
		assert.Less(t, len(err.Error()), 300)
	})
	t.Run("bad json", func(t *testing.T) {
		f := &fakeServer{body: "not json"}
		_, err := newTestClient(t, f).BooksWithProgress(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode komga books page 0")
	})
	t.Run("unauthorized", func(t *testing.T) {
		f := &fakeServer{status: http.StatusUnauthorized}
		_, err := newTestClient(t, f).BooksWithProgress(context.Background())
		var ae *AuthError
		require.ErrorAs(t, err, &ae)
		assert.Equal(t, http.StatusUnauthorized, ae.StatusCode)
	})
	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		_, err := NewClient(srv.URL, "k").BooksWithProgress(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "komga request")
	})
	t.Run("bad url", func(t *testing.T) {
		_, err := NewClient("http://bad host", "k").BooksWithProgress(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "komga create request")
	})
}

func TestMissingConfigurationIsReported(t *testing.T) {
	_, err := NewClient("", "k").BooksWithProgress(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "komga.url is not configured")

	_, err = NewClient("http://example.test", "").BooksWithProgress(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "komga.api_key is not configured")

	_, err = NewClient("", "k").Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "komga.url is not configured")
}

func TestCheckReachableAndAuthenticated(t *testing.T) {
	f := &fakeServer{body: `{"content":[],"totalElements":1234,"last":false}`}
	info, err := newTestClient(t, f).Check(context.Background())
	require.NoError(t, err)
	assert.True(t, info.Reachable)
	assert.True(t, info.Authenticated)
	assert.Equal(t, 1234, info.TotalBooks)
	require.Len(t, f.requests, 1)
	assert.Equal(t, "1", f.requests[0].URL.Query().Get("size"))
}

func TestCheckDistinguishesRejectedKeyFromUnreachable(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			f := &fakeServer{status: code}
			info, err := newTestClient(t, f).Check(context.Background())
			var ae *AuthError
			require.True(t, errors.As(err, &ae))
			assert.Equal(t, code, ae.StatusCode)
			assert.Equal(t, "/api/v1/books", ae.Path)
			assert.Contains(t, ae.Error(), "rejected the API key")
			assert.True(t, info.Reachable, "an auth rejection proves the server answered")
			assert.False(t, info.Authenticated)
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		info, err := NewClient(srv.URL, "k").Check(context.Background())
		require.Error(t, err)
		assert.False(t, info.Reachable)
		assert.False(t, info.Authenticated)
	})

	t.Run("bad json", func(t *testing.T) {
		f := &fakeServer{body: "<html>"}
		info, err := newTestClient(t, f).Check(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode komga check response")
		assert.False(t, info.Authenticated)
	})
}

func TestDisplayTitleAndSeriesAreCleaned(t *testing.T) {
	b := Book{Name: "Gantz v01 (2018) (Digital) (1r0n)", SeriesTitle: "Gantz (2018-2023) (Digital) (1r0n)"}
	assert.Equal(t, "Gantz v01", b.DisplayTitle(), "falls back to the file name")
	assert.Equal(t, "Gantz", b.DisplaySeries())

	b.Metadata.Title = "Volume 1 (Digital)"
	assert.Equal(t, "Volume 1", b.DisplayTitle(), "metadata title wins")
}

func TestAuthorDisplayPrefersWriters(t *testing.T) {
	type author = struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	var b Book
	assert.Equal(t, "", b.AuthorDisplay())

	b.Metadata.Authors = []author{
		{Name: "Penciller", Role: "penciller"},
		{Name: "", Role: "writer"},
		{Name: "Letterer", Role: "letterer"},
	}
	assert.Equal(t, "Penciller, Letterer", b.AuthorDisplay(), "no writer: everyone credited")

	b.Metadata.Authors = append(b.Metadata.Authors,
		author{Name: "Writer One", Role: "Writer"},
		author{Name: "Writer Two", Role: "writer"})
	assert.Equal(t, "Writer One, Writer Two", b.AuthorDisplay())
}

func TestVolumeNumber(t *testing.T) {
	var b Book
	assert.Equal(t, "", b.VolumeNumber())

	b.Number = 3
	assert.Equal(t, "3", b.VolumeNumber())
	b.Number = 10.5
	assert.Equal(t, "10.5", b.VolumeNumber())

	b.Metadata.Number = "10.5a"
	assert.Equal(t, "10.5a", b.VolumeNumber(), "stored metadata number wins")
}

func TestCompletedAt(t *testing.T) {
	read := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	modified := read.Add(time.Hour)

	assert.True(t, Book{}.CompletedAt().IsZero())

	b := Book{Progress: &ReadProgress{ReadDate: read, LastModified: modified}}
	assert.Equal(t, read, b.CompletedAt())

	b.Progress.ReadDate = time.Time{}
	assert.Equal(t, modified, b.CompletedAt(), "falls back to last modified")
}

func TestReadingSpan(t *testing.T) {
	created := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	assert.Zero(t, Book{}.ReadingSpan())
	assert.Zero(t, Book{Progress: &ReadProgress{ReadDate: created}}.ReadingSpan(), "no created")
	assert.Zero(t, Book{Progress: &ReadProgress{Created: created}}.ReadingSpan(), "no read date")

	b := Book{Progress: &ReadProgress{Created: created, ReadDate: created.Add(20 * time.Minute)}}
	assert.Equal(t, 20*time.Minute, b.ReadingSpan())

	b.Progress.ReadDate = created.Add(-time.Minute)
	assert.Zero(t, b.ReadingSpan(), "never negative")
}

func TestBookDecodesKomgaJSON(t *testing.T) {
	raw := `{
		"id":"b1","seriesId":"s1","seriesTitle":"Series","name":"Series v01","number":1,
		"readProgress":{"page":42,"completed":false,"readDate":"2026-05-01T12:00:00Z","created":"2026-05-01T11:30:00Z"},
		"metadata":{"title":"Vol 1","number":"1","authors":[{"name":"W","role":"writer"}],"tags":["action"]},
		"media":{"mediaType":"application/zip","pagesCount":200,"mediaProfile":"DIVINA"}
	}`
	var b Book
	require.NoError(t, json.Unmarshal([]byte(raw), &b))
	require.NotNil(t, b.Progress)
	assert.Equal(t, 42, b.Progress.Page)
	assert.Equal(t, 30*time.Minute, b.ReadingSpan())
	assert.Equal(t, 200, b.Media.PagesCount)
	assert.Equal(t, "W", b.AuthorDisplay())
	assert.Equal(t, []string{"action"}, b.Metadata.Tags)
}
