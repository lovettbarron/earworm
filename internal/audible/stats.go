package audible

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Audible API batch limits. These are server-enforced; exceeding any of them
// returns HTTP 400 rather than a truncated result, so callers must chunk. They
// are enforced here rather than left to callers.
const (
	// MaxDailyWindow is the largest daily_listening_interval_duration accepted.
	MaxDailyWindow = 30
	// MaxMonthlyWindow is the largest monthly_listening_interval_duration accepted.
	MaxMonthlyWindow = 12
	// MaxLastPositionASINs is the most ASINs accepted per lastpositions call.
	MaxLastPositionASINs = 25
	// maxASINParamLen is the character ceiling on the joined asins parameter,
	// which binds before the count limit if keys are unusually long.
	maxASINParamLen = 500
)

// statsAPITimeout bounds a single API subprocess call.
const statsAPITimeout = 90 * time.Second

// APIParam is one query parameter passed to `audible api` as -p key=value.
type APIParam struct {
	Key   string
	Value string
}

// StatsClient exposes the listening-statistics endpoints.
//
// This is deliberately separate from AudibleClient: the two describe different
// concerns (library management versus listening history), and widening
// AudibleClient would force every existing implementation, including test
// fakes, to grow methods they have no use for.
type StatsClient interface {
	// APIGet performs a raw GET against an Audible API endpoint.
	APIGet(ctx context.Context, endpoint string, params ...APIParam) ([]byte, error)
	// DailyListening returns listening seconds per day for a window.
	DailyListening(ctx context.Context, start time.Time, days int) (map[string]float64, error)
	// MonthlyListening returns listening seconds per month for a window.
	MonthlyListening(ctx context.Context, start time.Time, months int) (map[string]float64, error)
	// FinishedStatuses returns every book's current mark-as-finished state.
	FinishedStatuses(ctx context.Context) ([]FinishedStatus, error)
	// LastPositions returns the last playback position for each ASIN.
	LastPositions(ctx context.Context, asins []string) (map[string]LastPosition, error)
	// LibraryListening returns per-book listening state for the whole library.
	LibraryListening(ctx context.Context) ([]LibraryListening, error)
}

// NewStatsClient creates a client for the listening-statistics endpoints.
func NewStatsClient(audiblePath string, opts ...ClientOption) StatsClient {
	c := &client{audiblePath: audiblePath}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// APIGet runs `audible api <endpoint>` with the given query parameters and
// returns the raw JSON response body.
//
// audible-cli writes the JSON body to stdout but writes human-readable errors
// there too (for example "error: Bad Request (400): ..."), and may exit zero
// while doing so. Output that does not begin with a JSON value is therefore
// treated as an error and classified, rather than handed back to the caller as
// if it parsed.
func (c *client) APIGet(ctx context.Context, endpoint string, params ...APIParam) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, statsAPITimeout)
	defer cancel()

	args := []string{"api", endpoint}
	for _, p := range params {
		args = append(args, "-p", p.Key+"="+p.Value)
	}

	cmd := c.command(ctx, c.buildArgs(args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := bytes.TrimSpace(stdout.Bytes())

	if err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = string(out)
		}
		exit := -1
		if cmd.ProcessState != nil {
			exit = cmd.ProcessState.ExitCode()
		}
		return nil, classifyError("api "+endpoint, msg, exit, err)
	}

	if len(out) == 0 {
		return nil, &CommandError{Command: "api " + endpoint, Stderr: "empty response", ExitCode: 0}
	}
	if out[0] != '{' && out[0] != '[' {
		// Exit code was zero but the payload is not JSON: audible-cli reports
		// API-level failures this way.
		return nil, classifyError("api "+endpoint, string(out), 0, fmt.Errorf("non-JSON response"))
	}
	return out, nil
}

