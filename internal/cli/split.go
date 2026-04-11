package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/split"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var splitJSON bool

var splitCmd = &cobra.Command{
	Use:   "split",
	Short: "Detect and split multi-book folders",
}

var splitDetectCmd = &cobra.Command{
	Use:   "detect [path]",
	Short: "Detect book groupings in a multi-book folder",
	Args:  cobra.ExactArgs(1),
	RunE:  runSplitDetect,
}

var splitPlanCmd = &cobra.Command{
	Use:   "plan [path]",
	Short: "Create a split plan for a multi-book folder",
	Long: `Create a split plan for a multi-book folder.
Run 'earworm split detect <path>' first to preview groupings before creating a plan.`,
	Args: cobra.ExactArgs(1),
	RunE: runSplitPlan,
}

func init() {
	splitDetectCmd.Flags().BoolVar(&splitJSON, "json", false, "output in JSON format")
	splitPlanCmd.Flags().BoolVar(&splitJSON, "json", false, "output in JSON format")
	splitCmd.AddCommand(splitDetectCmd, splitPlanCmd)
	rootCmd.AddCommand(splitCmd)
}

func runSplitDetect(cmd *cobra.Command, args []string) error {
	absPath, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	result, err := split.GroupFiles(absPath)
	if err != nil {
		return fmt.Errorf("split detect: %w", err)
	}

	if result.Skipped {
		fmt.Fprintf(cmd.OutOrStdout(), "Skipped: %s\n", result.SkipReason)
		return nil
	}

	if splitJSON {
		type jsonGroup struct {
			Title      string   `json:"title"`
			Author     string   `json:"author"`
			Narrator   string   `json:"narrator,omitempty"`
			ASIN       string   `json:"asin,omitempty"`
			Files      int      `json:"files"`
			AudioFiles []string `json:"audio_files"`
			Confidence float64  `json:"confidence"`
		}

		groups := make([]jsonGroup, len(result.Groups))
		for i, g := range result.Groups {
			groups[i] = jsonGroup{
				Title:      g.Title,
				Author:     g.Author,
				Narrator:   g.Narrator,
				ASIN:       g.ASIN,
				Files:      len(g.AudioFiles),
				AudioFiles: g.AudioFiles,
				Confidence: g.Confidence,
			}
		}

		out := struct {
			SourceDir   string      `json:"source_dir"`
			Groups      []jsonGroup `json:"groups"`
			SharedFiles []string    `json:"shared_files"`
		}{
			SourceDir:   result.SourceDir,
			Groups:      groups,
			SharedFiles: result.SharedFiles,
		}

		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Table format output
	fmt.Fprintf(cmd.OutOrStdout(), "Detected %d book groupings in: %s\n\n", len(result.Groups), result.SourceDir)
	fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-30s %-20s %-8s %-10s\n", "Group #", "Title", "Author", "Files", "Confidence")
	fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-30s %-20s %-8s %-10s\n", "---", "---", "---", "---", "---")
	for i, g := range result.Groups {
		author := g.Author
		if author == "" {
			author = "(unknown)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-8d %-30s %-20s %-8d %.0f%%\n",
			i+1, g.Title, author, len(g.AudioFiles), g.Confidence*100)
	}

	if len(result.SharedFiles) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nShared files (%d) will be copied to all groups.\n", len(result.SharedFiles))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nRun `earworm split plan %s` to create a plan from these groupings.\n", absPath)
	return nil
}

func runSplitPlan(cmd *cobra.Command, args []string) error {
	absPath, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	result, err := split.GroupFiles(absPath)
	if err != nil {
		return fmt.Errorf("split plan: %w", err)
	}

	if result.Skipped {
		return fmt.Errorf("folder skipped: %s. Use CSV import for manual planning", result.SkipReason)
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	libraryRoot := viper.GetString("library_path")
	planID, err := split.CreateSplitPlan(database, result, libraryRoot)
	if err != nil {
		return fmt.Errorf("create split plan: %w", err)
	}

	if splitJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"plan_id":    planID,
			"source_dir": absPath,
			"groups":     len(result.Groups),
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created plan %d. Review with: earworm plan review %d\n", planID, planID)
	return nil
}
