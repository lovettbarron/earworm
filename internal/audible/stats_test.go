package audible

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStatsHelperProcess backs the stats subprocess fakes. It is separate from
// TestHelperProcess so that adding stats scenarios cannot disturb the existing
// library/download scenarios in this package.
func TestStatsHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	joined := strings.Join(args, " ")

	switch os.Getenv("GO_STATS_SCENARIO") {
	case "daily":
		fmt.Print(`{"aggregated_daily_listening_stats":[
			{"aggregated_sum":3600000.0,"interval_identifier":"2026-01-01","unit":"Milliseconds"},
			{"aggregated_sum":0.0,"interval_identifier":"2026-01-02","unit":"Milliseconds"},
			{"aggregated_sum":1800000.0,"interval_identifier":"2026-01-03","unit":"Milliseconds"}]}`)
	case "daily_seconds_unit":
		fmt.Print(`{"aggregated_daily_listening_stats":[
			{"aggregated_sum":120.0,"interval_identifier":"2026-01-01","unit":"Seconds"}]}`)
	case "daily_bad_unit":
		fmt.Print(`{"aggregated_daily_listening_stats":[
			{"aggregated_sum":120.0,"interval_identifier":"2026-01-01","unit":"Furlongs"}]}`)
	case "monthly":
		fmt.Print(`{"aggregated_monthly_listening_stats":[
			{"aggregated_sum":7200000.0,"interval_identifier":"2026-01","unit":"Milliseconds"}]}`)
	case "api_error_zero_exit":
		// audible-cli reports API failures on stdout while exiting zero.
		fmt.Print("error: Bad Request (400): validation error")
		os.Exit(0)
	case "empty":
		os.Exit(0)
	case "finished_paged":
		if strings.Contains(joined, "continuation_token=TOKEN1") {
			fmt.Print(`{"mark_as_finished_status_list":[
				{"asin":"ASIN003","event_timestamp":"2026-02-01T10:00:00.000Z","is_marked_as_finished":false}]}`)
		} else {
			fmt.Print(`{"continuation_token":"TOKEN1","mark_as_finished_status_list":[
				{"asin":"ASIN001","event_timestamp":"2026-01-01T10:00:00.000Z","is_marked_as_finished":true},
				{"asin":"ASIN002","event_timestamp":"2026-01-02T11:30:00.000Z","is_marked_as_finished":true}]}`)
		}
	case "finished_repeating_token":
		// Server keeps handing back the same token: must not loop forever.
		fmt.Print(`{"continuation_token":"SAME","mark_as_finished_status_list":[
			{"asin":"ASIN001","event_timestamp":"2026-01-01T10:00:00.000Z","is_marked_as_finished":true}]}`)
	case "lastpositions":
		// Echo one annotation per requested ASIN.
		var asins []string
		for i, a := range args {
			if strings.HasPrefix(a, "asins=") {
				asins = strings.Split(strings.TrimPrefix(a, "asins="), ",")
				break
			}
			_ = i
		}
		var sb strings.Builder
		sb.WriteString(`{"asin_last_position_heard_annots":[`)
		for i, a := range asins {
			if i > 0 {
				sb.WriteString(",")
			}
			if i == 0 {
				fmt.Fprintf(&sb, `{"asin":"%s","last_position_heard":{"status":"DoesNotExist"}}`, a)
				continue
			}
			fmt.Fprintf(&sb, `{"asin":"%s","last_position_heard":{"last_updated":"2026-03-04 05:06:07.089","position_ms":123456,"status":"Exists"}}`, a)
		}
		sb.WriteString(`]}`)
		fmt.Print(sb.String())
	case "library":
		fmt.Print(`{"items":[{
			"asin":"ASIN001","title":"A Test Title",
			"authors":[{"name":"Author One"}],
			"narrators":[{"name":"Narrator One"}],
			"series":[{"title":"Test Series","sequence":"2"}],
			"category_ladders":[{"ladder":[{"name":"Fiction"},{"name":"Fantasy"},{"name":"Epic"}]}],
			"thesaurus_subject_keywords":["magic","quest"],
			"runtime_length_min":600,
			"percent_complete":10.0,
			"is_finished":false,
			"purchase_date":"2025-01-02T03:04:05.000Z",
			"library_status":{"date_added":"2025-01-02T03:04:05.000Z"},
			"listening_status":{"finished_at_timestamp":"2026-06-16T20:31:07.369Z","is_finished":true,"percent_complete":98.0}
		}]}`)
	}
	os.Exit(0)
}

