package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/download"
	"github.com/lovettbarron/earworm/internal/listening"
	"github.com/lovettbarron/earworm/internal/stats"
	"github.com/lovettbarron/earworm/internal/venv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	statsSource   string
	statsJSON     bool
	statsFullScan bool
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Collect and inspect listening history",
	Long: `Collect listening history from Audible and Audiobookshelf into the local
database, then inspect or export it.

Listening data never leaves your machine: it is stored in the local SQLite
database and exported only to a local directory.`,
}

var statsBackfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Backfill complete listening history",
	Long: `Walk the full listening history of a source and store it locally.

Backfill is resumable: if interrupted, re-running continues from the last
completed window rather than starting over. Re-running a completed backfill
refreshes recent days without duplicating anything.

This makes many API calls. The configured rate limit is applied between them.`,
	RunE: runStatsBackfill,
}

var statsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Incrementally sync recent listening history",
	Long: `Fetch listening activity since the last sync and refresh per-book state.

Intended for repeated runs, including from the daemon. For the initial import
of historical data, use 'earworm stats backfill'.`,
	RunE: runStatsSync,
}

var statsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show what listening data is stored locally",
	RunE:  runStatsStatus,
}

var statsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify Audiobookshelf connectivity and credentials",
	Long: `Confirm that the configured Audiobookshelf server is reachable and that the
API token works, before running a sync that would otherwise fail partway.

Reports the server version and the account the token belongs to.`,
	RunE: runStatsCheck,
}

func init() {
	statsBackfillCmd.Flags().StringVar(&statsSource, "source", "audible", "source to backfill (audible, abs)")
	statsBackfillCmd.Flags().BoolVar(&statsJSON, "json", false, "output summary in JSON format")
	statsBackfillCmd.Flags().BoolVar(&statsFullScan, "full", false, "ignore the saved watermark and refetch everything")

	statsSyncCmd.Flags().StringVar(&statsSource, "source", "audible", "source to sync (audible, abs)")
	statsSyncCmd.Flags().BoolVar(&statsJSON, "json", false, "output summary in JSON format")
	statsSyncCmd.Flags().BoolVar(&statsFullScan, "full", false, "ignore the saved watermark and refetch everything")

	statsStatusCmd.Flags().BoolVar(&statsJSON, "json", false, "output status in JSON format")

	statsCmd.AddCommand(statsBackfillCmd)
	statsCmd.AddCommand(statsSyncCmd)
	statsCmd.AddCommand(statsStatusCmd)
	statsCheckCmd.Flags().BoolVar(&statsJSON, "json", false, "output result in JSON format")
	statsCmd.AddCommand(statsCheckCmd)
	rootCmd.AddCommand(statsCmd)
}

// newStatsClient builds an Audible stats client from config. Extracted as a
// variable so tests can inject a fake, matching newAudibleClient.
var newStatsClient = func() audible.StatsClient {
	cliPath := viper.GetString("audible_cli_path")
	if cliPath == "audible" {
		managed, err := venv.EnsureAudibleCLI(context.Background(), os.Stderr)
		if err == nil {
			cliPath = managed
		}
	}
	var opts []audible.ClientOption
	if profilePath := viper.GetString("audible.profile_path"); profilePath != "" {
		opts = append(opts, audible.WithProfilePath(profilePath))
	}
	return audible.NewStatsClient(cliPath, opts...)
}

// openStatsDB opens the configured database. Extracted for testability.
var openStatsDB = func() (*sql.DB, error) {
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return database, nil
}

// newAudibleIngestor assembles an ingestor from configuration.
func newAudibleIngestor(database *sql.DB) (*stats.AudibleIngestor, error) {
	bucket, err := listening.NewBucketer(viper.GetString("stats.timezone"))
	if err != nil {
		return nil, fmt.Errorf("stats.timezone is not a valid IANA timezone: %w", err)
	}

	startStr := viper.GetString("stats.backfill_start")
	start, err := time.ParseInLocation(listening.DayFormat, startStr, bucket.Location())
	if err != nil {
		return nil, fmt.Errorf("stats.backfill_start %q must be YYYY-MM-DD: %w", startStr, err)
	}

	return &stats.AudibleIngestor{
		DB:            database,
		Client:        newStatsClient(),
		Bucket:        bucket,
		Clock:         listening.SystemClock{},
		Limiter:       download.NewRateLimiter(viper.GetInt("stats.rate_limit_seconds")),
		BackfillStart: start,
		BulkOptions:   listening.DefaultBulkClusterOptions,
	}, nil
}

