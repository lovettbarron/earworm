package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/bookidentity"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/journal"
	"github.com/lovettbarron/earworm/internal/listening"
	"github.com/lovettbarron/earworm/internal/stats"
	"github.com/lovettbarron/earworm/internal/statsexport"
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
	require.Len(t, st.Sources, 3, "every source is reported, even when empty")

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

// fakeABSSource is a scripted stats.ABSSessionSource for CLI tests.
type fakeABSSource struct {
	user      audiobookshelf.User
	sessions  []audiobookshelf.PlaybackSession
	status    audiobookshelf.ServerStatus
	statusErr error
	userErr   error
}

func (f *fakeABSSource) Me(context.Context) (audiobookshelf.User, []audiobookshelf.MediaProgress, error) {
	return f.user, nil, f.userErr
}
func (f *fakeABSSource) ListSessions(context.Context, audiobookshelf.SessionsOptions) ([]audiobookshelf.PlaybackSession, error) {
	return f.sessions, nil
}
func (f *fakeABSSource) GetItems(context.Context, []string) (map[string]audiobookshelf.LibraryItem, error) {
	return map[string]audiobookshelf.LibraryItem{}, nil
}
func (f *fakeABSSource) Status(context.Context) (audiobookshelf.ServerStatus, error) {
	return f.status, f.statusErr
}

func withABSFakes(t *testing.T, src *fakeABSSource) {
	t.Helper()
	origClient, origStatus := newABSClient, newABSStatusClient
	newABSClient = func() stats.ABSSessionSource { return src }
	newABSStatusClient = func() statusChecker { return src }
	t.Cleanup(func() { newABSClient, newABSStatusClient = origClient, origStatus })
}

func setABSConfig() {
	setStatsConfig()
	viper.Set("audiobookshelf.url", "http://abs.invalid:13378")
	viper.Set("audiobookshelf.token", "test-token")
	viper.Set("audiobookshelf.user_id", "user-1")
	viper.Set("stats.enrich", false)
}

func TestStatsBackfillABSStoresSessions(t *testing.T) {
	started := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	src := &fakeABSSource{
		user: audiobookshelf.User{ID: "user-1"},
		sessions: []audiobookshelf.PlaybackSession{{
			ID: "s1", LibraryItemID: "item-1", DisplayTitle: "A Test Title",
			TimeListening: audiobookshelf.LenientSeconds(1800),
			Duration:      audiobookshelf.LenientSeconds(36000),
			StartedAt:     audiobookshelf.EpochMillis(started.UnixMilli()),
			UpdatedAt:     audiobookshelf.EpochMillis(started.Add(time.Hour).UnixMilli()),
		}},
	}
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	withABSFakes(t, src)

	_, err := executeCommandWithConfig(t, setABSConfig, "stats", "backfill", "--source", "abs", "--json")
	require.NoError(t, err)

	n, err := db.CountListeningSessions(database)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	days, err := db.ListListeningDays(database, listening.SourceABS)
	require.NoError(t, err)
	require.Len(t, days, 1)
	assert.Equal(t, 1800, days[0].Seconds)
}

func TestStatsABSRequiresURLAndToken(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withABSFakes(t, &fakeABSSource{})

	_, err := executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("audiobookshelf.url", "")
	}, "stats", "backfill", "--source", "abs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audiobookshelf.url is not configured")

	_, err = executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("audiobookshelf.url", "http://abs.invalid:13378")
		viper.Set("audiobookshelf.token", "")
	}, "stats", "backfill", "--source", "abs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audiobookshelf.token is not configured")
}

func TestStatsBackfillRejectsUnknownSourceMentionsBoth(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	_, err := executeCommandWithConfig(t, setStatsConfig, "stats", "backfill", "--source", "trakt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audible, abs")
}

func TestStatsCheckReportsServerAndUser(t *testing.T) {
	src := &fakeABSSource{
		user:   audiobookshelf.User{ID: "user-1", Username: "tester"},
		status: audiobookshelf.ServerStatus{App: "audiobookshelf", ServerVersion: "2.36.0"},
	}
	withStatsFakes(t, &fakeCLIStatsClient{})
	withABSFakes(t, src)

	out, err := executeCommandWithConfig(t, setABSConfig, "stats", "check", "--json")
	require.NoError(t, err)

	var res absCheckResult
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.Reachable)
	assert.True(t, res.Authenticated)
	assert.Equal(t, "2.36.0", res.ServerVersion)
	assert.Equal(t, "user-1", res.UserID)
}

