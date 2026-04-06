package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/download"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dryRun       bool
	downloadJSON bool
	limitN       int
	filterASINs  []string
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download new audiobooks from Audible",
	Long: `Download audiobooks that are in your Audible library but not yet
downloaded locally. Use --dry-run to preview what would be downloaded.`,
	RunE: runDownload,
}

func init() {
	downloadCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview downloads without downloading")
	downloadCmd.Flags().BoolVar(&downloadJSON, "json", false, "output in JSON format")
	downloadCmd.Flags().IntVar(&limitN, "limit", 0, "maximum number of books to download (0 = no limit)")
	downloadCmd.Flags().StringSliceVar(&filterASINs, "asin", nil, "download specific books by ASIN (repeatable)")
	rootCmd.AddCommand(downloadCmd)
}

// dryRunBook is the JSON output structure for dry-run mode.
type dryRunBook struct {
	ASIN           string `json:"asin"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	Narrators      string `json:"narrators"`
	RuntimeMinutes int    `json:"runtime_minutes"`
	SeriesName     string `json:"series_name,omitempty"`
	SeriesPosition string `json:"series_position,omitempty"`
}

func runDownload(cmd *cobra.Command, args []string) error {
	if dryRun {
		return runDryRun(cmd)
	}

	// Check ffmpeg is available (required for AAXC decryption).
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		hint := "Install ffmpeg to enable audiobook decryption:\n\n"
		if runtime.GOOS == "darwin" {
			hint += "  brew install ffmpeg"
		} else {
			hint += "  sudo apt install ffmpeg   # Debian/Ubuntu\n  sudo dnf install ffmpeg   # Fedora"
		}
		return fmt.Errorf("ffmpeg not found on PATH\n\n%s", hint)
	}

	// Validate library_path is configured.
	libraryPath := viper.GetString("library_path")
	if libraryPath == "" {
		return fmt.Errorf("library_path not configured\n\nRun: earworm config set library_path /path/to/audiobooks")
	}

	// Resolve staging path (default to ~/.config/earworm/staging).
	stagingPath := viper.GetString("staging_path")
	if stagingPath == "" {
		configDir, err := config.ConfigDir()
		if err != nil {
			return fmt.Errorf("failed to determine config directory: %w", err)
		}
		stagingPath = filepath.Join(configDir, "staging")
	}

	// Open database.
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Two-stage signal handling (D-05):
	// First SIGINT: cancel context (finish current book, stop batch).
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

	// Build PipelineConfig from Viper settings.
	cfg := download.PipelineConfig{
		StagingDir:        stagingPath,
		LibraryDir:        libraryPath,
		RateLimitSeconds:  viper.GetInt("download.rate_limit_seconds"),
		MaxRetries:        viper.GetInt("download.max_retries"),
		BackoffMultiplier: viper.GetFloat64("download.backoff_multiplier"),
		Quiet:             quiet,
		Limit:             limitN,
		FilterASINs:       filterASINs,
		TimeoutMinutes:    viper.GetInt("download.timeout_minutes"),
	}

	// Create audible client.
	client := newAudibleClient()

	// Create and run pipeline.
	pipeline := download.NewPipeline(client, database, cfg, cmd.OutOrStdout())
	summary, err := pipeline.Run(ctx)

	// Print summary always (D-04), even in quiet mode.
	if summary != nil {
		fmt.Fprintln(cmd.OutOrStdout(), summary.String())
		if len(summary.Errors) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "\nFailed downloads:")
			for _, e := range summary.Errors {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s - %s [%s]: %s\n", e.Author, e.Title, e.ASIN, e.Message)
			}
		}
		if summary.AuthFailed {
			fmt.Fprintln(cmd.OutOrStdout(), "\nRun `earworm auth` to re-authenticate.")
		}
	}

	return err
}

// runDryRun handles the --dry-run code path (list books without downloading).
func runDryRun(cmd *cobra.Command) error {
	// Open database.
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Get new books.
	books, err := db.ListNewBooks(database)
	if err != nil {
		return fmt.Errorf("failed to list new books: %w", err)
	}

	// Apply ASIN filter if specified.
	if len(filterASINs) > 0 {
		asinSet := make(map[string]bool, len(filterASINs))
		for _, a := range filterASINs {
			asinSet[a] = true
		}
		var filtered []db.Book
		for _, b := range books {
			if asinSet[b.ASIN] {
				filtered = append(filtered, b)
			}
		}
		books = filtered
	}

	// Apply limit if specified.
	if limitN > 0 && len(books) > limitN {
		books = books[:limitN]
	}

	// JSON output.
	if downloadJSON {
		var items []dryRunBook
		for _, b := range books {
			items = append(items, dryRunBook{
				ASIN:           b.ASIN,
				Title:          b.Title,
				Author:         b.Author,
				Narrators:      b.Narrators,
				RuntimeMinutes: b.RuntimeMinutes,
				SeriesName:     b.SeriesName,
				SeriesPosition: b.SeriesPosition,
			})
		}
		if items == nil {
			items = []dryRunBook{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Empty.
	if len(books) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No new books to download.")
		return nil
	}

	// Per D-08: Author - Title [ASIN] (runtime).
	for _, b := range books {
		runtime := formatRuntime(b.RuntimeMinutes)
		fmt.Fprintf(cmd.OutOrStdout(), "%s - %s [%s] (%s)\n", b.Author, b.Title, b.ASIN, runtime)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%d books to download\n", len(books))
	return nil
}

// formatRuntime formats minutes into "Xh Ym" display string.
func formatRuntime(minutes int) string {
	if minutes == 0 {
		return "unknown"
	}
	h := minutes / 60
	m := minutes % 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
