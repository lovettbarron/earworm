package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

// fakeCLIStatsClient is a minimal scripted audible.StatsClient for CLI tests.
type fakeCLIStatsClient struct {
	daily     map[string]float64
	library   []audible.LibraryListening
	finished  []audible.FinishedStatus
	positions map[string]audible.LastPosition
	err       error
}

func (f *fakeCLIStatsClient) APIGet(context.Context, string, ...audible.APIParam) ([]byte, error) {
	return nil, fmt.Errorf("not used")
}
func (f *fakeCLIStatsClient) DailyListening(context.Context, time.Time, int) (map[string]float64, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.daily, nil
}
func (f *fakeCLIStatsClient) MonthlyListening(context.Context, time.Time, int) (map[string]float64, error) {
	return map[string]float64{}, nil
}
func (f *fakeCLIStatsClient) FinishedStatuses(context.Context) ([]audible.FinishedStatus, error) {
	return f.finished, nil
}
func (f *fakeCLIStatsClient) LibraryListening(context.Context) ([]audible.LibraryListening, error) {
	return f.library, nil
}
func (f *fakeCLIStatsClient) LastPositions(context.Context, []string) (map[string]audible.LastPosition, error) {
	if f.positions == nil {
		return map[string]audible.LastPosition{}, nil
	}
	return f.positions, nil
}

// withStatsFakes points the stats commands at a temporary database and a
// scripted client for the duration of one test.
//
// The database is file-backed rather than in-memory because each command run
// closes the handle it was given; an in-memory database would vanish after the
// first command, taking the rows the test wants to assert on with it.
func withStatsFakes(t *testing.T, client audible.StatsClient) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "stats-test.db")

	origDB, origClient := openStatsDB, newStatsClient
	openStatsDB = func() (*sql.DB, error) { return db.Open(dbPath) }
	newStatsClient = func() audible.StatsClient { return client }
	t.Cleanup(func() {
		openStatsDB, newStatsClient = origDB, origClient
	})

	// A separate handle for assertions, independent of the commands' lifecycle.
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	return database
}

// setStatsConfig applies the defaults the commands read. executeCommand resets
// viper, so this runs after it inside each test via a pre-seeded command run.
func setStatsConfig() {
	viper.Set("stats.timezone", "UTC")
	viper.Set("stats.backfill_start", "2026-01-01")
	viper.Set("stats.rate_limit_seconds", 0)
}

func TestStatsBackfillStoresDaysAndBooks(t *testing.T) {
	client := &fakeCLIStatsClient{
		daily: map[string]float64{"2026-01-01": 3600, "2026-01-02": 1800},
		library: []audible.LibraryListening{
			{ASIN: "ASIN001", Title: "A Test Title", RuntimeMinutes: 600},
		},
	}
	database := withStatsFakes(t, client)

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "backfill", "--json")
	require.NoError(t, err)

	var summary statsSummary
	require.NoError(t, json.Unmarshal([]byte(out), &summary))
	assert.Equal(t, "audible", summary.Source)
	assert.Greater(t, summary.DaysStored, 0)
	assert.Equal(t, 1, summary.Books)

	books, err := db.ListBookListening(database, listening.SourceAudible)
	require.NoError(t, err)
	require.Len(t, books, 1)
	assert.Equal(t, "A Test Title", books[0].Title)
}

func TestStatsBackfillRejectsUnknownSource(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	_, err := executeCommandWithConfig(t, setStatsConfig, "stats", "backfill", "--source", "goodreads")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown source")
}

func TestStatsBackfillRejectsInvalidTimezone(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	_, err := executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("stats.timezone", "Nowhere/Atlantis")
	}, "stats", "backfill")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "valid IANA timezone",
		"a bad timezone must fail loudly rather than silently bucketing in UTC")
}

func TestStatsBackfillRejectsInvalidStartDate(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	_, err := executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("stats.backfill_start", "01/01/2026")
	}, "stats", "backfill")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "YYYY-MM-DD")
}

func TestStatsSyncIsIdempotent(t *testing.T) {
	client := &fakeCLIStatsClient{
		daily:   map[string]float64{"2026-01-01": 3600},
		library: []audible.LibraryListening{{ASIN: "ASIN001", Title: "One"}},
	}
	database := withStatsFakes(t, client)

	_, err := executeCommandWithConfig(t, setStatsConfig, "stats", "sync", "--json")
	require.NoError(t, err)
	_, err = executeCommandWithConfig(t, setStatsConfig, "stats", "sync", "--json")
	require.NoError(t, err)

	n, err := db.CountListeningDays(database, listening.SourceAudible)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "repeated syncs must not duplicate day rows")

	books, err := db.ListBookListening(database, listening.SourceAudible)
	require.NoError(t, err)
	assert.Len(t, books, 1)
}

func TestStatsStatusReportsStoredData(t *testing.T) {
	client := &fakeCLIStatsClient{
		daily:   map[string]float64{"2026-01-01": 3600},
		library: []audible.LibraryListening{{ASIN: "ASIN001", Title: "One"}},
	}
	withStatsFakes(t, client)

	_, err := executeCommandWithConfig(t, setStatsConfig, "stats", "backfill")
	require.NoError(t, err)

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "status", "--json")
	require.NoError(t, err)

	var st statsStatus
	require.NoError(t, json.Unmarshal([]byte(out), &st))
	require.Len(t, st.Sources, 2, "both sources are always reported, even when empty")

	var audibleStatus statsSourceStatus
	for _, s := range st.Sources {
		if s.Source == listening.SourceAudible {
			audibleStatus = s
		}
	}
	assert.Equal(t, 1, audibleStatus.Days)
	assert.Equal(t, 1, audibleStatus.Books)
	assert.Equal(t, 3600, audibleStatus.TotalSeconds)
}

func TestStatsStatusOnEmptyDatabase(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "status", "--json")
	require.NoError(t, err)

	var st statsStatus
	require.NoError(t, json.Unmarshal([]byte(out), &st))
	for _, s := range st.Sources {
		assert.Zero(t, s.Days)
		assert.Zero(t, s.Books)
	}
}

func TestStatsBackfillSurfacesClientErrors(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{err: fmt.Errorf("audible unreachable")})

	_, err := executeCommandWithConfig(t, setStatsConfig, "stats", "backfill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audible unreachable")
}

func TestStatsCommandIsRegistered(t *testing.T) {
	out, err := executeCommand(t, "stats", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "backfill")
	assert.Contains(t, out, "sync")
	assert.Contains(t, out, "status")
}
