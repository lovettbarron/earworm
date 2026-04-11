package planengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteOp_SplitAudioFile(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src := createTempFile(t, tmpDir, "audio.m4a", "audio content for split")
	dst := filepath.Join(tmpDir, "dest", "audio.m4a")

	planID := createReadyPlan(t, sqlDB, "split-audio-test", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: src, DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success, "split audio op should succeed")
	assert.NotEmpty(t, results[0].SHA256, "should have SHA-256 hash")
	assert.Empty(t, results[0].Error)

	// Audio files should be MOVED (source gone, dest exists)
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source audio should be removed (moved)")
	_, err = os.Stat(dst)
	assert.NoError(t, err, "dest audio should exist")
}

func TestExecuteOp_SplitSharedJPG(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src := createTempFile(t, tmpDir, "cover.jpg", "image data for split")
	dst := filepath.Join(tmpDir, "dest", "cover.jpg")

	planID := createReadyPlan(t, sqlDB, "split-shared-jpg", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: src, DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success, "split shared file op should succeed")
	assert.NotEmpty(t, results[0].SHA256, "should have SHA-256 hash")

	// Shared files should be COPIED (source still exists, dest also exists)
	_, err = os.Stat(src)
	assert.NoError(t, err, "source cover should still exist (copied, not moved)")
	_, err = os.Stat(dst)
	assert.NoError(t, err, "dest cover should exist")
}

func TestExecuteOp_SplitSharedJSON(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src := createTempFile(t, tmpDir, "metadata.json", `{"title":"Test"}`)
	dst := filepath.Join(tmpDir, "dest", "metadata.json")

	planID := createReadyPlan(t, sqlDB, "split-shared-json", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: src, DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success, "split JSON op should succeed")
	assert.NotEmpty(t, results[0].SHA256)

	// JSON files are shared -- should be COPIED (source remains)
	_, err = os.Stat(src)
	assert.NoError(t, err, "source JSON should still exist (copied)")
	_, err = os.Stat(dst)
	assert.NoError(t, err, "dest JSON should exist")
}

func TestSplitOp_MP3UsesVerifiedMove(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src := createTempFile(t, tmpDir, "audiobook.mp3", "mp3 audio content")
	dst := filepath.Join(tmpDir, "dest", "audiobook.mp3")

	planID := createReadyPlan(t, sqlDB, "split-mp3-test", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: src, DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success, "split mp3 op should succeed")
	assert.NotEmpty(t, results[0].SHA256)

	// MP3 should be MOVED (source gone, dest exists)
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source mp3 should be removed (moved via VerifiedMove)")
	_, err = os.Stat(dst)
	assert.NoError(t, err, "dest mp3 should exist")
	content, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "mp3 audio content", string(content))
}

func TestSplitOp_OGGUsesVerifiedMove(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src := createTempFile(t, tmpDir, "audiobook.ogg", "ogg audio content")
	dst := filepath.Join(tmpDir, "dest", "audiobook.ogg")

	planID := createReadyPlan(t, sqlDB, "split-ogg-test", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: src, DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success)
	// OGG should be MOVED
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source ogg should be removed (moved)")
	_, err = os.Stat(dst)
	assert.NoError(t, err, "dest ogg should exist")
}

func TestSplitOp_JPGUsesVerifiedCopy(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	src := createTempFile(t, tmpDir, "cover.jpg", "jpg image content")
	dst := filepath.Join(tmpDir, "dest", "cover.jpg")

	planID := createReadyPlan(t, sqlDB, "split-jpg-copy-test", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: src, DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	results, err := executor.Apply(context.Background(), planID)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Success)
	// JPG should be COPIED (source still exists)
	_, err = os.Stat(src)
	assert.NoError(t, err, "source jpg should still exist (copied, not moved)")
	_, err = os.Stat(dst)
	assert.NoError(t, err, "dest jpg should exist")
}

func TestExecuteOp_SplitFailure(t *testing.T) {
	sqlDB := setupTestDB(t)
	tmpDir := t.TempDir()

	dst := filepath.Join(tmpDir, "dest", "audio.m4a")

	planID := createReadyPlan(t, sqlDB, "split-fail-test", []db.PlanOperation{
		{Seq: 1, OpType: "split", SourcePath: "/nonexistent/audio.m4a", DestPath: dst},
	})

	executor := &Executor{DB: sqlDB}
	_, err := executor.Apply(context.Background(), planID)
	// Preflight catches missing source before execution begins
	require.Error(t, err, "split with missing source should fail at preflight")
	assert.Contains(t, err.Error(), "preflight check failed")
	assert.Contains(t, err.Error(), "missing source files")
}