// Unreachable and unauthorized are different problems and must not be
// reported as the same one.
func TestStatsCheckDistinguishesUnreachableFromUnauthorized(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	withABSFakes(t, &fakeABSSource{statusErr: fmt.Errorf("connection refused")})
	_, err := executeCommandWithConfig(t, setABSConfig, "stats", "check")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreachable")

	withABSFakes(t, &fakeABSSource{
		status:  audiobookshelf.ServerStatus{ServerVersion: "2.36.0"},
		userErr: fmt.Errorf("401 rejected"),
	})
	_, err = executeCommandWithConfig(t, func() {
		setABSConfig()
		viper.Set("audiobookshelf.user_id", "")
	}, "stats", "check")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token was not accepted")
	assert.Contains(t, err.Error(), "2.36.0", "a reachable server should still report its version")
}

func TestStatsCheckRequiresURL(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withABSFakes(t, &fakeABSSource{})

	_, err := executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("audiobookshelf.url", "")
	}, "stats", "check")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

// seedExportData populates a database with both sources' data.
func seedExportData(t *testing.T, database *sql.DB) {
	t.Helper()

	require.NoError(t, db.UpsertBookListeningBatch(database, []db.BookListening{
		{
			Source: listening.SourceAudible, SourceKey: "A1", ASIN: "A1",
			Title: "Example Chronicle", Author: "An Author",
			RuntimeSeconds: 7200, PercentComplete: 100, IsFinished: true,
			StatusChangedAt: "2026-01-03T12:00:00Z", LastPositionAt: "2026-01-03T12:00:00Z",
			AddedDate: "2026-01-01T00:00:00Z",
		},
		{
			Source: listening.SourceABS, SourceKey: "item-1", ASIN: "A1",
			Title: "Example Chronicle", Author: "An Author",
			RuntimeSeconds: 7200, SecondsListened: 1800,
		},
		{
			Source: listening.SourceABS, SourceKey: "item-2",
			Title: "Only In Audiobookshelf", Author: "Another Author",
			RuntimeSeconds: 3600, SecondsListened: 900,
		},
	}))

	require.NoError(t, db.UpsertListeningDays(database, []db.ListeningDay{
		{Day: "2026-01-02", Source: listening.SourceAudible, Seconds: 3600},
		{Day: "2026-01-03", Source: listening.SourceAudible, Seconds: 3600},
	}))

	require.NoError(t, db.UpsertListeningSessions(database, []db.ListeningSession{
		{ID: "s1", LibraryItemID: "item-1", Day: "2026-02-01", Seconds: 1800,
			Title: "Example Chronicle", DurationSeconds: 7200},
	}))
}

func TestStatsExportWritesFilesToLocalDir(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedExportData(t, database)
	outDir := filepath.Join(t.TempDir(), "export")

	out, err := executeCommandWithConfig(t, setStatsConfig,
		"stats", "export", "--output", outDir, "--timeline", "--json")
	require.NoError(t, err)

	var res statsexport.Result
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, outDir, res.Dir)
	assert.Equal(t, 2, res.Books, "the ASIN pair merges into one book")

	for _, name := range []string{"books.csv", "days.csv", "sessions.csv", "timeline.csv", "README.md"} {
		_, statErr := os.Stat(filepath.Join(outDir, name))
		assert.NoError(t, statErr, "%s should exist", name)
	}
}

func TestStatsExportUsesConfiguredDirByDefault(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedExportData(t, database)
	configured := filepath.Join(t.TempDir(), "configured-export")

	_, err := executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("stats.export_dir", configured)
	}, "stats", "export", "--json")
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(configured, "books.csv"))
	assert.NoError(t, statErr)
}

func TestStatsExportRequiresOutputDir(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	_, err := executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("stats.export_dir", "")
	}, "stats", "export")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no output directory")
}

