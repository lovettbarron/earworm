package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/daemon"
	"github.com/lovettbarron/earworm/internal/listening"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	daemonVerbose  bool
	daemonOnce     bool
	daemonInterval string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run in polling mode (sync, download, organize, notify)",
	Long: `Run earworm in daemon/polling mode. Each cycle runs the full pipeline:
sync -> download -> organize -> notify (Audiobookshelf scan).

The default polling interval is 6 hours. Use --interval to override.
Use --once to run a single cycle and exit.`,
	RunE: runDaemon,
}

func init() {
	daemonCmd.Flags().BoolVar(&daemonVerbose, "verbose", false, "log heartbeat messages between cycles")
	daemonCmd.Flags().BoolVar(&daemonOnce, "once", false, "run one cycle then exit")
	daemonCmd.Flags().StringVar(&daemonInterval, "interval", "", "polling interval (e.g. 6h, 30m) — overrides config")
	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	// Parse interval.
	intervalStr := daemonInterval
	if intervalStr == "" {
		intervalStr = viper.GetString("daemon.polling_interval")
	}
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return fmt.Errorf("invalid polling interval %q: %w", intervalStr, err)
	}

	// Two-stage signal handling (D-13):
	// First SIGINT: cancel context (finish current operation, stop batch).
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Second SIGINT: force exit.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan // first signal consumed by NotifyContext
		<-sigChan // second signal: force exit
		fmt.Fprintln(os.Stderr, "\nForce stopping...")
		os.Exit(1)
	}()
	defer signal.Stop(sigChan)

	// Define the cycle function that runs the full pipeline.
	cycle := func(cycleCtx context.Context) error {
		// The library path is checked once per cycle. Steps that write to it
		// are skipped when it is unreachable, rather than blocking the daemon:
		// a syscall on a dead network mount cannot be interrupted, so a single
		// hung step would silently end all future cycles.
		libraryUp := true
		if err := ensureLibraryAvailable(cycleCtx, os.Stderr); err != nil {
			libraryUp = false
			slog.Warn("daemon: library unavailable, skipping library steps",
				"error", err)
		}

		// Step 1: Sync
		slog.Info("daemon: running sync")
		if err := runSync(cmd, nil); err != nil {
			slog.Warn("daemon: sync failed", "error", err)
		}

		if libraryUp {
			// Step 2: Download (includes organize hook if wired)
			slog.Info("daemon: running download")
			if err := runDownload(cmd, nil); err != nil {
				slog.Warn("daemon: download failed", "error", err)
			}

			// Step 3: Organize
			slog.Info("daemon: running organize")
			if err := runOrganize(cmd, nil); err != nil {
				slog.Warn("daemon: organize failed", "error", err)
			}
		} else {
			slog.Info("daemon: skipped download and organize (library unavailable)")
		}

		// Step 4: Notify ABS
		if absURL := viper.GetString("audiobookshelf.url"); absURL != "" {
			slog.Info("daemon: triggering Audiobookshelf scan")
			abs := audiobookshelf.NewClient(
				absURL,
				viper.GetString("audiobookshelf.token"),
				viper.GetString("audiobookshelf.library_id"),
			)
			if err := abs.ScanLibrary(); err != nil {
				slog.Warn("Audiobookshelf scan failed", "error", err)
			}
		}

		// Step 5: Listening and reading stats, and optionally the journal.
		//
		// These reach networked services and the local database, never the
		// library mount, so they run even when the library is down. That is
		// the point of checking rather than hanging: an unreachable NAS should
		// not stop the parts of the cycle that have nothing to do with it.
		if viper.GetBool("daemon.stats_sync") {
			runDaemonStatsCycle(cmd)
		}

		return nil
	}

	// Single cycle mode.
	if daemonOnce {
		return cycle(ctx)
	}

	// Continuous polling.
	return daemon.Run(ctx, interval, cycle, daemonVerbose)
}

// statsCycleRunning guards the stats step against overlapping runs.
//
// A backfill can outlast a polling interval, and two concurrent syncs would
// interleave writes to the same rows and the same journal entries. The guard
// makes a slow cycle skip the next one rather than race it.
var statsCycleRunning atomic.Bool

// runDaemonStatsCycle syncs listening history and, when explicitly enabled,
// writes journal entries.
//
// Every failure is logged rather than returned: the stats step is an extra on
// top of the download pipeline, and a listening-history hiccup should not stop
// the daemon from fetching books.
func runDaemonStatsCycle(cmd *cobra.Command) {
	if !statsCycleRunning.CompareAndSwap(false, true) {
		slog.Warn("daemon: stats sync still running from a previous cycle, skipping")
		return
	}
	defer statsCycleRunning.Store(false)

	slog.Info("daemon: syncing listening stats")
	origSource := statsSource
	defer func() { statsSource = origSource }()

	statsSource = listening.SourceAudible
	if err := runStatsSync(cmd, nil); err != nil {
		slog.Warn("daemon: audible stats sync failed", "error", err)
	}

	if viper.GetString("audiobookshelf.url") != "" && viper.GetString("audiobookshelf.token") != "" {
		statsSource = listening.SourceABS
		if err := runStatsSync(cmd, nil); err != nil {
			slog.Warn("daemon: audiobookshelf stats sync failed", "error", err)
		}
	}

	// Reading must be refreshed before the journal step, or tonight's entry
	// would describe yesterday's reading.
	if viper.GetString("komga.url") != "" && viper.GetString("komga.api_key") != "" {
		statsSource = listening.SourceKomga
		if err := runStatsSync(cmd, nil); err != nil {
			slog.Warn("daemon: komga reading sync failed", "error", err)
		}
	}

	// Journal writes stay opt-in. The daemon should not modify a personal
	// record unattended unless the user has said so.
	if !viper.GetBool("journal.daemon_write") {
		return
	}
	if viper.GetString("journal.journal_id") == "" {
		slog.Warn("daemon: journal.daemon_write is on but journal.journal_id is unset, skipping")
		return
	}

	slog.Info("daemon: writing journal entries")
	origWrite := journalWrite
	defer func() { journalWrite = origWrite }()
	journalWrite = true
	if err := runStatsJournal(cmd, nil); err != nil {
		slog.Warn("daemon: journal write failed", "error", err)
	}
}
