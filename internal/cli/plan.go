package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lovettbarron/earworm/internal/config"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/planengine"
	"github.com/spf13/cobra"
)

var (
	planConfirm    bool
	planJSON       bool
	planStatus     string
	planImportName string
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage library cleanup plans",
	Long: `Manage library cleanup plans. Plans contain a set of operations
(move, flatten, delete, write_metadata) that can be reviewed before applying.`,
}

var planListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all plans",
	RunE:  runPlanList,
}

var planReviewCmd = &cobra.Command{
	Use:   "review [plan-id]",
	Short: "Review a plan's operations before applying",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlanReview,
}

var planApplyCmd = &cobra.Command{
	Use:   "apply [plan-id]",
	Short: "Apply a plan's operations",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlanApply,
}

var planImportCmd = &cobra.Command{
	Use:   "import FILE.csv",
	Short: "Import a plan from a CSV file",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlanImport,
}

var planApproveCmd = &cobra.Command{
	Use:   "approve [plan-id]",
	Short: "Approve a draft plan for execution",
	Long:  "Transition a plan from draft to ready status so it can be applied.",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlanApprove,
}

func init() {
	planListCmd.Flags().BoolVar(&planJSON, "json", false, "output in JSON format")
	planListCmd.Flags().StringVar(&planStatus, "status", "", "filter by plan status")
	planReviewCmd.Flags().BoolVar(&planJSON, "json", false, "output in JSON format")
	planApplyCmd.Flags().BoolVar(&planConfirm, "confirm", false, "actually apply the plan (default is dry-run)")
	planApplyCmd.Flags().BoolVar(&planJSON, "json", false, "output in JSON format")
	planImportCmd.Flags().StringVar(&planImportName, "name", "", "plan name (defaults to filename without extension)")
	planImportCmd.Flags().BoolVar(&planJSON, "json", false, "output in JSON format")
	planApproveCmd.Flags().BoolVar(&planJSON, "json", false, "output in JSON format")
	planCmd.AddCommand(planListCmd, planReviewCmd, planApplyCmd, planImportCmd, planApproveCmd)
	rootCmd.AddCommand(planCmd)
}

