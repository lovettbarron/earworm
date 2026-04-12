package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/metadata"
	"github.com/lovettbarron/earworm/internal/scanner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var scanRecursive bool
var scanDeep bool
var scanJSON bool
var scanIssuesJSON bool
var scanCreatePlan bool
var scanFilterType string

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan local library for audiobooks",
	Long: `Scan a Libation-compatible audiobook library directory and index all
discovered books into the local database. The library is expected to follow
the structure: Author Name/Book Title [ASIN]/book.m4a

Use --recursive to search deeply nested directory structures.`,
	RunE: runScan,
}

var scanIssuesCmd = &cobra.Command{
	Use:   "issues",
	Short: "List detected issues from the last deep scan",
	Long: `List issues detected by the last 'earworm scan --deep' run.
Use --type to filter by issue type (e.g., nested_audio, empty_dir).
Use --create-plan to generate a remediation plan from actionable issues.`,
	RunE: runScanIssues,
}

func init() {
	scanCmd.Flags().BoolVarP(&scanRecursive, "recursive", "r", false, "recursively scan nested directories")
	scanCmd.Flags().BoolVar(&scanDeep, "deep", false, "scan all folders including those without ASINs and detect issues")
	scanCmd.Flags().BoolVar(&scanJSON, "json", false, "output in JSON format (only with --deep)")

	scanIssuesCmd.Flags().BoolVar(&scanIssuesJSON, "json", false, "output in JSON format")
	scanIssuesCmd.Flags().BoolVar(&scanCreatePlan, "create-plan", false, "create a plan from detected issues")
	scanIssuesCmd.Flags().StringVar(&scanFilterType, "type", "", "filter issues by type")
	scanCmd.AddCommand(scanIssuesCmd)

	rootCmd.AddCommand(scanCmd)
}

// deepScanJSON is the JSON output structure for scan --deep --json.
type deepScanJSON struct {
	TotalDirs   int              `json:"total_dirs"`
	WithASIN    int              `json:"with_asin"`
	WithoutASIN int              `json:"without_asin"`
	IssuesFound int              `json:"issues_found"`
	IssueCounts map[string]int   `json:"issue_counts"`
	Issues      []scanIssueJSON  `json:"issues"`
}

// scanIssueJSON is the JSON representation of a single scan issue.
type scanIssueJSON struct {
	ID              int64  `json:"id"`
	Path            string `json:"path"`
	IssueType       string `json:"issue_type"`
	Severity        string `json:"severity"`
	Message         string `json:"message"`
	SuggestedAction string `json:"suggested_action"`
}

func runScan(cmd *cobra.Command, args []string) error {
	// Get library path from config
	libPath := viper.GetString("library_path")
	if libPath == "" {
		return fmt.Errorf("library path not configured\n\nRun 'earworm config set library_path /path/to/library' to set your library location")
	}

	// Validate library path exists and is a directory
	info, err := os.Stat(libPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("library path %q does not exist or is not a directory\n\nCheck that your library path is correct and accessible: earworm config show", libPath)
	}

	// Open database
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w\n\nTry removing %s and running again", err, dbPath)
	}
	defer database.Close()

	// Deep scan mode: walk all directories, detect issues, persist results
	if scanDeep {
		return runDeepScan(cmd, database, libPath)
	}

	// Start spinner for progress feedback on stderr
	spin := NewSpinner(cmd.ErrOrStderr(), "Scanning")
	if !quiet {
		spin.Start()
	}

	// Scan library directory
	discovered, skipped, err := scanner.ScanLibrary(libPath, scanRecursive)
	if err != nil {
		if !quiet {
			spin.Stop()
		}
		return fmt.Errorf("scan failed: %w\n\nCheck that the library path is accessible and you have read permissions", err)
	}

	// Update spinner count and stop
	for range discovered {
		spin.Increment()
	}
	if !quiet {
		spin.Stop()
		fmt.Fprintf(cmd.ErrOrStderr(), "Found %d books, extracting metadata...\n", len(discovered))
	}

	// Adapter: convert metadata.ExtractMetadata to scanner.BookMetadata
	metadataFn := func(bookDir string) (*scanner.BookMetadata, error) {
		meta, err := metadata.ExtractMetadata(bookDir)
		if err != nil {
			return nil, err
		}
		return &scanner.BookMetadata{
			Title:        meta.Title,
			Author:       meta.Author,
			Narrator:     meta.Narrator,
			Genre:        meta.Genre,
			Year:         meta.Year,
			Series:       meta.Series,
			HasCover:     meta.HasCover,
			Duration:     meta.Duration,
			ChapterCount: meta.ChapterCount,
			FileCount:    meta.FileCount,
			Source:       string(meta.Source),
		}, nil
	}

	// Incremental sync to database
	result, err := scanner.IncrementalSync(database, discovered, metadataFn)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Print summary to stdout
	fmt.Fprintf(cmd.OutOrStdout(), "Scan complete:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Added:   %d\n", result.Added)
	fmt.Fprintf(cmd.OutOrStdout(), "  Updated: %d\n", result.Updated)
	fmt.Fprintf(cmd.OutOrStdout(), "  Removed: %d\n", result.Removed)
	fmt.Fprintf(cmd.OutOrStdout(), "  Skipped: %d\n", result.Skipped)

	// Print skipped folders if any and not quiet
	if len(skipped) > 0 && !quiet {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nSkipped %d folders:\n", len(skipped))
		limit := 10
		if len(skipped) < limit {
			limit = len(skipped)
		}
		for _, s := range skipped[:limit] {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s (%s)\n", s.Path, s.Reason)
		}
		if len(skipped) > 10 {
			fmt.Fprintf(cmd.ErrOrStderr(), "  ... and %d more\n", len(skipped)-10)
		}
	}

	return nil
}

