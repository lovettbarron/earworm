package split

import (
	"testing"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSplitPlan_Basic(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &GroupResult{
		SourceDir: "/library/Author/MultiBook",
		Groups: []BookGroup{
			{
				Title:      "Book One",
				Author:     "Author A",
				AudioFiles: []string{"/library/Author/MultiBook/b1_ch1.m4a", "/library/Author/MultiBook/b1_ch2.m4a"},
				Confidence: 1.0,
			},
			{
				Title:      "Book Two",
				Author:     "Author B",
				AudioFiles: []string{"/library/Author/MultiBook/b2_ch1.m4a"},
				Confidence: 1.0,
			},
		},
		SharedFiles: []string{"/library/Author/MultiBook/cover.jpg"},
		Skipped:     false,
	}

	planID, err := CreateSplitPlan(database, result, "/library")
	require.NoError(t, err)
	assert.Greater(t, planID, int64(0))

	// Check plan was created with correct name
	plan, err := db.GetPlan(database, planID)
	require.NoError(t, err)
	assert.Equal(t, "split: MultiBook", plan.Name)

	// Check operations
	ops, err := db.ListOperations(database, planID)
	require.NoError(t, err)

	// 2 audio files for group 1 + 1 shared + 1 audio file for group 2 + 1 shared = 5
	assert.Len(t, ops, 5)

	for _, op := range ops {
		assert.Equal(t, "split", op.OpType)
	}
}

func TestCreateSplitPlan_UsesLibationNaming(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &GroupResult{
		SourceDir: "/library/Old/MultiBook",
		Groups: []BookGroup{
			{
				Title:      "My Book",
				Author:     "John Doe",
				ASIN:       "B08C6YJ1LS",
				AudioFiles: []string{"/library/Old/MultiBook/ch1.m4a"},
				Confidence: 1.0,
			},
		},
		SharedFiles: nil,
		Skipped:     false,
	}

	planID, err := CreateSplitPlan(database, result, "/library")
	require.NoError(t, err)

	ops, err := db.ListOperations(database, planID)
	require.NoError(t, err)
	assert.Len(t, ops, 1)

	// Destination should use BuildBookPath format: "Author/Title [ASIN]"
	assert.Contains(t, ops[0].DestPath, "John Doe")
	assert.Contains(t, ops[0].DestPath, "My Book")
	assert.Contains(t, ops[0].DestPath, "B08C6YJ1LS")
}

func TestCreateSplitPlan_SkippedResult(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &GroupResult{
		SourceDir:  "/library/Author/MultiBook",
		Skipped:    true,
		SkipReason: "confidence below threshold",
	}

	_, err = CreateSplitPlan(database, result, "/library")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "skipped")
}

func TestCreateSplitPlan_SharedFilesInAllGroups(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	result := &GroupResult{
		SourceDir: "/library/Author/MultiBook",
		Groups: []BookGroup{
			{
				Title:      "Book A",
				Author:     "Auth",
				AudioFiles: []string{"/library/Author/MultiBook/a.m4a"},
				Confidence: 1.0,
			},
			{
				Title:      "Book B",
				Author:     "Auth",
				AudioFiles: []string{"/library/Author/MultiBook/b.m4a"},
				Confidence: 1.0,
			},
		},
		SharedFiles: []string{"/library/Author/MultiBook/cover.jpg", "/library/Author/MultiBook/metadata.json"},
		Skipped:     false,
	}

	planID, err := CreateSplitPlan(database, result, "/library")
	require.NoError(t, err)

	ops, err := db.ListOperations(database, planID)
	require.NoError(t, err)

	// 1 audio + 2 shared per group = 3 * 2 groups = 6 ops
	assert.Len(t, ops, 6)

	// Count shared file operations (cover.jpg and metadata.json should each appear twice)
	coverCount := 0
	metaCount := 0
	for _, op := range ops {
		if op.SourcePath == "/library/Author/MultiBook/cover.jpg" {
			coverCount++
		}
		if op.SourcePath == "/library/Author/MultiBook/metadata.json" {
			metaCount++
		}
	}
	assert.Equal(t, 2, coverCount, "cover.jpg should appear in both groups")
	assert.Equal(t, 2, metaCount, "metadata.json should appear in both groups")
}