// fakeStatsCommand routes commands to TestStatsHelperProcess and records the
// argument list of every invocation so tests can assert on batching.
func fakeStatsCommand(scenario string, calls *[][]string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if calls != nil {
			*calls = append(*calls, append([]string(nil), args...))
		}
		cs := append([]string{"-test.run=TestStatsHelperProcess", "--"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_STATS_SCENARIO="+scenario)
		return cmd
	}
}

func newTestStatsClient(scenario string, calls *[][]string) StatsClient {
	return NewStatsClient("audible", WithCmdFactory(fakeStatsCommand(scenario, calls)))
}

func TestDailyListeningConvertsMillisecondsAndDropsZeroDays(t *testing.T) {
	c := newTestStatsClient("daily", nil)
	got, err := c.DailyListening(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 3)
	require.NoError(t, err)

	assert.Len(t, got, 2, "days with zero listening should be omitted")
	assert.InDelta(t, 3600.0, got["2026-01-01"], 0.001)
	assert.InDelta(t, 1800.0, got["2026-01-03"], 0.001)
	assert.NotContains(t, got, "2026-01-02")
}

func TestDailyListeningHonoursReportedUnit(t *testing.T) {
	c := newTestStatsClient("daily_seconds_unit", nil)
	got, err := c.DailyListening(context.Background(), time.Now(), 1)
	require.NoError(t, err)
	// Reported in seconds, so it must not be divided by 1000.
	assert.InDelta(t, 120.0, got["2026-01-01"], 0.001)
}

func TestDailyListeningRejectsUnknownUnit(t *testing.T) {
	c := newTestStatsClient("daily_bad_unit", nil)
	_, err := c.DailyListening(context.Background(), time.Now(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognised listening unit")
}

func TestDailyListeningEnforcesWindowLimitWithoutCallingAPI(t *testing.T) {
	var calls [][]string
	c := newTestStatsClient("daily", &calls)

	_, err := c.DailyListening(context.Background(), time.Now(), MaxDailyWindow+1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds Audible's maximum")
	assert.Empty(t, calls, "limit must be enforced before spending an API call")

	_, err = c.DailyListening(context.Background(), time.Now(), 0)
	require.Error(t, err)
	assert.Empty(t, calls)
}

func TestMonthlyListeningEnforcesWindowLimit(t *testing.T) {
	var calls [][]string
	c := newTestStatsClient("monthly", &calls)

	_, err := c.MonthlyListening(context.Background(), time.Now(), MaxMonthlyWindow+1)
	require.Error(t, err)
	assert.Empty(t, calls)

	got, err := c.MonthlyListening(context.Background(), time.Now(), MaxMonthlyWindow)
	require.NoError(t, err)
	assert.InDelta(t, 7200.0, got["2026-01"], 0.001)
}

func TestAPIGetTreatsNonJSONStdoutAsError(t *testing.T) {
	c := newTestStatsClient("api_error_zero_exit", nil)
	_, err := c.APIGet(context.Background(), "/1.0/stats/aggregates")
	require.Error(t, err, "a zero exit code with a non-JSON body is still a failure")
	assert.Contains(t, err.Error(), "Bad Request")
}

func TestAPIGetRejectsEmptyResponse(t *testing.T) {
	c := newTestStatsClient("empty", nil)
	_, err := c.APIGet(context.Background(), "/1.0/library")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestFinishedStatusesFollowsContinuationToken(t *testing.T) {
	var calls [][]string
	c := newTestStatsClient("finished_paged", &calls)

	got, err := c.FinishedStatuses(context.Background())
	require.NoError(t, err)

	require.Len(t, got, 3)
	assert.Len(t, calls, 2, "should have paged exactly twice")
	assert.Equal(t, "ASIN001", got[0].ASIN)
	assert.True(t, got[0].IsFinished)
	assert.False(t, got[2].IsFinished, "an unfinish event must be preserved as such")
	assert.Equal(t, 2026, got[0].EventTime.Year())
}

func TestFinishedStatusesStopsOnRepeatedToken(t *testing.T) {
	var calls [][]string
	c := newTestStatsClient("finished_repeating_token", &calls)

	got, err := c.FinishedStatuses(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 2, "one page, then one retry with the token before bailing")
	assert.LessOrEqual(t, len(calls), 3, "must not page forever on a stuck token")
}

func TestLastPositionsChunksByCountLimit(t *testing.T) {
	var calls [][]string
	c := newTestStatsClient("lastpositions", &calls)

	asins := make([]string, 60)
	for i := range asins {
		asins[i] = fmt.Sprintf("ASIN%06d", i)
	}

	got, err := c.LastPositions(context.Background(), asins)
	require.NoError(t, err)

	assert.Len(t, got, 60, "every requested ASIN should be represented")
	assert.Equal(t, 3, len(calls), "60 ASINs at 25 per call is 3 calls")

	for _, call := range calls {
		for _, arg := range call {
			if strings.HasPrefix(arg, "asins=") {
				list := strings.TrimPrefix(arg, "asins=")
				assert.LessOrEqual(t, len(strings.Split(list, ",")), MaxLastPositionASINs)
				assert.LessOrEqual(t, len(list), maxASINParamLen)
			}
		}
	}
}

func TestLastPositionsDistinguishesMissingFromPresent(t *testing.T) {
	c := newTestStatsClient("lastpositions", nil)
	got, err := c.LastPositions(context.Background(), []string{"ASINAAA", "ASINBBB"})
	require.NoError(t, err)

	require.Contains(t, got, "ASINAAA")
	assert.False(t, got["ASINAAA"].Exists, "never-played books are reported, not omitted")
	assert.True(t, got["ASINAAA"].LastUpdated.IsZero())

	require.Contains(t, got, "ASINBBB")
	assert.True(t, got["ASINBBB"].Exists)
	assert.EqualValues(t, 123456, got["ASINBBB"].PositionMS)
	assert.Equal(t, 2026, got["ASINBBB"].LastUpdated.Year())
}

func TestChunkASINsRespectsBothLimits(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		maxCount int
		maxLen   int
		wantMax  int
	}{
		{"count bound", 60, 25, 10000, 25},
		{"length bound", 60, 1000, 55, 5},
		{"single batch", 3, 25, 500, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asins := make([]string, tt.n)
			for i := range asins {
				asins[i] = "B000000000"
			}
			chunks := chunkASINs(asins, tt.maxCount, tt.maxLen)

			total := 0
			for _, c := range chunks {
				total += len(c)
				assert.LessOrEqual(t, len(c), tt.wantMax)
				assert.LessOrEqual(t, len(strings.Join(c, ",")), tt.maxLen)
			}
			assert.Equal(t, tt.n, total, "chunking must not drop or duplicate ASINs")
		})
	}
}

func TestChunkASINsEmpty(t *testing.T) {
	assert.Empty(t, chunkASINs(nil, 25, 500))
}

func TestLibraryListeningParsesMetadataAndPrefersListeningStatus(t *testing.T) {
	c := newTestStatsClient("library", nil)
	got, err := c.LibraryListening(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)

	b := got[0]
	assert.Equal(t, "ASIN001", b.ASIN)
	assert.Equal(t, "A Test Title", b.Title)
	assert.Equal(t, []string{"Author One"}, b.Authors)
	assert.Equal(t, []string{"Narrator One"}, b.Narrators)
	assert.Equal(t, "Test Series", b.Series)
	assert.Equal(t, "2", b.SeriesPosition)
	assert.Equal(t, []string{"Epic"}, b.Genres, "the most specific ladder entry is the useful genre")
	assert.Equal(t, []string{"magic", "quest"}, b.Keywords)
	assert.Equal(t, 600, b.RuntimeMinutes)

	// listening_status is more specific than the top-level fields and wins.
	assert.True(t, b.IsFinished)
	assert.InDelta(t, 98.0, b.PercentComplete, 0.001)
	assert.Equal(t, 2026, b.StatusChangedAt.Year())
}

func TestToSeconds(t *testing.T) {
	tests := []struct {
		unit string
		in   float64
		want float64
		err  bool
	}{
		{"Milliseconds", 1000, 1, false},
		{"milliseconds", 2500, 2.5, false},
		{"Seconds", 42, 42, false},
		{"Minutes", 2, 120, false},
		{"", 1000, 1, false},
		{"parsecs", 1, 0, true},
	}
	for _, tt := range tests {
		got, err := toSeconds(tt.in, tt.unit)
		if tt.err {
			assert.Error(t, err, tt.unit)
			continue
		}
		require.NoError(t, err, tt.unit)
		assert.InDelta(t, tt.want, got, 0.0001, tt.unit)
	}
}

func TestParseAudibleTimeHandlesAllObservedLayouts(t *testing.T) {
	// Audible returns different layouts per endpoint; all must parse.
	assert.Equal(t, 2026, parseAudibleTime("2026-01-06T17:20:37.562Z").Year())
	assert.Equal(t, 2026, parseAudibleTime("2026-07-19 13:58:39.503").Year())
	assert.Equal(t, 2026, parseAudibleTime("2026-05-19").Year())

	// Unparseable input must yield the zero time, never a 1970 epoch date,
	// so downstream code reads it as "no evidence".
	assert.True(t, parseAudibleTime("").IsZero())
	assert.True(t, parseAudibleTime("not a date").IsZero())
}