// newABSClient builds an Audiobookshelf client from config. Extracted as a
// variable so tests can inject a fake.
var newABSClient = func() stats.ABSSessionSource {
	return audiobookshelf.NewClient(
		viper.GetString("audiobookshelf.url"),
		viper.GetString("audiobookshelf.token"),
		viper.GetString("audiobookshelf.library_id"),
	)
}

// newABSIngestor assembles an Audiobookshelf ingestor from configuration.
func newABSIngestor(database *sql.DB) (*stats.ABSIngestor, error) {
	if viper.GetString("audiobookshelf.url") == "" {
		return nil, fmt.Errorf("audiobookshelf.url is not configured (see 'earworm config set')")
	}
	if viper.GetString("audiobookshelf.token") == "" {
		return nil, fmt.Errorf("audiobookshelf.token is not configured (create an API key in Audiobookshelf settings)")
	}

	bucket, err := listening.NewBucketer(viper.GetString("stats.timezone"))
	if err != nil {
		return nil, fmt.Errorf("stats.timezone is not a valid IANA timezone: %w", err)
	}

	return &stats.ABSIngestor{
		DB:     database,
		Client: newABSClient(),
		Bucket: bucket,
		Clock:  listening.SystemClock{},
		UserID: viper.GetString("audiobookshelf.user_id"),
		Enrich: viper.GetBool("stats.enrich"),
	}, nil
}

// runABSSync performs an Audiobookshelf sync and renders its summary.
func runABSSync(cmd *cobra.Command, full bool) error {
	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ing, err := newABSIngestor(database)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	res, err := ing.Sync(ctx, full)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if statsJSON {
		return json.NewEncoder(out).Encode(res)
	}
	if quiet {
		return nil
	}

	fmt.Fprintf(out, "Listening history (audiobookshelf):\n")
	fmt.Fprintf(out, "  Sessions:      %d\n", res.Sessions)
	fmt.Fprintf(out, "  Days:          %d\n", res.Days)
	fmt.Fprintf(out, "  Books:         %d\n", res.Books)
	if res.Enriched > 0 {
		fmt.Fprintf(out, "  Enriched:      %d items\n", res.Enriched)
	}
	if res.Incremental {
		fmt.Fprintf(out, "  Mode:          incremental since %s\n", res.Since)
	} else {
		fmt.Fprintf(out, "  Mode:          full\n")
	}
	hint(os.Stderr, "earworm stats status    # review stored listening data")
	return nil
}

