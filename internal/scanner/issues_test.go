package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectEmptyDir(t *testing.T) {
	t.Run("empty directory returns issue", func(t *testing.T) {
		dir := t.TempDir()
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectEmptyDir(dir, entries)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueEmptyDir, issues[0].IssueType)
		assert.Equal(t, SeverityWarning, issues[0].Severity)
		assert.Equal(t, dir, issues[0].Path)
		assert.Contains(t, issues[0].Message, "empty")
	})

	t.Run("non-empty directory returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), nil, 0644))
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectEmptyDir(dir, entries)
		assert.Empty(t, issues)
	})
}

func TestDetectNoASIN(t *testing.T) {
	t.Run("audio files but no ASIN returns issue", func(t *testing.T) {
		dir := t.TempDir()
		bookDir := filepath.Join(dir, "Some Book Title")
		require.NoError(t, os.Mkdir(bookDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(bookDir)
		require.NoError(t, err)

		issues := detectNoASIN(bookDir, entries)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueNoASIN, issues[0].IssueType)
		assert.Equal(t, SeverityWarning, issues[0].Severity)
	})

	t.Run("audio files with ASIN in folder name returns nil", func(t *testing.T) {
		dir := t.TempDir()
		bookDir := filepath.Join(dir, "Some Book [B08C6YJ1LS]")
		require.NoError(t, os.Mkdir(bookDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(bookDir)
		require.NoError(t, err)

		issues := detectNoASIN(bookDir, entries)
		assert.Empty(t, issues)
	})

	t.Run("no audio files returns nil even without ASIN", func(t *testing.T) {
		dir := t.TempDir()
		bookDir := filepath.Join(dir, "Some Folder")
		require.NoError(t, os.Mkdir(bookDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "readme.txt"), nil, 0644))

		entries, err := os.ReadDir(bookDir)
		require.NoError(t, err)

		issues := detectNoASIN(bookDir, entries)
		assert.Empty(t, issues)
	})
}

func TestDetectNestedAudio(t *testing.T) {
	t.Run("subdirectory with audio files returns issue", func(t *testing.T) {
		dir := t.TempDir()
		subdir := filepath.Join(dir, "disc1")
		require.NoError(t, os.Mkdir(subdir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "track01.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectNestedAudio(dir, entries)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueNestedAudio, issues[0].IssueType)
		assert.Equal(t, SeverityWarning, issues[0].Severity)
		assert.Contains(t, issues[0].Message, "disc1")
	})

	t.Run("no subdirectories with audio returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectNestedAudio(dir, entries)
		assert.Empty(t, issues)
	})

	t.Run("subdirectory without audio returns nil", func(t *testing.T) {
		dir := t.TempDir()
		subdir := filepath.Join(dir, "extras")
		require.NoError(t, os.Mkdir(subdir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "notes.txt"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectNestedAudio(dir, entries)
		assert.Empty(t, issues)
	})
}

func TestDetectOrphanFiles(t *testing.T) {
	t.Run("unknown extension returns issue", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.pdf"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "setup.exe"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectOrphanFiles(dir, entries)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueOrphanFiles, issues[0].IssueType)
		assert.Equal(t, SeverityInfo, issues[0].Severity)
		assert.Contains(t, issues[0].Message, "readme.pdf")
		assert.Contains(t, issues[0].Message, "setup.exe")
	})

	t.Run("known extensions return nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cover.jpg"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "info.nfo"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectOrphanFiles(dir, entries)
		assert.Empty(t, issues)
	})

	t.Run("directories are skipped", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0755))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectOrphanFiles(dir, entries)
		assert.Empty(t, issues)
	})
}

func TestDetectCoverMissing(t *testing.T) {
	t.Run("audio but no cover image and no meta cover returns issue", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectCoverMissing(dir, entries, nil)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueCoverMissing, issues[0].IssueType)
		assert.Equal(t, SeverityInfo, issues[0].Severity)
	})

	t.Run("cover.jpg present returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter1.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cover.jpg"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectCoverMissing(dir, entries, nil)
		assert.Empty(t, issues)
	})

	t.Run("meta HasCover true returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		meta := &BookMetadata{HasCover: true}
		issues := detectCoverMissing(dir, entries, meta)
		assert.Empty(t, issues)
	})

	t.Run("no audio files returns nil even without cover", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectCoverMissing(dir, entries, nil)
		assert.Empty(t, issues)
	})
}