// DailyListening returns total listening seconds per day key (YYYY-MM-DD as
// reported by Audible) for a window starting at start.
//
// days must not exceed MaxDailyWindow. Days with no listening are omitted.
func (c *client) DailyListening(ctx context.Context, start time.Time, days int) (map[string]float64, error) {
	if days <= 0 {
		return nil, fmt.Errorf("daily listening: days must be positive, got %d", days)
	}
	if days > MaxDailyWindow {
		return nil, fmt.Errorf("daily listening: window of %d days exceeds Audible's maximum of %d", days, MaxDailyWindow)
	}

	body, err := c.APIGet(ctx, "/1.0/stats/aggregates",
		APIParam{"daily_listening_interval_duration", fmt.Sprintf("%d", days)},
		APIParam{"daily_listening_interval_start_date", start.Format("2006-01-02")},
		APIParam{"store", "Audible"},
	)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Stats []struct {
			Sum      float64 `json:"aggregated_sum"`
			Interval string  `json:"interval_identifier"`
			Unit     string  `json:"unit"`
		} `json:"aggregated_daily_listening_stats"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode daily listening: %w", err)
	}

	out := make(map[string]float64, len(resp.Stats))
	for _, s := range resp.Stats {
		if s.Sum <= 0 {
			continue
		}
		secs, err := toSeconds(s.Sum, s.Unit)
		if err != nil {
			return nil, fmt.Errorf("daily listening %s: %w", s.Interval, err)
		}
		out[s.Interval] = secs
	}
	return out, nil
}

// MonthlyListening returns total listening seconds per month key (YYYY-MM).
//
// months must not exceed MaxMonthlyWindow. Audible returns an inclusive range,
// so consecutive calls overlap at the boundary month; callers keying by month
// overwrite rather than accumulate.
func (c *client) MonthlyListening(ctx context.Context, start time.Time, months int) (map[string]float64, error) {
	if months <= 0 {
		return nil, fmt.Errorf("monthly listening: months must be positive, got %d", months)
	}
	if months > MaxMonthlyWindow {
		return nil, fmt.Errorf("monthly listening: window of %d months exceeds Audible's maximum of %d", months, MaxMonthlyWindow)
	}

	body, err := c.APIGet(ctx, "/1.0/stats/aggregates",
		APIParam{"monthly_listening_interval_duration", fmt.Sprintf("%d", months)},
		APIParam{"monthly_listening_interval_start_date", start.Format("2006-01-02")},
		APIParam{"store", "Audible"},
	)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Stats []struct {
			Sum      float64 `json:"aggregated_sum"`
			Interval string  `json:"interval_identifier"`
			Unit     string  `json:"unit"`
		} `json:"aggregated_monthly_listening_stats"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode monthly listening: %w", err)
	}

	out := make(map[string]float64, len(resp.Stats))
	for _, s := range resp.Stats {
		if s.Sum <= 0 {
			continue
		}
		secs, err := toSeconds(s.Sum, s.Unit)
		if err != nil {
			return nil, fmt.Errorf("monthly listening %s: %w", s.Interval, err)
		}
		out[s.Interval] = secs
	}
	return out, nil
}

// toSeconds converts an aggregate sum to seconds using the unit Audible
// reports. The unit is honoured rather than assumed: silently treating a
// changed unit as milliseconds would corrupt every stored total.
func toSeconds(sum float64, unit string) (float64, error) {
	switch strings.ToLower(unit) {
	case "milliseconds", "millisecond", "ms":
		return sum / 1000.0, nil
	case "seconds", "second", "s":
		return sum, nil
	case "minutes", "minute":
		return sum * 60.0, nil
	case "":
		// Historically the field is always present; treat absence as the
		// documented default rather than failing the whole backfill.
		return sum / 1000.0, nil
	default:
		return 0, fmt.Errorf("unrecognised listening unit %q", unit)
	}
}

// FinishedStatus is one book's current mark-as-finished state.
//
// EventTimestamp records the last status CHANGE, which may be a change to
// unfinished. It is not by itself evidence that the book was completed.
type FinishedStatus struct {
	ASIN           string    `json:"asin"`
	EventTimestamp string    `json:"event_timestamp"`
	IsFinished     bool      `json:"is_marked_as_finished"`
	UpdateDate     string    `json:"update_date"`
	EventTime      time.Time `json:"-"`
}