func runStatsBackfill(cmd *cobra.Command, args []string) error {
	switch statsSource {
	case listening.SourceABS:
		return runABSSync(cmd, true)
	case listening.SourceAudible:
	default:
		return fmt.Errorf("unknown source %q (supported: audible, abs)", statsSource)
	}

	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ing, err := newAudibleIngestor(database)
	if err != nil {
		return err
	}

	if statsFullScan {
		// Clearing the watermark is what makes --full mean "start over"; the
		// stored rows themselves are corrected in place rather than deleted,
		// so a --full run is still safe to interrupt.
		if err := db.SetSyncState(database, "audible.daily.through", ""); err != nil {
			return err
		}
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if !quiet && !statsJSON {
		fmt.Fprintln(cmd.OutOrStdout(), "Backfilling Audible listening history...")
	}

	daily, err := ing.BackfillDaily(ctx)
	if err != nil {
		return fmt.Errorf("backfill daily totals: %w", err)
	}

	books, err := ing.SyncBooks(ctx)
	if err != nil {
		return fmt.Errorf("sync per-book state: %w", err)
	}

	summary := statsSummary{
		Source:          statsSource,
		WindowsFetched:  daily.WindowsFetched,
		DaysStored:      daily.DaysStored,
		From:            daily.From,
		Through:         daily.Through,
		Resumed:         daily.Resumed,
		Books:           books.Books,
		FinishedRecords: books.FinishedRecords,
		PositionsFound:  books.PositionsFound,
		BulkFlagged:     books.BulkFlagged,
	}
	return renderStatsSummary(cmd.OutOrStdout(), summary)
}

func runStatsSync(cmd *cobra.Command, args []string) error {
	switch statsSource {
	case listening.SourceABS:
		return runABSSync(cmd, statsFullScan)
	case listening.SourceAudible:
	default:
		return fmt.Errorf("unknown source %q (supported: audible, abs)", statsSource)
	}

	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ing, err := newAudibleIngestor(database)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Sync and backfill share a code path: the watermark already limits how
	// much a sync fetches, so a separate incremental routine would only be a
	// second thing to keep correct.
	daily, err := ing.BackfillDaily(ctx)
	if err != nil {
		return fmt.Errorf("sync daily totals: %w", err)
	}
	books, err := ing.SyncBooks(ctx)
	if err != nil {
		return fmt.Errorf("sync per-book state: %w", err)
	}

	return renderStatsSummary(cmd.OutOrStdout(), statsSummary{
		Source:          statsSource,
		WindowsFetched:  daily.WindowsFetched,
		DaysStored:      daily.DaysStored,
		From:            daily.From,
		Through:         daily.Through,
		Resumed:         daily.Resumed,
		Books:           books.Books,
		FinishedRecords: books.FinishedRecords,
		PositionsFound:  books.PositionsFound,
		BulkFlagged:     books.BulkFlagged,
	})
}

func runStatsStatus(cmd *cobra.Command, args []string) error {
	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	st := statsStatus{}
	for _, source := range []string{listening.SourceAudible, listening.SourceABS} {
		n, err := db.CountListeningDays(database, source)
		if err != nil {
			return err
		}
		books, err := db.ListBookListening(database, source)
		if err != nil {
			return err
		}
		days, err := db.ListListeningDays(database, source)
		if err != nil {
			return err
		}

		entry := statsSourceStatus{Source: source, Days: n, Books: len(books)}
		for _, d := range days {
			entry.TotalSeconds += d.Seconds
		}
		if len(days) > 0 {
			entry.FirstDay = days[0].Day
			entry.LastDay = days[len(days)-1].Day
		}
		for _, b := range books {
			if b.StatusIsBulk {
				entry.BulkFlagged++
			}
		}
		st.Sources = append(st.Sources, entry)
	}

	out := cmd.OutOrStdout()
	if statsJSON {
		return json.NewEncoder(out).Encode(st)
	}

	for _, s := range st.Sources {
		if s.Days == 0 && s.Books == 0 {
			fmt.Fprintf(out, "%-8s no data\n", s.Source)
			continue
		}
		fmt.Fprintf(out, "%-8s %d days, %d books, %.1f hours",
			s.Source, s.Days, s.Books, float64(s.TotalSeconds)/3600.0)
		if s.FirstDay != "" {
			fmt.Fprintf(out, " (%s to %s)", s.FirstDay, s.LastDay)
		}
		fmt.Fprintln(out)
		if s.BulkFlagged > 0 {
			fmt.Fprintf(out, "%-8s %d books have bulk-marked status timestamps (excluded from finish events)\n",
				"", s.BulkFlagged)
		}
	}
	return nil
}

type statsSummary struct {
	Source          string `json:"source"`
	WindowsFetched  int    `json:"windows_fetched"`
	DaysStored      int    `json:"days_stored"`
	From            string `json:"from,omitempty"`
	Through         string `json:"through,omitempty"`
	Resumed         bool   `json:"resumed"`
	Books           int    `json:"books"`
	FinishedRecords int    `json:"finished_records"`
	PositionsFound  int    `json:"positions_found"`
	BulkFlagged     int    `json:"bulk_flagged"`
}

type statsSourceStatus struct {
	Source       string `json:"source"`
	Days         int    `json:"days"`
	Books        int    `json:"books"`
	TotalSeconds int    `json:"total_seconds"`
	FirstDay     string `json:"first_day,omitempty"`
	LastDay      string `json:"last_day,omitempty"`
	BulkFlagged  int    `json:"bulk_flagged"`
}

type statsStatus struct {
	Sources []statsSourceStatus `json:"sources"`
}

func renderStatsSummary(w io.Writer, s statsSummary) error {
	if statsJSON {
		return json.NewEncoder(w).Encode(s)
	}
	if quiet {
		return nil
	}

	if s.Resumed {
		fmt.Fprintln(w, "Resumed from saved progress.")
	}
	fmt.Fprintf(w, "Listening history (%s):\n", s.Source)
	fmt.Fprintf(w, "  Days stored:   %d across %d windows\n", s.DaysStored, s.WindowsFetched)
	if s.From != "" {
		fmt.Fprintf(w, "  Range:         %s to %s\n", s.From, s.Through)
	}
	fmt.Fprintf(w, "  Books:         %d\n", s.Books)
	fmt.Fprintf(w, "  Finish records: %d\n", s.FinishedRecords)
	fmt.Fprintf(w, "  Last positions: %d\n", s.PositionsFound)
	if s.BulkFlagged > 0 {
		fmt.Fprintf(w, "  Bulk-marked:   %d books flagged (status timestamps not treated as finishes)\n", s.BulkFlagged)
	}

	hint(os.Stderr, "earworm stats status    # review stored listening data")
	return nil
}

// statusChecker is the subset of the Audiobookshelf client used by check.
type statusChecker interface {
	Status(ctx context.Context) (audiobookshelf.ServerStatus, error)
}

// newABSStatusClient is separated from newABSClient so tests can supply a
// checker without satisfying the whole session interface.
var newABSStatusClient = func() statusChecker {
	return audiobookshelf.NewClient(
		viper.GetString("audiobookshelf.url"),
		viper.GetString("audiobookshelf.token"),
		viper.GetString("audiobookshelf.library_id"),
	)
}

type absCheckResult struct {
	URL           string `json:"url"`
	Reachable     bool   `json:"reachable"`
	ServerVersion string `json:"server_version,omitempty"`
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"user_id,omitempty"`
	Username      string `json:"username,omitempty"`
	Error         string `json:"error,omitempty"`
}

func runStatsCheck(cmd *cobra.Command, args []string) error {
	url := viper.GetString("audiobookshelf.url")
	if url == "" {
		return fmt.Errorf("audiobookshelf.url is not configured (see 'earworm config set')")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	res := absCheckResult{URL: url}
	out := cmd.OutOrStdout()

	// Reachability is checked before credentials so the two failures are
	// distinguishable: the status endpoint needs no token.
	status, err := newABSStatusClient().Status(ctx)
	if err != nil {
		res.Error = err.Error()
		if statsJSON {
			return json.NewEncoder(out).Encode(res)
		}
		return fmt.Errorf("audiobookshelf unreachable at %s: %w", url, err)
	}
	res.Reachable = true
	res.ServerVersion = status.ServerVersion

	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ing, err := newABSIngestor(database)
	if err != nil {
		return err
	}
	userID, err := ing.ResolveUser(ctx)
	if err != nil {
		res.Error = err.Error()
		if statsJSON {
			return json.NewEncoder(out).Encode(res)
		}
		return fmt.Errorf("audiobookshelf reachable (version %s) but the token was not accepted: %w",
			status.ServerVersion, err)
	}
	res.Authenticated = true
	res.UserID = userID

	if statsJSON {
		return json.NewEncoder(out).Encode(res)
	}
	if quiet {
		return nil
	}
	fmt.Fprintf(out, "Audiobookshelf %s at %s\n", status.ServerVersion, url)
	fmt.Fprintf(out, "Token accepted for user %s\n", userID)
	hint(os.Stderr, "earworm stats backfill --source abs    # import session history")
	return nil
}