func runDeepScan(cmd *cobra.Command, database *sql.DB, libPath string) error {
	// Start spinner (skip in JSON mode to keep output clean)
	spin := NewSpinner(cmd.ErrOrStderr(), "Deep scanning")
	if !quiet && !scanJSON {
		spin.Start()
	}

	// Adapter for metadata extraction (same as existing scan)
	metadataFn := func(bookDir string) (*scanner.BookMetadata, error) {
		meta, err := metadata.ExtractMetadata(bookDir)
		if err != nil {
			return nil, err
		}
		return &scanner.BookMetadata{
			Title:        meta.Title,
			Author:       meta.Author,
			Narrator:     meta.Narrator,
			Genre:        meta.Genre,
			Year:         meta.Year,
			Series:       meta.Series,
			HasCover:     meta.HasCover,
			Duration:     meta.Duration,
			ChapterCount: meta.ChapterCount,
			FileCount:    meta.FileCount,
			Source:       string(meta.Source),
		}, nil
	}

	result, err := scanner.DeepScanLibrary(libPath, database, metadataFn)
	if !quiet && !scanJSON {
		spin.Stop()
	}
	if err != nil {
		return fmt.Errorf("deep scan failed: %w", err)
	}

	// JSON output mode
	if scanJSON {
		// Query persisted issues for full details
		issues, err := db.ListScanIssues(database)
		if err != nil {
			return fmt.Errorf("list scan issues: %w", err)
		}

		// Convert IssueCounts keys from scanner.IssueType to string
		issueCounts := make(map[string]int, len(result.IssueCounts))
		for k, v := range result.IssueCounts {
			issueCounts[string(k)] = v
		}

		// Build JSON issue list
		jsonIssues := make([]scanIssueJSON, len(issues))
		for i, issue := range issues {
			jsonIssues[i] = scanIssueJSON{
				ID:              issue.ID,
				Path:            issue.Path,
				IssueType:       issue.IssueType,
				Severity:        issue.Severity,
				Message:         issue.Message,
				SuggestedAction: issue.SuggestedAction,
			}
		}

		out := deepScanJSON{
			TotalDirs:   result.TotalDirs,
			WithASIN:    result.WithASIN,
			WithoutASIN: result.WithoutASIN,
			IssuesFound: result.IssuesFound,
			IssueCounts: issueCounts,
			Issues:      jsonIssues,
		}

		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Print summary to stdout
	fmt.Fprintf(cmd.OutOrStdout(), "Deep scan complete:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Directories: %d\n", result.TotalDirs)
	fmt.Fprintf(cmd.OutOrStdout(), "  With ASIN:   %d\n", result.WithASIN)
	fmt.Fprintf(cmd.OutOrStdout(), "  Without ASIN: %d\n", result.WithoutASIN)
	fmt.Fprintf(cmd.OutOrStdout(), "  Issues found: %d\n", result.IssuesFound)

	// Print issue breakdown if any (to stderr, for non-quiet mode)
	if result.IssuesFound > 0 && !quiet {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nIssues by type:\n")
		for issueType, count := range result.IssueCounts {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %-20s %d\n", issueType, count)
		}
	}

	return nil
}

func runScanIssues(cmd *cobra.Command, args []string) error {
	// Open database
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Fetch issues (filtered or all)
	var issues []db.ScanIssue
	if scanFilterType != "" {
		issues, err = db.ListScanIssuesByType(database, scanFilterType)
	} else {
		issues, err = db.ListScanIssues(database)
	}
	if err != nil {
		return fmt.Errorf("list scan issues: %w", err)
	}

	// Create plan mode
	if scanCreatePlan {
		result, err := scanner.CreatePlanFromIssues(database, issues)
		if err != nil {
			return fmt.Errorf("create plan: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Plan created: ID %d\n", result.PlanID)
		fmt.Fprintf(cmd.OutOrStdout(), "  %d operations created\n", result.Created)
		fmt.Fprintf(cmd.OutOrStdout(), "  %d issues skipped (require manual review)\n", result.Skipped)
		return nil
	}

	// JSON output mode
	if scanIssuesJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(issues)
	}

	// Human-readable output
	if len(issues) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No issues found.\n")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Scan issues (%d):\n", len(issues))
	for _, issue := range issues {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-12s %-8s %s\n    %s\n", issue.IssueType, issue.Severity, issue.Path, issue.Message)
	}

	return nil
}