func TestStatsExportReportsAttributionSplit(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedExportData(t, database)
	outDir := filepath.Join(t.TempDir(), "export")

	out, err := executeCommandWithConfig(t, setStatsConfig,
		"stats", "export", "--output", outDir, "--json")
	require.NoError(t, err)

	var res statsexport.Result
	require.NoError(t, json.Unmarshal([]byte(out), &res))

	assert.Positive(t, res.Stats.ExactSeconds, "session-backed listening is measured")
	assert.Positive(t, res.Stats.InferredSeconds+res.Stats.UnattributedSeconds,
		"Audible day totals cannot be measured, so they land in the weaker buckets")
}

func TestStatsMatchesHidesASINMatchesByDefault(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedExportData(t, database)

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "matches", "--json")
	require.NoError(t, err)

	var rows []matchRow
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	for _, r := range rows {
		assert.NotEqual(t, bookidentity.MatchASIN, r.Method,
			"certain matches are noise in a review listing")
	}

	outAll, err := executeCommandWithConfig(t, setStatsConfig, "stats", "matches", "--all", "--json")
	require.NoError(t, err)

	var allRows []matchRow
	require.NoError(t, json.Unmarshal([]byte(outAll), &allRows))
	assert.Greater(t, len(allRows), len(rows), "--all should include the ASIN matches")
}

func TestStatsMatchesOnEmptyDatabase(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "matches")
	require.NoError(t, err)
	assert.Contains(t, out, "No matches to review")
}

func TestStatsExportRejectsInvalidTimezone(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	_, err := executeCommandWithConfig(t, func() {
		setStatsConfig()
		viper.Set("stats.timezone", "Nowhere/Atlantis")
	}, "stats", "export", "--output", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "valid IANA timezone")
}

func TestStatsExportTextOutput(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedExportData(t, database)
	outDir := filepath.Join(t.TempDir(), "export")

	out, err := executeCommandWithConfig(t, setStatsConfig,
		"stats", "export", "--output", outDir, "--timeline")
	require.NoError(t, err)

	assert.Contains(t, out, "Exported to")
	assert.Contains(t, out, "books.csv")
	assert.Contains(t, out, "timeline.csv")
	// The attribution breakdown is the point of the summary and must be shown.
	assert.Contains(t, out, "Measured:")
	assert.Contains(t, out, "Inferred:")
	assert.Contains(t, out, "Unattributed:")
}

func TestStatsMatchesTextOutput(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedExportData(t, database)

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "matches")
	require.NoError(t, err)

	assert.Contains(t, out, "METHOD")
	assert.Contains(t, out, "Only In Audiobookshelf")
	assert.Contains(t, out, "--all")

	outAll, err := executeCommandWithConfig(t, setStatsConfig, "stats", "matches", "--all")
	require.NoError(t, err)
	assert.Contains(t, outAll, "Example Chronicle")
}

func TestStatsStatusTextOutput(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedExportData(t, database)
	require.NoError(t, db.UpsertBookListening(database, db.BookListening{
		Source: listening.SourceAudible, SourceKey: "A2", StatusIsBulk: true,
	}))

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "status")
	require.NoError(t, err)

	assert.Contains(t, out, "audible")
	assert.Contains(t, out, "hours")
	assert.Contains(t, out, "bulk-marked",
		"a bulk-marked count is worth surfacing since it affects finish data")
}

func TestStatsStatusTextOutputOnEmptyDatabase(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "no data")
}

func TestStatsBackfillTextOutput(t *testing.T) {
	client := &fakeCLIStatsClient{
		daily:   map[string]float64{"2026-01-01": 3600},
		library: []audible.LibraryListening{{ASIN: "ASIN001", Title: "One"}},
	}
	withStatsFakes(t, client)

	out, err := executeCommandWithConfig(t, setStatsConfig, "stats", "backfill")
	require.NoError(t, err)

	assert.Contains(t, out, "Backfilling")
	assert.Contains(t, out, "Days stored")
	assert.Contains(t, out, "Books:")
}

