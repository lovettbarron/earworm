package journal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/db"
)

func TestEntryIDIsDeterministicAndWellFormed(t *testing.T) {
	id := EntryID(DayKey("2026-09-20"))

	assert.Equal(t, id, EntryID(DayKey("2026-09-20")), "the same key must always yield the same id")
	assert.Len(t, id, 32, "Day One expects a 32-character id")
	assert.Equal(t, strings.ToUpper(id), id)
	for _, c := range id {
		assert.True(t, strings.ContainsRune("0123456789ABCDEF", c), "id must be hex, got %q", c)
	}

	assert.NotEqual(t, id, EntryID(DayKey("2026-09-21")), "different keys must differ")
}

func TestContentHashChangesWithBody(t *testing.T) {
	a := Entry{Body: "one"}
	b := Entry{Body: "two"}
	assert.NotEqual(t, a.ContentHash(), b.ContentHash())
	assert.Equal(t, a.ContentHash(), Entry{Body: "one"}.ContentHash())
}

func sessions() []db.ListeningSession {
	return []db.ListeningSession{
		{ID: "s1", LibraryItemID: "item-1", Day: "2026-09-19", Seconds: 3600,
			Title: "Example Chronicle", Author: "An Author"},
		{ID: "s2", LibraryItemID: "item-1", Day: "2026-09-20", Seconds: 2700,
			Title: "Example Chronicle", Author: "An Author"},
		{ID: "s3", LibraryItemID: "item-2", Day: "2026-09-20", Seconds: 900,
			Title: "Second Example Tale", Author: "Another Author"},
	}
}

func TestBuildDayEntriesGroupsByDay(t *testing.T) {
	entries, err := BuildDayEntries(sessions(), BuildOptions{})
	require.NoError(t, err)
	require.Len(t, entries, 2)

	assert.Equal(t, DayKey("2026-09-19"), entries[0].Key)
	assert.Equal(t, KindDay, entries[0].Kind)
	assert.Contains(t, entries[0].Body, "## Listening — 2026-09-19")
	assert.Contains(t, entries[0].Body, "**1h** across 1 book")

	assert.Contains(t, entries[1].Body, "## Listening — 2026-09-20")
	assert.Contains(t, entries[1].Body, "2 books")
	assert.Contains(t, entries[1].Body, "Example Chronicle")
	assert.Contains(t, entries[1].Body, "Second Example Tale")
}

func TestBuildDayEntriesOrdersBooksByTime(t *testing.T) {
	entries, err := BuildDayEntries(sessions(), BuildOptions{})
	require.NoError(t, err)

	body := entries[1].Body
	assert.Less(t, strings.Index(body, "Example Chronicle"), strings.Index(body, "Second Example Tale"),
		"the book with more listening should lead")
}

// A journal entry saying nothing happened is noise.
func TestBuildDayEntriesSkipsDaysWithNoListening(t *testing.T) {
	entries, err := BuildDayEntries([]db.ListeningSession{
		{ID: "s1", Day: "2026-09-19", Seconds: 0, Title: "Zero"},
		{ID: "s2", Day: "", Seconds: 3600, Title: "No day"},
	}, BuildOptions{})

	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestBuildDayEntriesRespectsDateRange(t *testing.T) {
	entries, err := BuildDayEntries(sessions(), BuildOptions{Since: "2026-09-20"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, DayKey("2026-09-20"), entries[0].Key)

	entries, err = BuildDayEntries(sessions(), BuildOptions{Until: "2026-09-19"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, DayKey("2026-09-19"), entries[0].Key)
}

func TestBuildDayEntriesIsDeterministic(t *testing.T) {
	first, err := BuildDayEntries(sessions(), BuildOptions{})
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		again, err := BuildDayEntries(sessions(), BuildOptions{})
		require.NoError(t, err)
		require.Equal(t, len(first), len(again))
		for j := range first {
			assert.Equal(t, first[j].Body, again[j].Body,
				"rendering must not vary between runs, or every sync would rewrite every entry")
		}
	}
}

func TestBuildDayEntriesHandlesMissingMetadata(t *testing.T) {
	entries, err := BuildDayEntries([]db.ListeningSession{
		{ID: "s1", LibraryItemID: "item-1", Day: "2026-09-20", Seconds: 600},
	}, BuildOptions{})

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Body, "Unknown title")
}

func books() []db.BookListening {
	return []db.BookListening{
		{
			Source: "audible", SourceKey: "A1", Title: "Example Chronicle",
			Author: "An Author", Narrator: "A Narrator", Series: "A Series",
			SeriesPosition: "2", Genres: "Fantasy,Epic", RuntimeSeconds: 36000,
			IsFinished: true, StatusChangedAt: "2026-05-05T12:00:00Z",
			PurchaseDate: "2026-01-01T00:00:00Z",
		},
		{
			Source: "audible", SourceKey: "A2", Title: "Bulk Marked Book",
			IsFinished: true, StatusIsBulk: true,
			StatusChangedAt: "2021-12-08T22:55:38Z",
		},
		{
			Source: "audible", SourceKey: "A3", Title: "Unfinished Book",
			IsFinished: false, StatusChangedAt: "2026-06-01T12:00:00Z",
		},
	}
}

func TestBuildFinishEntriesRendersGenuineFinishes(t *testing.T) {
	entries, err := BuildFinishEntries(books(), BuildOptions{})
	require.NoError(t, err)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, KindFinish, e.Kind)
	assert.Contains(t, e.Body, "## Finished — Example Chronicle")
	assert.Contains(t, e.Body, "by An Author, narrated by A Narrator")
	assert.Contains(t, e.Body, "A Series #2")
	assert.Contains(t, e.Body, "Fantasy, Epic")
	assert.Contains(t, e.Body, "10h")
	assert.Equal(t, 2026, e.Date.Year())
}

