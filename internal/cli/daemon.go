package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lovettbarron/earworm/internal/audiobookshelf"
	"github.com/lovettbarron/earworm/internal/daemon"
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
	cycle := func(_ context.Context) error {
		// Step 1: Sync
		slog.Info("daemon: running sync")
		if err := runSync(cmd, nil); err != nil {
			slog.Warn("daemon: sync failed", "error", err)
		}

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

		return nil
	}

	// Single cycle mode.
	if daemonOnce {
		return cycle(ctx)
	}

	// Continuous polling.
	return daemon.Run(ctx, interval, cycle, daemonVerbose)
}