func runPlanList(cmd *cobra.Command, args []string) error {
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	plans, err := db.ListPlans(database, planStatus)
	if err != nil {
		return fmt.Errorf("failed to list plans: %w", err)
	}

	if planJSON {
		type jsonPlan struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Status      string `json:"status"`
			CreatedAt   string `json:"created_at"`
		}
		items := make([]jsonPlan, len(plans))
		for i, p := range plans {
			items[i] = jsonPlan{
				ID:          p.ID,
				Name:        p.Name,
				Description: p.Description,
				Status:      p.Status,
				CreatedAt:   p.CreatedAt.Format("2006-01-02 15:04:05"),
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	if len(plans) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No plans found.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-30s %-12s %s\n", "ID", "Name", "Status", "Created")
	fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-30s %-12s %s\n", "---", "---", "---", "---")
	for _, p := range plans {
		fmt.Fprintf(cmd.OutOrStdout(), "%-6d %-30s %-12s %s\n",
			p.ID, p.Name, p.Status, p.CreatedAt.Format("2006-01-02 15:04"))
	}
	return nil
}

func runPlanReview(cmd *cobra.Command, args []string) error {
	planID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid plan ID %q: %w", args[0], err)
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

	plan, err := db.GetPlan(database, planID)
	if err != nil {
		return fmt.Errorf("failed to get plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("plan %d not found", planID)
	}

	ops, err := db.ListOperations(database, planID)
	if err != nil {
		return fmt.Errorf("failed to list operations: %w", err)
	}

	if planJSON {
		result := struct {
			Plan       *db.Plan            `json:"plan"`
			Operations []db.PlanOperation  `json:"operations"`
		}{
			Plan:       plan,
			Operations: ops,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Plan #%d: %s\n", plan.ID, plan.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", plan.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "Operations: %d\n\n", len(ops))

	fmt.Fprintf(cmd.OutOrStdout(), "%-4s %-16s %-10s %s\n", "#", "Type", "Status", "Path")
	fmt.Fprintf(cmd.OutOrStdout(), "%-4s %-16s %-10s %s\n", "---", "---", "---", "---")
	for _, op := range ops {
		pathStr := op.SourcePath
		if op.DestPath != "" {
			pathStr = op.SourcePath + " -> " + op.DestPath
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-4d %-16s %-10s %s\n",
			op.Seq, op.OpType, op.Status, pathStr)
	}
	return nil
}

func runPlanApply(cmd *cobra.Command, args []string) error {
	planID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid plan ID %q: %w", args[0], err)
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

	// If no --confirm, show dry-run output (same as review) and exit.
	if !planConfirm {
		plan, err := db.GetPlan(database, planID)
		if err != nil {
			return fmt.Errorf("failed to get plan: %w", err)
		}
		if plan == nil {
			return fmt.Errorf("plan %d not found", planID)
		}

		ops, err := db.ListOperations(database, planID)
		if err != nil {
			return fmt.Errorf("failed to list operations: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Plan #%d: %s\n", plan.ID, plan.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", plan.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "Operations: %d\n\n", len(ops))

		fmt.Fprintf(cmd.OutOrStdout(), "%-4s %-16s %-10s %s\n", "#", "Type", "Status", "Path")
		fmt.Fprintf(cmd.OutOrStdout(), "%-4s %-16s %-10s %s\n", "---", "---", "---", "---")
		for _, op := range ops {
			pathStr := op.SourcePath
			if op.DestPath != "" {
				pathStr = op.SourcePath + " -> " + op.DestPath
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-4d %-16s %-10s %s\n",
				op.Seq, op.OpType, op.Status, pathStr)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\nDry run — no changes made. Add --confirm to apply this plan.\n")
		return nil
	}

	// --confirm: actually execute the plan.
	executor := &planengine.Executor{DB: database}
	results, err := executor.Apply(cmd.Context(), planID)
	if err != nil {
		return fmt.Errorf("failed to apply plan: %w", err)
	}

	if planJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	completed := 0
	failed := 0
	for i, r := range results {
		status := "completed"
		if !r.Success {
			status = "failed"
			failed++
		} else {
			completed++
		}
		detail := ""
		if r.SHA256 != "" {
			detail = fmt.Sprintf(" [sha256:%s]", r.SHA256[:12])
		}
		if r.Error != "" {
			detail = fmt.Sprintf(" error: %s", r.Error)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s%s\n", i+1, status, detail)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nApplied: %d completed, %d failed out of %d operations\n",
		completed, failed, len(results))
	return nil
}

func runPlanImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Derive plan name from filename if not provided.
	planName := planImportName
	if planName == "" {
		base := filepath.Base(filePath)
		planName = strings.TrimSuffix(base, filepath.Ext(base))
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open CSV file: %w", err)
	}
	defer f.Close()

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("failed to determine database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	result, err := planengine.ImportCSV(database, planName, f)
	if err != nil {
		return fmt.Errorf("CSV import failed: %w", err)
	}

	if result.ErrorCount > 0 {
		for _, e := range result.Errors {
			fmt.Fprintf(cmd.ErrOrStderr(), "line %d: %s: %s\n", e.Line, e.Column, e.Message)
		}
		return fmt.Errorf("CSV import failed: %d validation errors", result.ErrorCount)
	}

	if planJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"plan_id":   result.PlanID,
			"name":      planName,
			"row_count": result.RowCount,
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created plan %d (%q) with %d operations\n",
		result.PlanID, planName, result.RowCount)
	return nil
}

func runPlanApprove(cmd *cobra.Command, args []string) error {
	planID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid plan ID %q: %w", args[0], err)
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

	plan, err := db.GetPlan(database, planID)
	if err != nil {
		return fmt.Errorf("failed to get plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("plan %d not found", planID)
	}
	if plan.Status != "draft" {
		return fmt.Errorf("can only approve draft plans, current status: %s", plan.Status)
	}

	if err := db.UpdatePlanStatusAudited(database, planID, "ready"); err != nil {
		return fmt.Errorf("failed to approve plan: %w", err)
	}

	if planJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"id":     planID,
			"name":   plan.Name,
			"status": "ready",
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Approved plan %d (%q) — status is now ready\n", planID, plan.Name)
	return nil
}
