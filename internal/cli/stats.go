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

func init() {
	statsBackfillCmd.Flags().StringVar(&statsSource, "source", "audible", "source to backfill (audible)")
	statsBackfillCmd.Flags().BoolVar(&statsJSON, "json", false, "output summary in JSON format")
	statsBackfillCmd.Flags().BoolVar(&statsFullScan, "full", false, "ignore the saved watermark and refetch everything")

	statsSyncCmd.Flags().StringVar(&statsSource, "source", "audible", "source to sync (audible)")
	statsSyncCmd.Flags().BoolVar(&statsJSON, "json", false, "output summary in JSON format")

	statsStatusCmd.Flags().BoolVar(&statsJSON, "json", false, "output status in JSON format")

	statsCmd.AddCommand(statsBackfillCmd)
	statsCmd.AddCommand(statsSyncCmd)
	statsCmd.AddCommand(statsStatusCmd)
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

func runStatsBackfill(cmd *cobra.Command, args []string) error {
	if statsSource != listening.SourceAudible {
		return fmt.Errorf("unknown source %q (supported: audible)", statsSource)
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
	if statsSource != listening.SourceAudible {
		return fmt.Errorf("unknown source %q (supported: audible)", statsSource)
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
