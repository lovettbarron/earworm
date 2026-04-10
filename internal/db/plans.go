package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ValidPlanStatuses defines the allowed plan status values.
var ValidPlanStatuses = []string{"draft", "ready", "running", "completed", "failed", "cancelled"}

// ValidOpTypes defines the allowed operation type values.
var ValidOpTypes = []string{"move", "flatten", "split", "delete", "write_metadata"}

// ValidOpStatuses defines the allowed operation status values.
var ValidOpStatuses = []string{"pending", "running", "completed", "failed", "skipped"}

// isValidPlanStatus checks whether a plan status string is in the allowed set.
func isValidPlanStatus(status string) bool {
	for _, s := range ValidPlanStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// IsValidOpType checks whether an operation type string is in the allowed set.
func IsValidOpType(opType string) bool {
	for _, t := range ValidOpTypes {
		if t == opType {
			return true
		}
	}
	return false
}

// isValidOpStatus checks whether an operation status string is in the allowed set.
func isValidOpStatus(status string) bool {
	for _, s := range ValidOpStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// Plan represents a named container for a set of library operations.
type Plan struct {
	ID          int64
	Name        string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PlanOperation represents an individual action within a plan.
type PlanOperation struct {
	ID           int64
	PlanID       int64
	Seq          int
	OpType       string
	SourcePath   string
	DestPath     string
	Status       string
	ErrorMessage string
	CompletedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreatePlan inserts a new plan with status "draft" and logs an audit entry.
func CreatePlan(db *sql.DB, name, description string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO plans (name, description) VALUES (?, ?)`,
		name, description,
	)
	if err != nil {
		return 0, fmt.Errorf("create plan: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create plan last insert id: %w", err)
	}

	afterState, _ := json.Marshal(map[string]string{
		"name":        name,
		"description": description,
		"status":      "draft",
	})

	err = LogAudit(db, AuditEntry{
		EntityType: "plan",
		EntityID:   fmt.Sprintf("%d", id),
		Action:     "create",
		AfterState: string(afterState),
		Success:    true,
	})
	if err != nil {
		return id, fmt.Errorf("create plan audit: %w", err)
	}

	return id, nil
}

// GetPlan retrieves a plan by ID. Returns nil and no error if not found.
func GetPlan(db *sql.DB, id int64) (*Plan, error) {
	var plan Plan
	err := db.QueryRow(
		`SELECT id, name, description, status, created_at, updated_at FROM plans WHERE id = ?`,
		id,
	).Scan(&plan.ID, &plan.Name, &plan.Description, &plan.Status, &plan.CreatedAt, &plan.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get plan %d: %w", id, err)
	}
	return &plan, nil
}

// ListPlans returns plans ordered by created_at descending.
// If status is non-empty, only plans with that status are returned.
// Returns an empty slice (not nil) when no plans exist.
func ListPlans(db *sql.DB, status string) ([]Plan, error) {
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = db.Query(
			`SELECT id, name, description, status, created_at, updated_at FROM plans ORDER BY created_at DESC`,
		)
	} else {
		rows, err = db.Query(
			`SELECT id, name, description, status, created_at, updated_at FROM plans WHERE status = ? ORDER BY created_at DESC`,
			status,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan plan row: %w", err)
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plans: %w", err)
	}

	if plans == nil {
		plans = []Plan{}
	}
	return plans, nil
}

// UpdatePlanStatus updates a plan's status after validation.
func UpdatePlanStatus(db *sql.DB, id int64, status string) error {
	if !isValidPlanStatus(status) {
		return fmt.Errorf("invalid plan status %q", status)
	}

	result, err := db.Exec(
		`UPDATE plans SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("update plan status %d: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("plan %d not found", id)
	}
	return nil
}

// UpdatePlanStatusAudited updates a plan's status and logs an audit entry atomically.
func UpdatePlanStatusAudited(db *sql.DB, id int64, newStatus string) error {
	if !isValidPlanStatus(newStatus) {
		return fmt.Errorf("invalid plan status %q", newStatus)
	}

	// Capture before state
	plan, err := GetPlan(db, id)
	if err != nil {
		return fmt.Errorf("update plan status audited: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("plan %d not found", id)
	}

	beforeJSON, _ := json.Marshal(map[string]string{"status": plan.Status})
	afterJSON, _ := json.Marshal(map[string]string{"status": newStatus})

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	_, err = tx.Exec(
		`UPDATE plans SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newStatus, id,
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("update plan status in tx: %w", err)
	}

	err = LogAuditTx(tx, AuditEntry{
		EntityType:  "plan",
		EntityID:    fmt.Sprintf("%d", id),
		Action:      "status_change",
		BeforeState: string(beforeJSON),
		AfterState:  string(afterJSON),
		Success:     true,
	})
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("audit plan status change: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit plan status update: %w", err)
	}
	return nil
}

// AddOperation adds an operation to a plan after validating the op type and plan existence.
func AddOperation(db *sql.DB, op PlanOperation) (int64, error) {
	if !IsValidOpType(op.OpType) {
		return 0, fmt.Errorf("invalid op type %q", op.OpType)
	}

	// Verify plan exists (enforcing FK in Go per research pitfall 1)
	plan, err := GetPlan(db, op.PlanID)
	if err != nil {
		return 0, fmt.Errorf("add operation check plan: %w", err)
	}
	if plan == nil {
		return 0, fmt.Errorf("add operation: plan %d not found", op.PlanID)
	}

	result, err := db.Exec(
		`INSERT INTO plan_operations (plan_id, seq, op_type, source_path, dest_path) VALUES (?, ?, ?, ?, ?)`,
		op.PlanID, op.Seq, op.OpType, op.SourcePath, op.DestPath,
	)
	if err != nil {
		return 0, fmt.Errorf("add operation: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("add operation last insert id: %w", err)
	}
	return id, nil
}

// ListOperations returns all operations for a plan ordered by sequence ascending.
// Returns an empty slice (not nil) when no operations exist.
func ListOperations(db *sql.DB, planID int64) ([]PlanOperation, error) {
	rows, err := db.Query(
		`SELECT id, plan_id, seq, op_type, source_path, dest_path, status, error_message, completed_at, created_at, updated_at
		FROM plan_operations WHERE plan_id = ? ORDER BY seq ASC`,
		planID,
	)
	if err != nil {
		return nil, fmt.Errorf("list operations for plan %d: %w", planID, err)
	}
	defer rows.Close()

	var ops []PlanOperation
	for rows.Next() {
		var op PlanOperation
		var completedAt sql.NullTime
		err := rows.Scan(
			&op.ID, &op.PlanID, &op.Seq, &op.OpType, &op.SourcePath, &op.DestPath,
			&op.Status, &op.ErrorMessage, &completedAt, &op.CreatedAt, &op.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan operation row: %w", err)
		}
		if completedAt.Valid {
			op.CompletedAt = &completedAt.Time
		}
		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operations: %w", err)
	}

	if ops == nil {
		ops = []PlanOperation{}
	}
	return ops, nil
}

// UpdateOperationStatus updates an operation's status and error message.
// If status is "completed", completed_at is set to CURRENT_TIMESTAMP.
func UpdateOperationStatus(db *sql.DB, id int64, status, errorMsg string) error {
	if !isValidOpStatus(status) {
		return fmt.Errorf("invalid op status %q", status)
	}

	var result sql.Result
	var err error

	if status == "completed" {
		result, err = db.Exec(
			`UPDATE plan_operations SET status = ?, error_message = ?, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, errorMsg, id,
		)
	} else {
		result, err = db.Exec(
			`UPDATE plan_operations SET status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, errorMsg, id,
		)
	}
	if err != nil {
		return fmt.Errorf("update operation status %d: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("operation %d not found", id)
	}
	return nil
}