func TestStatsABSSyncTextOutput(t *testing.T) {
	started := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	src := &fakeABSSource{
		user: audiobookshelf.User{ID: "user-1"},
		sessions: []audiobookshelf.PlaybackSession{{
			ID: "s1", LibraryItemID: "item-1", DisplayTitle: "A Test Title",
			TimeListening: audiobookshelf.LenientSeconds(1800),
			Duration:      audiobookshelf.LenientSeconds(36000),
			StartedAt:     audiobookshelf.EpochMillis(started.UnixMilli()),
			UpdatedAt:     audiobookshelf.EpochMillis(started.Add(time.Hour).UnixMilli()),
		}},
	}
	withStatsFakes(t, &fakeCLIStatsClient{})
	withABSFakes(t, src)

	out, err := executeCommandWithConfig(t, setABSConfig, "stats", "sync", "--source", "abs")
	require.NoError(t, err)

	assert.Contains(t, out, "audiobookshelf")
	assert.Contains(t, out, "Sessions:")
	assert.Contains(t, out, "Mode:")
}

func TestStatsCheckTextOutput(t *testing.T) {
	src := &fakeABSSource{
		user:   audiobookshelf.User{ID: "user-1", Username: "tester"},
		status: audiobookshelf.ServerStatus{ServerVersion: "2.36.0"},
	}
	withStatsFakes(t, &fakeCLIStatsClient{})
	withABSFakes(t, src)

	out, err := executeCommandWithConfig(t, setABSConfig, "stats", "check")
	require.NoError(t, err)

	assert.Contains(t, out, "2.36.0")
	assert.Contains(t, out, "Token accepted")
}

// --quiet must suppress human output without suppressing the work itself.
func TestStatsQuietSuppressesOutput(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{
		daily:   map[string]float64{"2026-01-01": 3600},
		library: []audible.LibraryListening{{ASIN: "ASIN001", Title: "One"}},
	})

	out, err := executeCommandWithConfig(t, setStatsConfig, "--quiet", "stats", "backfill")
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(out))

	n, err := db.CountListeningDays(database, listening.SourceAudible)
	require.NoError(t, err)
	assert.Positive(t, n, "quiet suppresses reporting, not the work")
}

// fakeJournalWriter records deliveries instead of shelling out.
type fakeJournalWriter struct {
	writes   []journal.Entry
	flushes  int
	writeErr error
}

func (f *fakeJournalWriter) Write(_ context.Context, journalID string, e journal.Entry) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes = append(f.writes, e)
	return nil
}
func (f *fakeJournalWriter) Flush(context.Context) error { f.flushes++; return nil }

type fakeJournalLister struct {
	journals []journal.Journal
	err      error
}

func (f *fakeJournalLister) ListJournals(context.Context) ([]journal.Journal, error) {
	return f.journals, f.err
}

func withJournalFakes(t *testing.T, w *fakeJournalWriter, l *fakeJournalLister) {
	t.Helper()
	origW, origL := newJournalWriter, newJournalLister
	newJournalWriter = func() journal.Writer { return w }
	newJournalLister = func() interface {
		ListJournals(ctx context.Context) ([]journal.Journal, error)
	} {
		return l
	}
	t.Cleanup(func() { newJournalWriter, newJournalLister = origW, origL })
}

func setJournalConfig() {
	setStatsConfig()
	viper.Set("journal.journal_id", "journal-1")
	viper.Set("journal.cli_path", "dayone")
}

func seedJournalData(t *testing.T, database *sql.DB) {
	t.Helper()
	require.NoError(t, db.UpsertListeningSessions(database, []db.ListeningSession{
		{ID: "s1", LibraryItemID: "item-1", Day: "2026-09-19", Seconds: 3600,
			Title: "Example Chronicle", Author: "An Author"},
		{ID: "s2", LibraryItemID: "item-1", Day: "2026-09-20", Seconds: 2700,
			Title: "Example Chronicle", Author: "An Author"},
	}))
}