// FinishedStatuses returns the mark-as-finished state for every book the
// account has one for, following continuation tokens to exhaustion.
//
// The endpoint returns current state, one record per ASIN, not an event
// history: a book finished twice appears once.
func (c *client) FinishedStatuses(ctx context.Context) ([]FinishedStatus, error) {
	var all []FinishedStatus
	token := ""
	seenTokens := make(map[string]bool)

	for {
		params := []APIParam{{"num_results", "1000"}}
		if token != "" {
			params = append(params, APIParam{"continuation_token", token})
		}
		body, err := c.APIGet(ctx, "/1.0/stats/status/finished", params...)
		if err != nil {
			return nil, err
		}

		var resp struct {
			List              []FinishedStatus `json:"mark_as_finished_status_list"`
			ContinuationToken string           `json:"continuation_token"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode finished statuses: %w", err)
		}
		if len(resp.List) == 0 {
			break
		}
		for i := range resp.List {
			resp.List[i].EventTime = parseAudibleTime(resp.List[i].EventTimestamp)
		}
		all = append(all, resp.List...)

		if resp.ContinuationToken == "" {
			break
		}
		// A repeated token means the server is not advancing; stopping beats
		// paging forever.
		if seenTokens[resp.ContinuationToken] {
			break
		}
		seenTokens[resp.ContinuationToken] = true
		token = resp.ContinuationToken
	}
	return all, nil
}

// LastPosition is the most recent playback position recorded for a book.
type LastPosition struct {
	ASIN        string
	LastUpdated time.Time
	PositionMS  int64
	Exists      bool
}

// LastPositions returns the last playback position for each requested ASIN,
// chunking automatically to respect the endpoint's limits.
//
// ASINs the account has no position for are returned with Exists false rather
// than omitted, so callers can distinguish "never played" from "not asked".
func (c *client) LastPositions(ctx context.Context, asins []string) (map[string]LastPosition, error) {
	out := make(map[string]LastPosition, len(asins))
	for _, chunk := range chunkASINs(asins, MaxLastPositionASINs, maxASINParamLen) {
		body, err := c.APIGet(ctx, "/1.0/annotations/lastpositions",
			APIParam{"asins", strings.Join(chunk, ",")})
		if err != nil {
			return nil, err
		}

		var resp struct {
			Annots []struct {
				ASIN     string `json:"asin"`
				Position struct {
					LastUpdated string  `json:"last_updated"`
					PositionMS  float64 `json:"position_ms"`
					Status      string  `json:"status"`
				} `json:"last_position_heard"`
			} `json:"asin_last_position_heard_annots"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode last positions: %w", err)
		}

		for _, a := range resp.Annots {
			lp := LastPosition{
				ASIN:       a.ASIN,
				PositionMS: int64(a.Position.PositionMS),
				Exists:     strings.EqualFold(a.Position.Status, "Exists"),
			}
			if lp.Exists {
				lp.LastUpdated = parseAudibleTime(a.Position.LastUpdated)
			}
			out[a.ASIN] = lp
		}
	}
	return out, nil
}