// The bulk-marking artifact is the reason this filter exists: writing those
// timestamps as finish dates would put a fiction in a personal record.
func TestBuildFinishEntriesExcludesBulkMarkedBooks(t *testing.T) {
	entries, err := BuildFinishEntries(books(), BuildOptions{})
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.Body, "Bulk Marked Book",
			"a mass-marking timestamp is not evidence a book was finished")
	}
}

func TestBuildFinishEntriesExcludesUnfinishedBooks(t *testing.T) {
	entries, err := BuildFinishEntries(books(), BuildOptions{})
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Body, "Unfinished Book")
	}
}

func TestBuildFinishEntriesSkipsUnparseableTimestamps(t *testing.T) {
	entries, err := BuildFinishEntries([]db.BookListening{
		{Source: "audible", SourceKey: "A1", Title: "Bad Date",
			IsFinished: true, StatusChangedAt: "not a date"},
	}, BuildOptions{})

	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "0m", formatDuration(0))
	assert.Equal(t, "30s", formatDuration(30))
	assert.Equal(t, "45m", formatDuration(2700))
	assert.Equal(t, "1h", formatDuration(3600))
	assert.Equal(t, "1h 30m", formatDuration(5400))
	assert.Equal(t, "10h", formatDuration(36000))
}

func TestPluralise(t *testing.T) {
	assert.Equal(t, "1 book", pluralise(1, "book"))
	assert.Equal(t, "2 books", pluralise(2, "book"))
	assert.Equal(t, "0 books", pluralise(0, "book"))
}

// --- Day One CLI -----------------------------------------------------------

// TestDayOneHelperProcess backs the subprocess fakes for this package.
func TestDayOneHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	joined := strings.Join(args, " ")

	switch os.Getenv("GO_DAYONE_SCENARIO") {
	case "ok":
		// Echo back the flags so tests can assert on the invocation.
		fmt.Printf(`{"ok":true,"invocation":%q}`, joined)
	case "queued":
		fmt.Print(`{"ok":true,"queued":true,"synced":false}`)
	case "not_ok":
		fmt.Print(`{"ok":false,"error":"journal not found"}`)
	case "exit_fail":
		fmt.Fprint(os.Stderr, "dayone: not authenticated")
		os.Exit(1)
	case "journals":
		fmt.Print(`{"ok":true,"count":2,"items":[
			{"id":"111","name":"Journal","encryption":"plaintext","state":"active"},
			{"id":"222","name":"Audiobooks","encryption":"plaintext","state":"active"}]}`)
	case "plain_text":
		fmt.Print("done")
	}
	os.Exit(0)
}

func fakeDayOneCommand(scenario string, calls *[][]string, stdin *[]string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if calls != nil {
			*calls = append(*calls, append([]string(nil), args...))
		}
		cs := append([]string{"-test.run=TestDayOneHelperProcess", "--"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_DAYONE_SCENARIO="+scenario)
		return cmd
	}
}

func TestDayOneWritePassesDeterministicID(t *testing.T) {
	var calls [][]string
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("ok", &calls, nil)))

	e := Entry{
		Key:  DayKey("2026-09-20"),
		ID:   EntryID(DayKey("2026-09-20")),
		Kind: KindDay,
		Date: time.Date(2026, 9, 20, 21, 0, 0, 0, time.UTC),
		Body: "## Listening\n",
	}
	require.NoError(t, d.Write(context.Background(), "journal-1", e))

	require.Len(t, calls, 1)
	args := strings.Join(calls[0], " ")
	assert.Contains(t, args, "entry write")
	assert.Contains(t, args, "--journal-id journal-1")
	assert.Contains(t, args, "--entry-id "+e.ID,
		"the deterministic id is what makes a rewrite an update rather than a duplicate")
	assert.Contains(t, args, "--body-stdin",
		"entry text goes on stdin so it never appears in the process list")
}

