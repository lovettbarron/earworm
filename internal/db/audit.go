package db

import (
	"database/sql"
	"fmt"
	"time"
)

// AuditEntry represents an immutable record of a plan mutation.
type AuditEntry struct {
	ID           int64
	EntityType   string // e.g., "plan", "operation", "library_item"
	EntityID     string // plan ID as string, operation ID as string, or path
	Action       string // e.g., "create", "status_change", "update", "delete"
	BeforeState  string // JSON string
	AfterState   string // JSON string
	Success      bool
	ErrorMessage string
	CreatedAt    time.Time
}

// LogAudit inserts an audit log entry.
func LogAudit(db *sql.DB, entry AuditEntry) error {
	success := 0
	if entry.Success {
		success = 1
	}

	_, err := db.Exec(
		`INSERT INTO audit_log (entity_type, entity_id, action, before_state, after_state, success, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.EntityType, entry.EntityID, entry.Action,
		entry.BeforeState, entry.AfterState, success, entry.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("log audit: %w", err)
	}
	return nil
}

// LogAuditTx inserts an audit log entry within an existing transaction.
// Used by UpdatePlanStatusAudited to write audit + status change atomically.
func LogAuditTx(tx *sql.Tx, entry AuditEntry) error {
	success := 0
	if entry.Success {
		success = 1
	}

	_, err := tx.Exec(
		`INSERT INTO audit_log (entity_type, entity_id, action, before_state, after_state, success, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.EntityType, entry.EntityID, entry.Action,
		entry.BeforeState, entry.AfterState, success, entry.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("log audit tx: %w", err)
	}
	return nil
}

// ListAuditEntries retrieves audit entries for a given entity, ordered newest first.
// Returns an empty slice (not nil) when no entries exist.
func ListAuditEntries(db *sql.DB, entityType, entityID string) ([]AuditEntry, error) {
	rows, err := db.Query(
		`SELECT id, entity_type, entity_id, action, before_state, after_state, success, error_message, created_at
		FROM audit_log
		WHERE entity_type = ? AND entity_id = ?
		ORDER BY id DESC`,
		entityType, entityID,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var success int
		err := rows.Scan(
			&e.ID, &e.EntityType, &e.EntityID, &e.Action,
			&e.BeforeState, &e.AfterState, &success, &e.ErrorMessage, &e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		e.Success = success != 0
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit entries: %w", err)
	}

	if entries == nil {
		entries = []AuditEntry{}
	}
	return entries, nil
}
