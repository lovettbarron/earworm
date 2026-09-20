package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lovettbarron/earworm/internal/journal"
	"github.com/lovettbarron/earworm/internal/listening"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	journalWrite     bool
	journalDate      string
	journalSince     string
	journalUntil     string
	journalFinishes  bool
	journalPreview   bool
	journalEstimated bool
)

var statsJournalCmd = &cobra.Command{
	Use:   "journal",
	Short: "Write listening digests to Day One",
	Long: `Render listening history as journal entries and write them to Day One.

Dry-run by default. Nothing is written until you pass --write: this is the one
command that modifies a personal record outside earworm's own database.

Only measured facts become entries. Audiobookshelf sessions name the book that
was played, so those become daily digests. Audible reports how long you
listened but never to what, so its days are deliberately excluded rather than
guessed at -- that reconstruction lives in 'earworm stats export', where the
attribution column marks it as such.

Entries use a deterministic ID derived from the date, so re-running updates the
existing entry instead of adding a duplicate.`,
	RunE: runStatsJournal,
}

var statsJournalsCmd = &cobra.Command{
	Use:   "journals",
	Short: "List Day One journals available for writing",
	RunE:  runStatsJournals,
}

func init() {
	statsJournalCmd.Flags().BoolVar(&journalWrite, "write", false, "actually write entries (default is a dry run)")
	statsJournalCmd.Flags().StringVar(&journalDate, "date", "", "write a single day (YYYY-MM-DD, or 'today'/'yesterday')")
	statsJournalCmd.Flags().StringVar(&journalSince, "since", "", "earliest day to include (YYYY-MM-DD)")
	statsJournalCmd.Flags().StringVar(&journalUntil, "until", "", "latest day to include (YYYY-MM-DD)")
	statsJournalCmd.Flags().BoolVar(&journalFinishes, "finishes", false, "also write an entry for each finished book")
	statsJournalCmd.Flags().BoolVar(&journalEstimated, "estimated-finishes", false,
		"include finishes whose date is estimated from the last playback position")
	statsJournalCmd.Flags().BoolVar(&journalPreview, "preview", false, "print the rendered entries")
	statsJournalCmd.Flags().BoolVar(&statsJSON, "json", false, "output summary in JSON format")
	statsCmd.AddCommand(statsJournalCmd)

	statsJournalsCmd.Flags().BoolVar(&statsJSON, "json", false, "output in JSON format")
	statsCmd.AddCommand(statsJournalsCmd)
}

// newJournalWriter builds a Day One writer from config. Extracted for testing.
var newJournalWriter = func() journal.Writer {
	return journal.NewDayOne(viper.GetString("journal.cli_path"))
}

// newJournalLister builds a client that can enumerate journals.
var newJournalLister = func() interface {
	ListJournals(ctx context.Context) ([]journal.Journal, error)
} {
	return journal.NewDayOne(viper.GetString("journal.cli_path"))
}

// resolveJournalRange turns the date flags into a build range.
func resolveJournalRange(bucket *listening.Bucketer, now time.Time) (journal.BuildOptions, error) {
	opts := journal.BuildOptions{Since: journalSince, Until: journalUntil}

	if journalDate != "" {
		day := journalDate
		switch journalDate {
		case "today":
			day = bucket.Day(now)
		case "yesterday":
			day = bucket.Day(now.AddDate(0, 0, -1))
		default:
			if _, err := time.Parse(listening.DayFormat, journalDate); err != nil {
				return opts, fmt.Errorf("--date %q must be YYYY-MM-DD, 'today' or 'yesterday'", journalDate)
			}
		}
		opts.Since, opts.Until = day, day
	}

	for _, v := range []struct{ flag, value string }{
		{"--since", opts.Since}, {"--until", opts.Until},
	} {
		if v.value == "" {
			continue
		}
		if _, err := time.Parse(listening.DayFormat, v.value); err != nil {
			return opts, fmt.Errorf("%s %q must be YYYY-MM-DD", v.flag, v.value)
		}
	}
	return opts, nil
}

func runStatsJournal(cmd *cobra.Command, args []string) error {
	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	bucket, err := listening.NewBucketer(viper.GetString("stats.timezone"))
	if err != nil {
		return fmt.Errorf("stats.timezone is not a valid IANA timezone: %w", err)
	}

	buildOpts, err := resolveJournalRange(bucket, time.Now())
	if err != nil {
		return err
	}

	syncer := &journal.Syncer{
		DB:                database,
		JournalID:         viper.GetString("journal.journal_id"),
		DryRun:            !journalWrite,
		IncludeFinishes:   journalFinishes || viper.GetBool("journal.include_finishes"),
		EstimatedFinishes: journalEstimated || viper.GetBool("journal.estimated_finishes"),
	}
	if journalWrite {
		syncer.Writer = newJournalWriter()
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	res, err := syncer.Sync(ctx, buildOpts)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()

	if journalPreview {
		for _, e := range res.Entries {
			fmt.Fprintf(out, "%s\n", e.Body)
		}
	}

	if statsJSON {
		return json.NewEncoder(out).Encode(res)
	}
	if quiet {
		return nil
	}

	if res.Rendered == 0 {
		fmt.Fprintln(out, "No entries to write.")
		return nil
	}

	if res.DryRun {
		fmt.Fprintf(out, "Dry run — nothing written.\n")
		fmt.Fprintf(out, "  Would create: %d\n", res.Written)
		fmt.Fprintf(out, "  Would update: %d\n", res.Updated)
		fmt.Fprintf(out, "  Unchanged:    %d\n", res.Unchanged)
		hint(os.Stderr, "earworm stats journal --write    # write these entries to Day One")
		return nil
	}

	fmt.Fprintf(out, "Journal updated.\n")
	fmt.Fprintf(out, "  Created:   %d\n", res.Written)
	fmt.Fprintf(out, "  Updated:   %d\n", res.Updated)
	fmt.Fprintf(out, "  Unchanged: %d\n", res.Unchanged)
	return nil
}

func runStatsJournals(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	journals, err := newJournalLister().ListJournals(ctx)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if statsJSON {
		return json.NewEncoder(out).Encode(journals)
	}
	if quiet {
		return nil
	}

	if len(journals) == 0 {
		fmt.Fprintln(out, "No journals found. Is the Day One CLI signed in?")
		return nil
	}

	fmt.Fprintf(out, "%-14s %-24s %s\n", "ID", "NAME", "ENCRYPTION")
	for _, j := range journals {
		fmt.Fprintf(out, "%-14s %-24s %s\n", j.ID, j.Name, j.Encryption)
	}
	hint(os.Stderr, "earworm config set journal.journal_id <id>    # choose a destination")
	return nil
}