// chunkASINs splits ASINs into batches bounded by both a count limit and a
// joined-length limit, since the endpoint enforces both independently.
func chunkASINs(asins []string, maxCount, maxLen int) [][]string {
	var out [][]string
	var cur []string
	curLen := 0

	for _, a := range asins {
		add := len(a)
		if len(cur) > 0 {
			add++ // separating comma
		}
		if len(cur) > 0 && (len(cur)+1 > maxCount || curLen+add > maxLen) {
			out = append(out, cur)
			cur, curLen = nil, 0
			add = len(a)
		}
		cur = append(cur, a)
		curLen += add
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// parseAudibleTime parses the timestamp shapes Audible returns across these
// endpoints. An unparseable or absent value yields the zero time, which
// downstream code treats as "no evidence" rather than as an epoch date.
func parseAudibleTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// LibraryListening is one library book's listening state and descriptive
// metadata, as returned by the library endpoint with listening_status.
type LibraryListening struct {
	ASIN            string
	Title           string
	Authors         []string
	Narrators       []string
	Series          string
	SeriesPosition  string
	Genres          []string
	Keywords        []string
	RuntimeMinutes  int
	PercentComplete float64
	IsFinished      bool
	PurchaseDate    string
	DateAdded       string
	// StatusChangedAt is listening_status.finished_at_timestamp. Despite the
	// name it records the last status CHANGE and is set even when the book is
	// unfinished, so it must not be read as a completion date on its own.
	StatusChangedAt time.Time
}

// libraryListeningResponseGroups requests exactly the fields the stats
// pipeline consumes. Asking for more inflates the response considerably.
const libraryListeningResponseGroups = "product_desc,product_attrs,contributors,series," +
	"category_ladders,is_finished,percent_complete,listening_status"

// LibraryListening returns per-book listening state for the entire library.
func (c *client) LibraryListening(ctx context.Context) ([]LibraryListening, error) {
	body, err := c.APIGet(ctx, "/1.0/library",
		APIParam{"num_results", "1000"},
		APIParam{"response_groups", libraryListeningResponseGroups},
	)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Items []struct {
			ASIN    string `json:"asin"`
			Title   string `json:"title"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Narrators []struct {
				Name string `json:"name"`
			} `json:"narrators"`
			Series []struct {
				Title    string `json:"title"`
				Sequence string `json:"sequence"`
			} `json:"series"`
			CategoryLadders []struct {
				Ladder []struct {
					Name string `json:"name"`
				} `json:"ladder"`
			} `json:"category_ladders"`
			Keywords        []string `json:"thesaurus_subject_keywords"`
			RuntimeMinutes  *int     `json:"runtime_length_min"`
			PercentComplete *float64 `json:"percent_complete"`
			IsFinished      *bool    `json:"is_finished"`
			PurchaseDate    string   `json:"purchase_date"`
			LibraryStatus   *struct {
				DateAdded string `json:"date_added"`
			} `json:"library_status"`
			ListeningStatus *struct {
				FinishedAtTimestamp string   `json:"finished_at_timestamp"`
				IsFinished          *bool    `json:"is_finished"`
				PercentComplete     *float64 `json:"percent_complete"`
			} `json:"listening_status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode library listening: %w", err)
	}

	out := make([]LibraryListening, 0, len(resp.Items))
	for _, it := range resp.Items {
		b := LibraryListening{
			ASIN:         it.ASIN,
			Title:        it.Title,
			PurchaseDate: it.PurchaseDate,
			Keywords:     it.Keywords,
		}
		for _, a := range it.Authors {
			b.Authors = append(b.Authors, a.Name)
		}
		for _, n := range it.Narrators {
			b.Narrators = append(b.Narrators, n.Name)
		}
		if len(it.Series) > 0 {
			b.Series = it.Series[0].Title
			b.SeriesPosition = it.Series[0].Sequence
		}
		// The ladder runs general to specific; the last entry is the most
		// descriptive genre and the one worth keeping.
		for _, cl := range it.CategoryLadders {
			if n := len(cl.Ladder); n > 0 {
				b.Genres = append(b.Genres, cl.Ladder[n-1].Name)
			}
		}
		if it.RuntimeMinutes != nil {
			b.RuntimeMinutes = *it.RuntimeMinutes
		}
		if it.PercentComplete != nil {
			b.PercentComplete = *it.PercentComplete
		}
		if it.IsFinished != nil {
			b.IsFinished = *it.IsFinished
		}
		if it.LibraryStatus != nil {
			b.DateAdded = it.LibraryStatus.DateAdded
		}
		// listening_status is the more specific source and wins where present.
		if it.ListeningStatus != nil {
			b.StatusChangedAt = parseAudibleTime(it.ListeningStatus.FinishedAtTimestamp)
			if it.ListeningStatus.IsFinished != nil {
				b.IsFinished = *it.ListeningStatus.IsFinished
			}
			if it.ListeningStatus.PercentComplete != nil {
				b.PercentComplete = *it.ListeningStatus.PercentComplete
			}
		}
		out = append(out, b)
	}
	return out, nil
}