// Nothing is written unless --write is passed. This is the only command that
// modifies a record outside earworm's own database.
func TestStatsJournalIsDryRunByDefault(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedJournalData(t, database)
	w := &fakeJournalWriter{}
	withJournalFakes(t, w, &fakeJournalLister{})

	out, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journal")
	require.NoError(t, err)

	assert.Contains(t, out, "Dry run")
	assert.Empty(t, w.writes, "a dry run must not touch the journal")

	n, err := db.CountJournalEntries(database)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestStatsJournalWritesWithFlag(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedJournalData(t, database)
	w := &fakeJournalWriter{}
	withJournalFakes(t, w, &fakeJournalLister{})

	out, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journal", "--write")
	require.NoError(t, err)

	assert.Contains(t, out, "Journal updated")
	assert.Len(t, w.writes, 2)
	assert.Equal(t, 1, w.flushes, "queued writes must be pushed")

	n, err := db.CountJournalEntries(database)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestStatsJournalRepeatedWriteIsNoop(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedJournalData(t, database)
	w := &fakeJournalWriter{}
	withJournalFakes(t, w, &fakeJournalLister{})

	_, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journal", "--write")
	require.NoError(t, err)
	out, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journal", "--write")
	require.NoError(t, err)

	assert.Len(t, w.writes, 2, "a second run must not duplicate entries")
	assert.Contains(t, out, "Unchanged: 2")
}

func TestStatsJournalSingleDate(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedJournalData(t, database)
	w := &fakeJournalWriter{}
	withJournalFakes(t, w, &fakeJournalLister{})

	_, err := executeCommandWithConfig(t, setJournalConfig,
		"stats", "journal", "--date", "2026-09-20", "--write")
	require.NoError(t, err)

	require.Len(t, w.writes, 1)
	assert.Equal(t, journal.DayKey("2026-09-20"), w.writes[0].Key)
}

func TestStatsJournalRejectsBadDates(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withJournalFakes(t, &fakeJournalWriter{}, &fakeJournalLister{})

	_, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journal", "--date", "20/09/2026")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "YYYY-MM-DD")

	_, err = executeCommandWithConfig(t, setJournalConfig, "stats", "journal", "--since", "nonsense")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--since")
}

func TestStatsJournalPreviewShowsBody(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedJournalData(t, database)
	withJournalFakes(t, &fakeJournalWriter{}, &fakeJournalLister{})

	out, err := executeCommandWithConfig(t, setJournalConfig,
		"stats", "journal", "--preview", "--date", "2026-09-20")
	require.NoError(t, err)

	assert.Contains(t, out, "## 2026-09-20")
	assert.Contains(t, out, "Example Chronicle")
}

func TestStatsJournalWithNoDataSaysSo(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withJournalFakes(t, &fakeJournalWriter{}, &fakeJournalLister{})

	out, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journal")
	require.NoError(t, err)
	assert.Contains(t, out, "No entries to write")
}

func TestStatsJournalRequiresJournalIDToWrite(t *testing.T) {
	database := withStatsFakes(t, &fakeCLIStatsClient{})
	seedJournalData(t, database)
	withJournalFakes(t, &fakeJournalWriter{}, &fakeJournalLister{})

	_, err := executeCommandWithConfig(t, func() {
		setJournalConfig()
		viper.Set("journal.journal_id", "")
	}, "stats", "journal", "--write")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal_id is not configured")
}

func TestStatsJournalsListsDestinations(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withJournalFakes(t, &fakeJournalWriter{}, &fakeJournalLister{journals: []journal.Journal{
		{ID: "111", Name: "Journal", Encryption: "plaintext"},
		{ID: "222", Name: "Audiobooks", Encryption: "plaintext"},
	}})

	out, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journals")
	require.NoError(t, err)
	assert.Contains(t, out, "Audiobooks")
	assert.Contains(t, out, "222")
}

func TestStatsJournalsHandlesEmptyList(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withJournalFakes(t, &fakeJournalWriter{}, &fakeJournalLister{})

	out, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journals")
	require.NoError(t, err)
	assert.Contains(t, out, "No journals found")
}

func TestStatsJournalsReportsCLIError(t *testing.T) {
	withStatsFakes(t, &fakeCLIStatsClient{})
	withJournalFakes(t, &fakeJournalWriter{},
		&fakeJournalLister{err: fmt.Errorf("dayone not signed in")})

	_, err := executeCommandWithConfig(t, setJournalConfig, "stats", "journals")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not signed in")
}

func TestResolveJournalRangeRelativeDates(t *testing.T) {
	bucket, err := listening.NewBucketer("UTC")
	require.NoError(t, err)
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	journalDate = "today"
	opts, err := resolveJournalRange(bucket, now)
	require.NoError(t, err)
	assert.Equal(t, "2026-09-20", opts.Since)
	assert.Equal(t, "2026-09-20", opts.Until)

	journalDate = "yesterday"
	opts, err = resolveJournalRange(bucket, now)
	require.NoError(t, err)
	assert.Equal(t, "2026-09-19", opts.Since)

	journalDate = ""
}
