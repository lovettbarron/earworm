package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/komga"
	"github.com/lovettbarron/earworm/internal/listening"
	"github.com/lovettbarron/earworm/internal/stats"
)

// fakeKomgaSource is a scripted stats.KomgaSource.
type fakeKomgaSource struct {
	books []komga.Book
	err   error
}

func (f *fakeKomgaSource) BooksWithProgress(context.Context) ([]komga.Book, error) {
	return f.books, f.err
}

func withKomgaFakes(t *testing.T, src *fakeKomgaSource) {
	t.Helper()
	orig := newKomgaClient
	newKomgaClient = func() stats.KomgaSource { return src }
	t.Cleanup(func() { newKomgaClient = orig })
}

func setKomgaConfig() {
	setStatsConfig()
	viper.Set("komga.url", "http://komga.invalid:25600")
	viper.Set("komga.api_key", "test-key")
}

func komgaTestBook(id, series string, completed bool, readAt time.Time) komga.Book {
	b := komga.Book{
		ID:          id,
		SeriesTitle: series,
		Name:        series + " " + id,
		Progress:    &komga.ReadProgress{Page: 10, Completed: completed, ReadDate: readAt, Created: readAt},
	}
	b.Media.PagesCount = 20
	return b
}

func sampleKomgaSource() *fakeKomgaSource {
	return &fakeKomgaSource{books: []komga.Book{
		komgaTestBook("k1", "Old Series", true, time.Date(2026, 5, 31, 23, 0, 0, 0, time.UTC)),
		komgaTestBook("k2", "New Series", true, time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)),
		komgaTestBook("k3", "New Series", false, time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)),
	}}
}

func TestStatsSyncKomgaStoresReading(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	withKomgaFakes(t, sampleKomgaSource())

	out, err := executeCommandWithConfig(t, setKomgaConfig, "stats", "sync", "--source", "komga", "--json")
	require.NoError(t, err)

	var res stats.KomgaResult
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, stats.KomgaResult{Books: 3, Completed: 2, InProgress: 1, Series: 2, Days: 3}, res)

	books, err := db.ListBookListening(database, listening.SourceKomga)
	require.NoError(t, err)
	require.Len(t, books, 3)
	assert.Equal(t, "Old Series", books[0].Series)
}

func TestStatsBackfillKomgaIsASync(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	withKomgaFakes(t, sampleKomgaSource())

	_, err := executeCommandWithConfig(t, setKomgaConfig, "stats", "backfill", "--source", "komga", "--json")
	require.NoError(t, err)

	books, err := db.ListBookListening(database, listening.SourceKomga)
	require.NoError(t, err)
	assert.Len(t, books, 3)
}

func TestStatsSyncKomgaTextOutputReportsUnreliable(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withKomgaFakes(t, sampleKomgaSource())

	out, err := executeCommandWithConfig(t, func() {
		setKomgaConfig()
		viper.Set("komga.unreliable_before", "2026-05-31")
	}, "stats", "sync", "--source", "komga")
	require.NoError(t, err)

	assert.Contains(t, out, "Reading history (komga):")
	assert.Contains(t, out, "3 across 2 series")
	assert.Contains(t, out, "Completed:     2")
	assert.Contains(t, out, "In progress:   1")
	assert.Contains(t, out, "Reading days:  2", "the flagged completion adds no day")
	assert.Contains(t, out, "Unreliable:    1 books", "the whole named cutoff day is included")
}

func TestStatsSyncKomgaQuietPrintsNothing(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withKomgaFakes(t, sampleKomgaSource())

	out, err := executeCommandWithConfig(t, setKomgaConfig, "stats", "sync", "--source", "komga", "--quiet")
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestStatsKomgaConfigValidation(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
		want  string
	}{
		{"missing url", func() { setKomgaConfig(); viper.Set("komga.url", "") }, "komga.url is not configured"},
		{"missing key", func() { setKomgaConfig(); viper.Set("komga.api_key", "") }, "komga.api_key is not configured"},
		{"bad cutoff", func() { setKomgaConfig(); viper.Set("komga.unreliable_before", "31/05/2026") }, "komga.unreliable_before"},
		{"bad timezone", func() { setKomgaConfig(); viper.Set("stats.timezone", "Not/AZone") }, "stats.timezone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStatsFakes(t, &fakeCLIStatsClient{})
			withKomgaFakes(t, sampleKomgaSource())

			_, err := executeCommandWithConfig(t, tt.setup, "stats", "sync", "--source", "komga")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestStatsSyncKomgaPropagatesClientError(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withKomgaFakes(t, &fakeKomgaSource{err: fmt.Errorf("connection refused")})

	_, err := executeCommandWithConfig(t, setKomgaConfig, "stats", "sync", "--source", "komga")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestStatsStatusReportsKomgaInBooks(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withKomgaFakes(t, sampleKomgaSource())

	_, err := executeCommandWithConfig(t, func() {
		setKomgaConfig()
		viper.Set("komga.unreliable_before", "2026-05-31")
	}, "stats", "sync", "--source", "komga", "--quiet")
	require.NoError(t, err)

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "status")
	require.NoError(t, err)

	assert.Contains(t, out, "komga    3 books read or started")
	assert.Contains(t, out, "1 flagged as library-migration artifacts")
	assert.NotContains(t, out, "komga    0 days", "reading is not reported as listening time")
	assert.Contains(t, out, "audible  no data")
}

func TestStatsStatusKomgaNoData(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "komga    no data")
}
