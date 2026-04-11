package cli

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/planengine"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cleanupPlanID    int64
	cleanupPermanent bool
	cleanupJSON      bool
)

// stdinReader is the default reader for confirmation prompts.
// Override in tests to inject input.
var stdinReader io.Reader = os.Stdin

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Move delete-marked files to trash directory",
	Long: `Processes delete operations from completed plans by moving files to a trash
directory. Requires double confirmation before any files are moved.

Only processes delete operations from plans with status "completed".
Files are moved to a trash directory by default, preserving reversibility.
Use --permanent to permanently delete files instead (DANGEROUS).`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().Int64Var(&cleanupPlanID, "plan-id", 0, "only process deletes from this plan")
	cleanupCmd.Flags().BoolVar(&cleanupPermanent, "permanent", false, "permanently delete instead of moving to trash (DANGEROUS)")
	cleanupCmd.Flags().BoolVar(&cleanupJSON, "json", false, "output in JSON format")
	rootCmd.AddCommand(cleanupCmd)
}

// confirmCleanup asks the user to confirm cleanup with double confirmation.
// Returns true only if both confirmations are affirmative.
func confirmCleanup(w io.Writer, r io.Reader, fileCount int) bool {
	scanner := bufio.NewScanner(r)

	fmt.Fprintf(w, "\nMove %d files to trash? [y/N]: ", fileCount)
	if !scanner.Scan() {
		return false
	}
	resp := strings.TrimSpace(scanner.Text())
	if resp != "y" && resp != "Y" {
		return false
	}

	fmt.Fprintf(w, "Are you sure? This removes files from the library. [y/N]: ")
	if !scanner.Scan() {
		return false
	}
	resp = strings.TrimSpace(scanner.Text())
	return resp == "y" || resp == "Y"
}

func runCleanup(cmd *cobra.Command, args []string) error {
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	trashDir := viper.GetString("cleanup.trash_dir")
	executor := &planengine.CleanupExecutor{DB: database, TrashDir: trashDir}

	ops, err := executor.ListPending(cleanupPlanID)
	if err != nil {
		return fmt.Errorf("failed to list pending cleanup: %w", err)
	}

	if len(ops) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No files pending cleanup.")
		return nil
	}

	// Display file list
	fmt.Fprintf(cmd.OutOrStdout(), "Files pending cleanup (%d):\n", len(ops))
	for _, op := range ops {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", op.SourcePath)
	}

	// Ask for confirmation
	if !confirmCleanup(cmd.OutOrStdout(), stdinReader, len(ops)) {
		fmt.Fprintln(cmd.OutOrStdout(), "Cleanup cancelled.")
		return nil
	}

	// Execute cleanup
	var result *planengine.CleanupResult
	if cleanupPermanent {
		fmt.Fprintln(cmd.OutOrStdout(), "WARNING: Permanently deleting files (--permanent flag set)")
		result, err = executePermanentDelete(database, ops)
	} else {
		result, err = executor.Execute(ops)
	}
	if err != nil {
		return fmt.Errorf("cleanup execution failed: %w", err)
	}

	// Output results
	if cleanupJSON {
		type jsonResult struct {
			Moved   int      `json:"moved"`
			Skipped int      `json:"skipped"`
			Errors  []string `json:"errors"`
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(jsonResult{
			Moved:   result.Moved,
			Skipped: result.Skipped,
			Errors:  result.Errors,
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nMoved: %d, Skipped: %d, Errors: %d\n",
		result.Moved, result.Skipped, len(result.Errors))
	for _, e := range result.Errors {
		fmt.Fprintf(cmd.OutOrStdout(), "  Error: %s\n", e)
	}

	return nil
}

// executePermanentDelete permanently removes files instead of moving to trash.
// Each deletion (success, failure, or skip) is recorded in the audit log.
func executePermanentDelete(database *sql.DB, ops []db.PlanOperation) (*planengine.CleanupResult, error) {
	result := &planengine.CleanupResult{}

	for _, op := range ops {
		entityID := strconv.FormatInt(op.ID, 10)
		beforeJSON, _ := json.Marshal(map[string]string{
			"source_path": op.SourcePath,
			"action":      "permanent_delete",
		})

		if _, err := os.Stat(op.SourcePath); os.IsNotExist(err) {
			result.Skipped++
			_ = db.UpdateOperationStatus(database, op.ID, "skipped", "file not found")
			afterJSON, _ := json.Marshal(map[string]string{
				"skipped": "file not found",
			})
			_ = db.LogAudit(database, db.AuditEntry{
				EntityType:  "operation",
				EntityID:    entityID,
				Action:      "permanent_delete",
				BeforeState: string(beforeJSON),
				AfterState:  string(afterJSON),
				Success:     true,
			})
			continue
		}

		if err := os.Remove(op.SourcePath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", op.SourcePath, err))
			_ = db.UpdateOperationStatus(database, op.ID, "failed", err.Error())
			afterJSON, _ := json.Marshal(map[string]string{
				"error": err.Error(),
			})
			_ = db.LogAudit(database, db.AuditEntry{
				EntityType:   "operation",
				EntityID:     entityID,
				Action:       "permanent_delete",
				BeforeState:  string(beforeJSON),
				AfterState:   string(afterJSON),
				Success:      false,
				ErrorMessage: err.Error(),
			})
			continue
		}

		result.Moved++
		_ = db.UpdateOperationStatus(database, op.ID, "completed", "")
		afterJSON, _ := json.Marshal(map[string]string{
			"deleted": "true",
		})
		_ = db.LogAudit(database, db.AuditEntry{
			EntityType:  "operation",
			EntityID:    entityID,
			Action:      "permanent_delete",
			BeforeState: string(beforeJSON),
			AfterState:  string(afterJSON),
			Success:     true,
		})
	}

	return result, nil
}
