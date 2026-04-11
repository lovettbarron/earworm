package split

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/organize"
)

// CreateSplitPlan generates a plan with operations to split a multi-book folder
// into individual book directories using Libation-compatible naming.
// Shared files (covers, metadata) are included in every group's operations.
// Returns the plan ID or an error if the GroupResult is skipped.
func CreateSplitPlan(database *sql.DB, result *GroupResult, libraryRoot string) (int64, error) {
	if result.Skipped {
		return 0, fmt.Errorf("cannot create plan for skipped folder: %s", result.SkipReason)
	}

	planName := "split: " + filepath.Base(result.SourceDir)
	planID, err := db.CreatePlan(database, planName, "Split multi-book folder into individual book directories")
	if err != nil {
		return 0, fmt.Errorf("create split plan: %w", err)
	}

	seq := 1

	for _, group := range result.Groups {
		author := group.Author
		if author == "" {
			author = "Unknown Author"
		}
		title := group.Title
		if title == "" {
			title = "Unknown Title"
		}

		relPath, err := organize.BuildBookPath(author, title, group.ASIN)
		if err != nil {
			return 0, fmt.Errorf("build book path for %q/%q: %w", author, title, err)
		}
		fullDestDir := filepath.Join(libraryRoot, relPath)

		// Add operations for audio files
		for _, audioFile := range group.AudioFiles {
			_, err := db.AddOperation(database, db.PlanOperation{
				PlanID:     planID,
				Seq:        seq,
				OpType:     "split",
				SourcePath: audioFile,
				DestPath:   filepath.Join(fullDestDir, filepath.Base(audioFile)),
			})
			if err != nil {
				return 0, fmt.Errorf("add split operation: %w", err)
			}
			seq++
		}

		// Add operations for shared files (copies to each group)
		for _, sharedFile := range result.SharedFiles {
			_, err := db.AddOperation(database, db.PlanOperation{
				PlanID:     planID,
				Seq:        seq,
				OpType:     "split",
				SourcePath: sharedFile,
				DestPath:   filepath.Join(fullDestDir, filepath.Base(sharedFile)),
			})
			if err != nil {
				return 0, fmt.Errorf("add shared file operation: %w", err)
			}
			seq++
		}
	}

	return planID, nil
}
