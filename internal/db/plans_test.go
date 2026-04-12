package db

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePlan(t *testing.T) {
	db := setupTestDB(t)

	id, err := CreatePlan(db, "my plan", "a description")
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	plan, err := GetPlan(db, id)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "my plan", plan.Name)
	assert.Equal(t, "a description", plan.Description)
	assert.Equal(t, "draft", plan.Status)
	assert.False(t, plan.CreatedAt.IsZero())
	assert.False(t, plan.UpdatedAt.IsZero())
}

func TestGetPlanNotFound(t *testing.T) {
	db := setupTestDB(t)

	plan, err := GetPlan(db, 999)
	assert.NoError(t, err)
	assert.Nil(t, plan)
}

func TestListPlans(t *testing.T) {
	db := setupTestDB(t)

	// Empty returns empty slice
	plans, err := ListPlans(db, "")
	require.NoError(t, err)
	assert.NotNil(t, plans)
	assert.Len(t, plans, 0)

	// Create two plans
	id1, err := CreatePlan(db, "plan1", "desc1")
	require.NoError(t, err)
	id2, err := CreatePlan(db, "plan2", "desc2")
	require.NoError(t, err)

	// Update one to "ready"
	err = UpdatePlanStatus(db, id2, "ready")
	require.NoError(t, err)

	// List all
	plans, err = ListPlans(db, "")
	require.NoError(t, err)
	assert.Len(t, plans, 2)

	// Filter by status "draft"
	plans, err = ListPlans(db, "draft")
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, id1, plans[0].ID)

	// Filter by status "ready"
	plans, err = ListPlans(db, "ready")
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, id2, plans[0].ID)
}

func TestUpdatePlanStatus(t *testing.T) {
	db := setupTestDB(t)

	id, err := CreatePlan(db, "test", "desc")
	require.NoError(t, err)

	err = UpdatePlanStatus(db, id, "ready")
	require.NoError(t, err)

	plan, err := GetPlan(db, id)
	require.NoError(t, err)
	assert.Equal(t, "ready", plan.Status)
}

func TestUpdatePlanStatusInvalid(t *testing.T) {
	db := setupTestDB(t)

	id, err := CreatePlan(db, "test", "desc")
	require.NoError(t, err)

	err = UpdatePlanStatus(db, id, "bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid plan status")
}

func TestAddOperation(t *testing.T) {
	db := setupTestDB(t)

	planID, err := CreatePlan(db, "test", "desc")
	require.NoError(t, err)

	opID, err := AddOperation(db, PlanOperation{
		PlanID:     planID,
		Seq:        1,
		OpType:     "move",
		SourcePath: "/src/book.m4a",
		DestPath:   "/dst/book.m4a",
	})
	require.NoError(t, err)
	assert.Greater(t, opID, int64(0))

	ops, err := ListOperations(db, planID)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	op := ops[0]
	assert.Equal(t, planID, op.PlanID)
	assert.Equal(t, 1, op.Seq)
	assert.Equal(t, "move", op.OpType)
	assert.Equal(t, "/src/book.m4a", op.SourcePath)
	assert.Equal(t, "/dst/book.m4a", op.DestPath)
	assert.Equal(t, "pending", op.Status)
}