func TestDetectMissingMetadata(t *testing.T) {
	t.Run("nil metadata and no sidecar returns issue", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectMissingMetadata(dir, entries, nil)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueMissingMetadata, issues[0].IssueType)
		assert.Equal(t, SeverityInfo, issues[0].Severity)
	})

	t.Run("non-nil metadata returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		meta := &BookMetadata{Title: "Test Book"}
		issues := detectMissingMetadata(dir, entries, meta)
		assert.Empty(t, issues)
	})

	t.Run("metadata.json sidecar present returns nil even without meta", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "chapter1.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{}"), 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectMissingMetadata(dir, entries, nil)
		assert.Empty(t, issues)
	})

	t.Run("no audio files returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectMissingMetadata(dir, entries, nil)
		assert.Empty(t, issues)
	})
}

func TestDetectWrongStructure(t *testing.T) {
	t.Run("path 3+ levels deep returns issue", func(t *testing.T) {
		root := t.TempDir()
		deep := filepath.Join(root, "a", "b", "c")
		require.NoError(t, os.MkdirAll(deep, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(deep, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(deep)
		require.NoError(t, err)

		issues := detectWrongStructure(deep, entries, root)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueWrongStructure, issues[0].IssueType)
		assert.Equal(t, SeverityInfo, issues[0].Severity)
	})

	t.Run("path 2 levels deep returns nil", func(t *testing.T) {
		root := t.TempDir()
		bookDir := filepath.Join(root, "Author", "Title [B08C6YJ1LS]")
		require.NoError(t, os.MkdirAll(bookDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "chapter1.m4a"), nil, 0644))

		entries, err := os.ReadDir(bookDir)
		require.NoError(t, err)

		issues := detectWrongStructure(bookDir, entries, root)
		assert.Empty(t, issues)
	})

	t.Run("deep path without audio returns nil", func(t *testing.T) {
		root := t.TempDir()
		deep := filepath.Join(root, "a", "b", "c")
		require.NoError(t, os.MkdirAll(deep, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(deep, "notes.txt"), nil, 0644))

		entries, err := os.ReadDir(deep)
		require.NoError(t, err)

		issues := detectWrongStructure(deep, entries, root)
		assert.Empty(t, issues)
	})
}

func TestDetectMultiBook(t *testing.T) {
	t.Run("different titles return issue", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "The Hobbit - Chapter 1.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "The Hobbit - Chapter 2.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Lord of the Rings - Chapter 1.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectMultiBook(dir, entries)
		require.Len(t, issues, 1)
		assert.Equal(t, IssueMultiBook, issues[0].IssueType)
		assert.Equal(t, SeverityWarning, issues[0].Severity)
	})

	t.Run("same title multi-disc returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "01 - Chapter One.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "02 - Chapter Two.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "03 - Chapter Three.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectMultiBook(dir, entries)
		assert.Empty(t, issues)
	})

	t.Run("single audio file returns nil", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "book.m4a"), nil, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		issues := detectMultiBook(dir, entries)
		assert.Empty(t, issues)
	})
}

func TestDetectIssues_Aggregation(t *testing.T) {
	t.Run("directory with multiple problems returns multiple issues", func(t *testing.T) {
		root := t.TempDir()
		// Create a book dir with no ASIN, no cover, and an orphan file
		bookDir := filepath.Join(root, "Some Book Without ASIN")
		require.NoError(t, os.Mkdir(bookDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "chapter1.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "random.pdf"), nil, 0644))

		entries, err := os.ReadDir(bookDir)
		require.NoError(t, err)

		issues := DetectIssues(bookDir, entries, nil, root)

		// Should have: no_asin, orphan_files, cover_missing, missing_metadata
		typeSet := make(map[IssueType]bool)
		for _, issue := range issues {
			typeSet[issue.IssueType] = true
		}
		assert.True(t, typeSet[IssueNoASIN], "expected no_asin issue")
		assert.True(t, typeSet[IssueOrphanFiles], "expected orphan_files issue")
		assert.True(t, typeSet[IssueCoverMissing], "expected cover_missing issue")
		assert.True(t, typeSet[IssueMissingMetadata], "expected missing_metadata issue")
		assert.GreaterOrEqual(t, len(issues), 4)
	})

	t.Run("clean directory returns no issues", func(t *testing.T) {
		root := t.TempDir()
		bookDir := filepath.Join(root, "Author", "Great Book [B08C6YJ1LS]")
		require.NoError(t, os.MkdirAll(bookDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "chapter1.m4a"), nil, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(bookDir, "cover.jpg"), nil, 0644))

		entries, err := os.ReadDir(bookDir)
		require.NoError(t, err)

		meta := &BookMetadata{Title: "Great Book", HasCover: true}
		issues := DetectIssues(bookDir, entries, meta, root)
		assert.Empty(t, issues)
	})
}
