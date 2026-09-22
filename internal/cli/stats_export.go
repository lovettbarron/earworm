package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/lovettbarron/earworm/internal/bookidentity"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
	"github.com/lovettbarron/earworm/internal/statsexport"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	statsExportDir      string
	statsExportTimeline bool
	statsMatchesAll     bool
)

var statsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export listening history as CSV files",
	Long: `Write the collected listening history to CSV files for analysis.

Three normalised files are written -- books.csv, days.csv and sessions.csv --
plus a README explaining how to read them. Use --timeline to additionally write
a denormalised timeline.csv.

Every row carries the source it came from and an attribution quality saying
whether the book assignment was measured or reconstructed. Audiobookshelf
sessions name the book that was played; Audible reports only how long was
listened, so its days are attributed by inference or left unattributed.

Files are written to a local directory and are never transmitted anywhere.`,
	RunE: runStatsExport,
}

var statsMatchesCmd = &cobra.Command{
	Use:   "matches",
	Short: "Review how books were matched across sources",
	Long: `Show how source records were reconciled into books.

By default only the results worth a human glance are listed: books matched by
title rather than ASIN, and books that matched nothing. Use --all to list
every identity.`,
	RunE: runStatsMatches,
}

func init() {
	statsExportCmd.Flags().StringVarP(&statsExportDir, "output", "o", "", "output directory (default: stats.export_dir)")
	statsExportCmd.Flags().BoolVar(&statsExportTimeline, "timeline", false, "also write the denormalised timeline.csv")
	statsExportCmd.Flags().BoolVar(&statsJSON, "json", false, "output summary in JSON format")
	statsCmd.AddCommand(statsExportCmd)

	statsMatchesCmd.Flags().BoolVar(&statsMatchesAll, "all", false, "list every identity, not just uncertain ones")
	statsMatchesCmd.Flags().BoolVar(&statsJSON, "json", false, "output in JSON format")
	statsCmd.AddCommand(statsMatchesCmd)
}

// loadDataset reads everything an export needs and resolves book identities.
func loadDataset(database *sql.DB) (statsexport.Dataset, error) {
	var data statsexport.Dataset

	books, err := db.ListBookListening(database, "")
	if err != nil {
		return data, err
	}
	days, err := db.ListListeningDays(database, "")
	if err != nil {
		return data, err
	}
	sessions, err := db.ListListeningSessions(database)
	if err != nil {
		return data, err
	}

	records := make([]bookidentity.Record, 0, len(books))
	for _, b := range books {
		records = append(records, bookidentity.Record{
			Source:         b.Source,
			SourceKey:      b.SourceKey,
			ASIN:           b.ASIN,
			Title:          b.Title,
			Author:         b.Author,
			Series:         b.Series,
			SeriesPosition: b.SeriesPosition,
			Seconds:        b.SecondsListened,
		})
	}

	data.Books = books
	data.Days = days
	data.Sessions = sessions
	data.Identities = bookidentity.Resolve(records, bookidentity.DefaultOptions())
	return data, nil
}

func runStatsExport(cmd *cobra.Command, args []string) error {
	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	dir := statsExportDir
	if dir == "" {
		dir = viper.GetString("stats.export_dir")
	}
	if dir == "" {
		return fmt.Errorf("no output directory: set stats.export_dir or pass --output")
	}

	bucket, err := listening.NewBucketer(viper.GetString("stats.timezone"))
	if err != nil {
		return fmt.Errorf("stats.timezone is not a valid IANA timezone: %w", err)
	}

	data, err := loadDataset(database)
	if err != nil {
		return err
	}

	res, err := statsexport.Export(data, statsexport.Options{
		Dir:      dir,
		Timeline: statsExportTimeline,
		Bucket:   bucket,
	})
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

	fmt.Fprintf(out, "Exported to %s\n", res.Dir)
	for _, f := range res.Files {
		fmt.Fprintf(out, "  %s\n", f)
	}
	fmt.Fprintf(out, "\n  Books:         %d\n", res.Books)
	fmt.Fprintf(out, "  Day rows:      %d\n", res.Days)
	fmt.Fprintf(out, "  Sessions:      %d\n", res.Sessions)

	s := res.Stats
	fmt.Fprintf(out, "\n  Measured:      %.1f hours\n", float64(s.ExactSeconds)/3600.0)
	fmt.Fprintf(out, "  Inferred:      %.1f hours\n", float64(s.InferredSeconds)/3600.0)
	fmt.Fprintf(out, "  Unattributed:  %.1f hours\n", float64(s.UnattributedSeconds)/3600.0)

	hint(os.Stderr, "earworm stats matches    # review uncertain book matches")
	return nil
}

type matchRow struct {
	IdentityID string   `json:"identity_id"`
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	ASIN       string   `json:"asin,omitempty"`
	Method     string   `json:"match_method"`
	Sources    []string `json:"sources"`
}

func runStatsMatches(cmd *cobra.Command, args []string) error {
	database, err := openStatsDB()
	if err != nil {
		return err
	}
	defer database.Close()

	data, err := loadDataset(database)
	if err != nil {
		return err
	}

	var rows []matchRow
	for _, id := range data.Identities {
		method := id.Method()
		// ASIN matches are effectively certain, so they are noise in a review
		// listing unless the user asks for everything.
		if !statsMatchesAll && method == bookidentity.MatchASIN {
			continue
		}
		rows = append(rows, matchRow{
			IdentityID: id.ID, Title: id.Title, Author: id.Author,
			ASIN: id.ASIN, Method: method, Sources: id.Sources(),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Method != rows[j].Method {
			return rows[i].Method < rows[j].Method
		}
		return rows[i].Title < rows[j].Title
	})

	out := cmd.OutOrStdout()
	if statsJSON {
		return json.NewEncoder(out).Encode(rows)
	}
	if quiet {
		return nil
	}

	if len(rows) == 0 {
		fmt.Fprintln(out, "No matches to review.")
		return nil
	}

	fmt.Fprintf(out, "%-14s %-10s %-40s %s\n", "METHOD", "SOURCES", "TITLE", "AUTHOR")
	for _, r := range rows {
		srcs := ""
		for i, s := range r.Sources {
			if i > 0 {
				srcs += "+"
			}
			srcs += s
		}
		title := r.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Fprintf(out, "%-14s %-10s %-40s %s\n", r.Method, srcs, title, r.Author)
	}

	if !statsMatchesAll {
		fmt.Fprintf(out, "\n%d shown. ASIN matches are hidden; use --all to see them.\n", len(rows))
	}
	return nil
}
