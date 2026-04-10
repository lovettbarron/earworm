package planengine

import (
	"strings"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidOpType(t *testing.T) {
	assert.True(t, db.IsValidOpType("move"))
	assert.True(t, db.IsValidOpType("delete"))
	assert.True(t, db.IsValidOpType("flatten"))
	assert.True(t, db.IsValidOpType("split"))
	assert.True(t, db.IsValidOpType("write_metadata"))
	assert.False(t, db.IsValidOpType("rename"))
	assert.False(t, db.IsValidOpType(""))
}

func TestImportCSV_Valid(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path\nmove,/src/a.m4a,/dst/a.m4a\ndelete,/src/b.m4a,\nflatten,/src/dir,/dst/dir\n"

	result, err := ImportCSV(database, "test-plan", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, 3, result.RowCount)
	assert.Equal(t, 0, result.ErrorCount)
	assert.NotZero(t, result.PlanID)

	plan, err := db.GetPlan(database, result.PlanID)
	require.NoError(t, err)
	assert.Equal(t, "draft", plan.Status)
	assert.Equal(t, "test-plan", plan.Name)

	ops, err := db.ListOperations(database, result.PlanID)
	require.NoError(t, err)
	require.Len(t, ops, 3)
	assert.Equal(t, "move", ops[0].OpType)
	assert.Equal(t, "/src/a.m4a", ops[0].SourcePath)
	assert.Equal(t, "/dst/a.m4a", ops[0].DestPath)
	assert.Equal(t, 1, ops[0].Seq)
	assert.Equal(t, "delete", ops[1].OpType)
	assert.Equal(t, 2, ops[1].Seq)
	assert.Equal(t, "flatten", ops[2].OpType)
	assert.Equal(t, 3, ops[2].Seq)
}

func TestImportCSV_BOM(t *testing.T) {
	database := setupTestDB(t)
	// UTF-8 BOM prefix: 0xEF 0xBB 0xBF
	bom := "\xEF\xBB\xBF"
	csv := bom + "op_type,source_path,dest_path\nmove,/src/a.m4a,/dst/a.m4a\n"

	result, err := ImportCSV(database, "bom-plan", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowCount)
	assert.Equal(t, 0, result.ErrorCount)
	assert.NotZero(t, result.PlanID)
}

func TestImportCSV_CRLF(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path\r\nmove,/src/a.m4a,/dst/a.m4a\r\ndelete,/src/b.m4a,\r\n"

	result, err := ImportCSV(database, "crlf-plan", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, 2, result.RowCount)
	assert.Equal(t, 0, result.ErrorCount)
}

func TestImportCSV_MissingColumn(t *testing.T) {
	database := setupTestDB(t)
	csv := "source_path,dest_path\n/src/a.m4a,/dst/a.m4a\n"

	_, err := ImportCSV(database, "missing-col", strings.NewReader(csv))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required column")
}

func TestImportCSV_InvalidOpType(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path\nrename,/src/a.m4a,/dst/a.m4a\n"

	result, err := ImportCSV(database, "invalid-op", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.PlanID)
	assert.Equal(t, 1, result.ErrorCount)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, 2, result.Errors[0].Line)
	assert.Equal(t, "op_type", result.Errors[0].Column)
	assert.Contains(t, result.Errors[0].Message, "rename")
}

func TestImportCSV_EmptySourcePath(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path\nmove,,/dst/a.m4a\n"

	result, err := ImportCSV(database, "empty-src", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.PlanID)
	assert.Equal(t, 1, result.ErrorCount)
	assert.Equal(t, 2, result.Errors[0].Line)
	assert.Equal(t, "source_path", result.Errors[0].Column)
}

func TestImportCSV_MoveNoDest(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path\nmove,/src/a.m4a,\n"

	result, err := ImportCSV(database, "move-no-dest", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.PlanID)
	assert.Equal(t, 1, result.ErrorCount)
	assert.Equal(t, "dest_path", result.Errors[0].Column)
}

func TestImportCSV_DeleteNoDest(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path\ndelete,/src/a.m4a,\n"

	result, err := ImportCSV(database, "delete-no-dest", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowCount)
	assert.Equal(t, 0, result.ErrorCount)
	assert.NotZero(t, result.PlanID)
}

func TestImportCSV_ExtraColumns(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path,notes\nmove,/src/a.m4a,/dst/a.m4a,some note\n"

	result, err := ImportCSV(database, "extra-cols", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowCount)
	assert.Equal(t, 0, result.ErrorCount)
}

func TestImportCSV_CaseInsensitiveHeaders(t *testing.T) {
	database := setupTestDB(t)
	csv := "Op_Type,Source_Path,Dest_Path\nmove,/src/a.m4a,/dst/a.m4a\n"

	result, err := ImportCSV(database, "case-insensitive", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowCount)
	assert.Equal(t, 0, result.ErrorCount)
}

func TestImportCSV_EmptyFile(t *testing.T) {
	database := setupTestDB(t)
	csv := "op_type,source_path,dest_path\n"

	result, err := ImportCSV(database, "empty-plan", strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, 0, result.RowCount)
	assert.NotZero(t, result.PlanID)
}