func TestDayOneWriteRequiresJournalID(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("ok", nil, nil)))
	err := d.Write(context.Background(), "", Entry{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal id is not configured")
}

func TestDayOneWriteReportsCLIFailure(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("exit_fail", nil, nil)))
	err := d.Write(context.Background(), "journal-1", Entry{ID: "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")
}

// The CLI reports some failures in its body while still exiting zero.
func TestDayOneWriteDetectsFailureWithZeroExit(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("not_ok", nil, nil)))
	err := d.Write(context.Background(), "journal-1", Entry{ID: "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal not found")
}

func TestDayOneWriteAcceptsQueuedResponse(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("queued", nil, nil)))
	assert.NoError(t, d.Write(context.Background(), "journal-1", Entry{ID: "X"}),
		"a queued write is a success; delivery is Flush's job")
}

func TestDayOneWriteToleratesNonJSONOutput(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("plain_text", nil, nil)))
	assert.NoError(t, d.Write(context.Background(), "journal-1", Entry{ID: "X"}))
}

func TestDayOneFlushRunsSync(t *testing.T) {
	var calls [][]string
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("ok", &calls, nil)))

	require.NoError(t, d.Flush(context.Background()))
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"sync"}, calls[0])
}

func TestDayOneFlushReportsFailure(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("exit_fail", nil, nil)))
	err := d.Flush(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync")
}

func TestDayOneListJournals(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("journals", nil, nil)))

	journals, err := d.ListJournals(context.Background())
	require.NoError(t, err)
	require.Len(t, journals, 2)
	assert.Equal(t, "Audiobooks", journals[1].Name)
	assert.Equal(t, "plaintext", journals[1].Encryption)
}

func TestDayOneListJournalsReportsFailure(t *testing.T) {
	d := NewDayOne("dayone", WithCmdFactory(fakeDayOneCommand("exit_fail", nil, nil)))
	_, err := d.ListJournals(context.Background())
	require.Error(t, err)
}

func TestNewDayOneDefaultsBinaryName(t *testing.T) {
	assert.Equal(t, "dayone", NewDayOne("").Path)
	assert.Equal(t, "/custom/dayone", NewDayOne("/custom/dayone").Path)
}

// recoverableBooks covers the three shapes that can yield a finish entry plus
// one that must not.
func recoverableBooks() []db.BookListening {
	return []db.BookListening{
		// Genuine finish event.
		{Source: "audible", SourceKey: "A1", Title: "Genuine Finish", IsFinished: true,
			StatusChangedAt: "2026-05-05T12:00:00Z", LastPositionAt: "2026-05-05T12:30:00Z"},
		// Finished, but the timestamp was lost to a bulk marking.
		{Source: "audible", SourceKey: "A2", Title: "Bulk Stamped", IsFinished: true,
			StatusIsBulk: true, StatusChangedAt: "2021-12-08T22:55:38Z",
			LastPositionAt: "2018-03-14T09:00:00Z"},
		// Listened to the end, never marked finished.
		{Source: "audible", SourceKey: "A3", Title: "Never Marked", IsFinished: false,
			PercentComplete: 98, LastPositionAt: "2016-07-02T18:00:00Z"},
		// Abandoned partway: must never produce an entry.
		{Source: "audible", SourceKey: "A4", Title: "Abandoned", IsFinished: false,
			PercentComplete: 40, LastPositionAt: "2019-01-01T10:00:00Z"},
	}
}

func TestBuildFinishEntriesExcludesEstimatesByDefault(t *testing.T) {
	entries, err := BuildFinishEntries(recoverableBooks(), BuildOptions{})
	require.NoError(t, err)

	require.Len(t, entries, 1, "only the genuine finish event without opting in")
	assert.Contains(t, entries[0].Body, "Genuine Finish")
}

// The bulk marking destroyed the finish timestamp, but the last playback
// position keeps its own date and is the best remaining evidence.
func TestBuildFinishEntriesRecoversBulkStampedBooks(t *testing.T) {
	entries, err := BuildFinishEntries(recoverableBooks(), BuildOptions{EstimatedFinishes: true})
	require.NoError(t, err)

	require.Len(t, entries, 3, "genuine, bulk-recovered and never-marked; not the abandoned one")

	var bulk *Entry
	for i := range entries {
		if strings.Contains(entries[i].Body, "Bulk Stamped") {
			bulk = &entries[i]
		}
	}
	require.NotNil(t, bulk)
	assert.Equal(t, 2018, bulk.Date.Year(),
		"the date must come from the playback position, not the bulk timestamp")
	assert.NotEqual(t, 2021, bulk.Date.Year())
}