func TestAddOperationInvalidType(t *testing.T) {
	db := setupTestDB(t)

	planID, err := CreatePlan(db, "test", "desc")
	require.NoError(t, err)

	_, err = AddOperation(db, PlanOperation{
		PlanID:     planID,
		Seq:        1,
		OpType:     "bogus",
		SourcePath: "/src",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid op type")
}

func TestListOperationsOrdering(t *testing.T) {
	db := setupTestDB(t)

	planID, err := CreatePlan(db, "test", "desc")
	require.NoError(t, err)

	// Add 3 ops out of seq order to verify ORDER BY seq ASC
	for _, seq := range []int{3, 1, 2} {
		_, err := AddOperation(db, PlanOperation{
			PlanID:     planID,
			Seq:        seq,
			OpType:     "move",
			SourcePath: fmt.Sprintf("/src/%d", seq),
			DestPath:   fmt.Sprintf("/dst/%d", seq),
		})
		require.NoError(t, err)
	}

	ops, err := ListOperations(db, planID)
	require.NoError(t, err)
	require.Len(t, ops, 3)
	assert.Equal(t, 1, ops[0].Seq)
	assert.Equal(t, 2, ops[1].Seq)
	assert.Equal(t, 3, ops[2].Seq)
}

func TestUpdateOperationStatus(t *testing.T) {
	db := setupTestDB(t)

	planID, err := CreatePlan(db, "test", "desc")
	require.NoError(t, err)

	opID, err := AddOperation(db, PlanOperation{
		PlanID:     planID,
		Seq:        1,
		OpType:     "move",
		SourcePath: "/src",
		DestPath:   "/dst",
	})
	require.NoError(t, err)

	err = UpdateOperationStatus(db, opID, "completed", "")
	require.NoError(t, err)

	ops, err := ListOperations(db, planID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "completed", ops[0].Status)
	assert.NotNil(t, ops[0].CompletedAt)
}

func TestUpdatePlanStatusAudited(t *testing.T) {
	db := setupTestDB(t)

	id, err := CreatePlan(db, "test", "desc")
	require.NoError(t, err)

	err = UpdatePlanStatusAudited(db, id, "ready")
	require.NoError(t, err)

	// Verify plan status changed
	plan, err := GetPlan(db, id)
	require.NoError(t, err)
	assert.Equal(t, "ready", plan.Status)

	// Verify audit entries: create + status_change
	entries, err := ListAuditEntries(db, "plan", fmt.Sprintf("%d", id))
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// Newest first (id DESC), so entries[0] is status_change
	assert.Equal(t, "status_change", entries[0].Action)
	assert.Contains(t, entries[0].BeforeState, "draft")
	assert.Contains(t, entries[0].AfterState, "ready")
	assert.True(t, entries[0].Success)
}

func TestAddOperationWithMetadata(t *testing.T) {
	db := setupTestDB(t)

	planID, err := CreatePlan(db, "meta-plan", "desc")
	require.NoError(t, err)

	opID, err := AddOperation(db, PlanOperation{
		PlanID:     planID,
		Seq:        1,
		OpType:     "write_metadata",
		SourcePath: "/lib/Book1",
		Metadata:   `{"title":"Great Book","author":"Jane"}`,
	})
	require.NoError(t, err)
	assert.Greater(t, opID, int64(0))

	ops, err := ListOperations(db, planID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, `{"title":"Great Book","author":"Jane"}`, ops[0].Metadata)
}

func TestAddOperationEmptyMetadata(t *testing.T) {
	db := setupTestDB(t)

	planID, err := CreatePlan(db, "no-meta", "desc")
	require.NoError(t, err)

	_, err = AddOperation(db, PlanOperation{
		PlanID:     planID,
		Seq:        1,
		OpType:     "move",
		SourcePath: "/src",
		DestPath:   "/dst",
	})
	require.NoError(t, err)

	ops, err := ListOperations(db, planID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "", ops[0].Metadata)
}

func TestListDeleteOperationsMetadata(t *testing.T) {
	db := setupTestDB(t)

	planID, err := CreatePlan(db, "del-meta", "desc")
	require.NoError(t, err)
	err = UpdatePlanStatus(db, planID, "ready")
	require.NoError(t, err)

	_, err = AddOperation(db, PlanOperation{
		PlanID:     planID,
		Seq:        1,
		OpType:     "delete",
		SourcePath: "/lib/old",
		Metadata:   `{"reason":"duplicate"}`,
	})
	require.NoError(t, err)

	ops, err := ListDeleteOperations(db, "ready", 0)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, `{"reason":"duplicate"}`, ops[0].Metadata)
}

func TestCreatePlanAudited(t *testing.T) {
	db := setupTestDB(t)

	id, err := CreatePlan(db, "audit-test", "test description")
	require.NoError(t, err)

	entries, err := ListAuditEntries(db, "plan", fmt.Sprintf("%d", id))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	assert.Equal(t, "create", entries[0].Action)
	assert.Contains(t, entries[0].AfterState, "audit-test")
	assert.True(t, entries[0].Success)
}
