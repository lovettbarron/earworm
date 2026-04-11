package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitDetect_MultiBookDir(t *testing.T) {
	// Create a directory with files that have distinct filename prefixes
	// (will trigger filename-based grouping since we can't inject real metadata)
	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "multibook")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	// Create audio files with prefix patterns for grouping
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookA_01.m4a"), []byte("a1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookA_02.m4a"), []byte("a2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookB_01.m4a"), []byte("b1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookB_02.m4a"), []byte("b2"), 0644))

	out, err := executeCommand(t, "split", "detect", bookDir)
	assert.NoError(t, err)
	// Should show groupings in table format
	assert.Contains(t, out, "BookA")
	assert.Contains(t, out, "BookB")
}

func TestSplitDetect_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "multibook")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookA_01.m4a"), []byte("a1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookA_02.m4a"), []byte("a2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookB_01.m4a"), []byte("b1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookB_02.m4a"), []byte("b2"), 0644))

	out, err := executeCommand(t, "split", "detect", bookDir, "--json")
	assert.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Contains(t, result, "groups")
}

func TestSplitDetect_SkippedDir(t *testing.T) {
	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "singlebook")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	// Single file -- can't split
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "track01.m4a"), []byte("audio"), 0644))

	out, err := executeCommand(t, "split", "detect", bookDir)
	// Should NOT error -- just informational per D-03
	assert.NoError(t, err)
	assert.Contains(t, out, "unable to determine")
}

func TestSplitPlan_CreatesPlan(t *testing.T) {
	database := setupPlanTestDB(t)
	_ = database // Ensures DB is created at expected config path

	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "multibook")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookA_01.m4a"), []byte("a1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookA_02.m4a"), []byte("a2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookB_01.m4a"), []byte("b1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "BookB_02.m4a"), []byte("b2"), 0644))

	out, err := executeCommand(t, "split", "plan", bookDir)
	assert.NoError(t, err)
	assert.Contains(t, out, "Created plan")
	assert.Contains(t, out, "Review with")
}

func TestSplitPlan_SkippedDirReturnsError(t *testing.T) {
	_ = setupPlanTestDB(t)

	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "singlebook")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "track01.m4a"), []byte("audio"), 0644))

	_, err := executeCommand(t, "split", "plan", bookDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "folder skipped")
}