func TestBuildFinishEntriesRecoversNearCompleteBooks(t *testing.T) {
	entries, err := BuildFinishEntries(recoverableBooks(), BuildOptions{EstimatedFinishes: true})
	require.NoError(t, err)

	var found bool
	for _, e := range entries {
		if strings.Contains(e.Body, "Never Marked") {
			found = true
			assert.Equal(t, 2016, e.Date.Year())
		}
		assert.NotContains(t, e.Body, "Abandoned",
			"a book abandoned partway is not a finish at any threshold")
	}
	assert.True(t, found)
}

func TestBuildFinishEntriesRespectsNearCompleteThreshold(t *testing.T) {
	justUnder := []db.BookListening{{
		Source: "audible", SourceKey: "A1", Title: "Just Under",
		PercentComplete: NearCompleteThreshold - 0.1, LastPositionAt: "2020-01-01T00:00:00Z",
	}}
	entries, err := BuildFinishEntries(justUnder, BuildOptions{EstimatedFinishes: true})
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// An estimated date must be visibly estimated, or a reader years later cannot
// tell it apart from a recorded one.
func TestEstimatedEntriesAreLabelledAsSuch(t *testing.T) {
	entries, err := BuildFinishEntries(recoverableBooks(), BuildOptions{EstimatedFinishes: true})
	require.NoError(t, err)

	for _, e := range entries {
		if strings.Contains(e.Body, "Genuine Finish") {
			assert.Contains(t, e.Body, "## Finished — ")
			assert.Contains(t, e.Body, "from an Audible finish event")
			assert.NotContains(t, e.Body, "estimated")
			continue
		}
		assert.Contains(t, e.Body, "## Finished (estimated) — ")
		assert.Contains(t, e.Body, "Date estimated by earworm from the last playback position")
		assert.Contains(t, e.Body, "no finish event")
	}
}

func TestBuildFinishEntriesOrdersByResolvedDate(t *testing.T) {
	entries, err := BuildFinishEntries(recoverableBooks(), BuildOptions{EstimatedFinishes: true})
	require.NoError(t, err)

	for i := 1; i < len(entries); i++ {
		assert.False(t, entries[i].Date.Before(entries[i-1].Date),
			"entries must be ordered by the date actually used, not by one stored column")
	}
}

func TestBuildFinishEntriesRangeAppliesToEstimatedDates(t *testing.T) {
	entries, err := BuildFinishEntries(recoverableBooks(),
		BuildOptions{EstimatedFinishes: true, Since: "2017-01-01"})
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.Body, "Never Marked",
			"the 2016 estimate falls outside the requested range")
	}
}

// A genuine event must win even when a playback date is also present.
func TestGenuineFinishTakesPrecedenceOverEstimate(t *testing.T) {
	entries, err := BuildFinishEntries([]db.BookListening{{
		Source: "audible", SourceKey: "A1", Title: "Both Signals", IsFinished: true,
		StatusChangedAt: "2026-05-05T12:00:00Z", LastPositionAt: "2015-01-01T00:00:00Z",
	}}, BuildOptions{EstimatedFinishes: true})

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 2026, entries[0].Date.Year())
	assert.NotContains(t, entries[0].Body, "estimated")
}

// An Audible-worded footer on an Audiobookshelf book would misdescribe where
// the evidence came from.
func TestEstimatedFooterNamesTheRightSource(t *testing.T) {
	entries, err := BuildFinishEntries([]db.BookListening{
		{Source: "audible", SourceKey: "A1", Title: "From Audible", IsFinished: true,
			StatusIsBulk: true, LastPositionAt: "2018-01-01T00:00:00Z"},
		{Source: "abs", SourceKey: "item-1", Title: "From Audiobookshelf",
			PercentComplete: 99, LastPositionAt: "2026-02-01T00:00:00Z"},
	}, BuildOptions{EstimatedFinishes: true})
	require.NoError(t, err)
	require.Len(t, entries, 2)

	for _, e := range entries {
		if strings.Contains(e.Body, "From Audible") {
			assert.Contains(t, e.Body, "Audible recorded no finish event")
			assert.NotContains(t, e.Body, "measured playback")
			continue
		}
		assert.Contains(t, e.Body, "measured playback")
		assert.NotContains(t, e.Body, "Audible",
			"an Audiobookshelf book must not claim Audible as its source")
	}
}
