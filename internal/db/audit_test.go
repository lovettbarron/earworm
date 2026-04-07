package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogAudit(t *testing.T) {
	db := setupTestDB(t)

	entry := AuditEntry{
		EntityType: "plan",
		EntityID:   "1",
		Action:     "create",
		AfterState: `{"name":"test"}`,
		Success:    true,
	}

	err := LogAudit(db, entry)
	require.NoError(t, err)

	entries, err := ListAuditEntries(db, "plan", "1")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	got := entries[0]
	assert.Equal(t, "plan", got.EntityType)
	assert.Equal(t, "1", got.EntityID)
	assert.Equal(t, "create", got.Action)
	assert.Equal(t, `{"name":"test"}`, got.AfterState)
	assert.Equal(t, "", got.BeforeState)
	assert.True(t, got.Success)
	assert.Equal(t, "", got.ErrorMessage)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestLogAuditFailure(t *testing.T) {
	db := setupTestDB(t)

	entry := AuditEntry{
		EntityType:   "plan",
		EntityID:     "2",
		Action:       "status_change",
		BeforeState:  `{"status":"draft"}`,
		AfterState:   `{"status":"running"}`,
		Success:      false,
		ErrorMessage: "something broke",
	}

	err := LogAudit(db, entry)
	require.NoError(t, err)

	entries, err := ListAuditEntries(db, "plan", "2")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	got := entries[0]
	assert.False(t, got.Success)
	assert.Equal(t, "something broke", got.ErrorMessage)
}

func TestListAuditEntriesEmpty(t *testing.T) {
	db := setupTestDB(t)

	entries, err := ListAuditEntries(db, "plan", "999")
	require.NoError(t, err)
	assert.NotNil(t, entries)
	assert.Len(t, entries, 0)
}

func TestListAuditEntriesOrdering(t *testing.T) {
	db := setupTestDB(t)

	// Log 3 entries in order
	for i, action := range []string{"create", "status_change", "update"} {
		err := LogAudit(db, AuditEntry{
			EntityType: "plan",
			EntityID:   "10",
			Action:     action,
			AfterState: `{"seq":` + string(rune('1'+i)) + `}`,
			Success:    true,
		})
		require.NoError(t, err)
	}

	entries, err := ListAuditEntries(db, "plan", "10")
	require.NoError(t, err)
	require.Len(t, entries, 3)

	// Newest first (ORDER BY created_at DESC), but with in-memory SQLite
	// all inserts happen at the same second, so we rely on ROWID ordering.
	// The DESC order with same timestamps means highest ID first.
	assert.Equal(t, "update", entries[0].Action)
	assert.Equal(t, "status_change", entries[1].Action)
	assert.Equal(t, "create", entries[2].Action)
}
