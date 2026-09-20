package statsexport

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovettbarron/earworm/internal/bookidentity"
	"github.com/lovettbarron/earworm/internal/db"
	"github.com/lovettbarron/earworm/internal/listening"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAllocateWalksBackwardFromEndDate(t *testing.T) {
	days := []DayTotal{
		{Day: "2026-01-01", Seconds: 3600},
		{Day: "2026-01-02", Seconds: 3600},
		{Day: "2026-01-03", Seconds: 3600},
	}
	candidates := []AllocCandidate{
		{IdentityID: "book-a", Title: "Book A", EndDate: day("2026-01-03"), Seconds: 7200},
	}

	got := Allocate(days, candidates)

	// The book should consume Jan 3 and Jan 2, leaving Jan 1 unattributed.
	byDay := map[string][]Allocation{}
	for _, a := range got {
		byDay[a.Day] = append(byDay[a.Day], a)
	}

	require.Len(t, byDay["2026-01-03"], 1)
	assert.Equal(t, "book-a", byDay["2026-01-03"][0].IdentityID)
	assert.Equal(t, AttributionInferredSingle, byDay["2026-01-03"][0].Attribution)

	require.Len(t, byDay["2026-01-02"], 1)
	assert.Equal(t, "book-a", byDay["2026-01-02"][0].IdentityID)

	require.Len(t, byDay["2026-01-01"], 1)
	assert.Equal(t, AttributionUnattributed, byDay["2026-01-01"][0].Attribution,
		"time no candidate explains must stay visibly unassigned")
}

func TestAllocateSplitsDayAcrossCandidates(t *testing.T) {
	days := []DayTotal{{Day: "2026-01-03", Seconds: 7200}}
	candidates := []AllocCandidate{
		{IdentityID: "book-a", EndDate: day("2026-01-03"), Seconds: 3600},
		{IdentityID: "book-b", EndDate: day("2026-01-03"), Seconds: 3600},
	}

	got := Allocate(days, candidates)

	require.Len(t, got, 2)
	for _, a := range got {
		assert.Equal(t, AttributionInferredSplit, a.Attribution,
			"a shared day is weaker evidence and must say so")
		assert.Equal(t, 3600, a.Seconds)
	}
}

func TestAllocateRespectsEarliestDate(t *testing.T) {
	days := []DayTotal{
		{Day: "2026-01-01", Seconds: 3600},
		{Day: "2026-01-05", Seconds: 3600},
	}
	// The book was only acquired on the 5th, so it cannot explain the 1st.
	candidates := []AllocCandidate{{
		IdentityID:   "book-a",
		EndDate:      day("2026-01-05"),
		EarliestDate: day("2026-01-05"),
		Seconds:      7200,
	}}

	got := Allocate(days, candidates)

	for _, a := range got {
		if a.Day == "2026-01-01" {
			assert.Equal(t, AttributionUnattributed, a.Attribution,
				"allocation must not reach back before a book existed")
		}
	}
}

func TestAllocateLeavesSurplusUnattributed(t *testing.T) {
	days := []DayTotal{{Day: "2026-01-03", Seconds: 7200}}
	candidates := []AllocCandidate{
		{IdentityID: "book-a", EndDate: day("2026-01-03"), Seconds: 1800},
	}

	got := Allocate(days, candidates)

	var attributed, unattributed int
	for _, a := range got {
		if a.Attribution == AttributionUnattributed {
			unattributed += a.Seconds
		} else {
			attributed += a.Seconds
		}
	}
	assert.Equal(t, 1800, attributed)
	assert.Equal(t, 5400, unattributed,
		"time beyond what candidates explain must not be absorbed into a book")
}

func TestAllocateIsDeterministic(t *testing.T) {
	days := []DayTotal{{Day: "2026-01-03", Seconds: 7200}}
	candidates := []AllocCandidate{
		{IdentityID: "book-b", EndDate: day("2026-01-03"), Seconds: 3600},
		{IdentityID: "book-a", EndDate: day("2026-01-03"), Seconds: 3600},
	}

	first := Allocate(days, candidates)
	for i := 0; i < 10; i++ {
		again := Allocate(days, candidates)
		require.Equal(t, first, again, "identical input must allocate identically")
	}
}

func TestAllocateIgnoresCandidatesWithoutEvidence(t *testing.T) {
	days := []DayTotal{{Day: "2026-01-03", Seconds: 3600}}
	candidates := []AllocCandidate{
		{IdentityID: "no-end-date", Seconds: 3600},
		{IdentityID: "no-seconds", EndDate: day("2026-01-03")},
	}

	got := Allocate(days, candidates)
	require.Len(t, got, 1)
	assert.Equal(t, AttributionUnattributed, got[0].Attribution)
}

func TestAllocateEmptyInputs(t *testing.T) {
	assert.Empty(t, Allocate(nil, nil))
	assert.Empty(t, Allocate([]DayTotal{{Day: "2026-01-01", Seconds: 0}}, nil),
		"days with no listening produce no rows")
}

func TestSummarise(t *testing.T) {
	stats := Summarise([]Allocation{
		{Seconds: 3600, Attribution: AttributionExact},
		{Seconds: 1800, Attribution: AttributionInferredSingle},
		{Seconds: 900, Attribution: AttributionInferredSplit},
		{Seconds: 1800, Attribution: AttributionUnattributed},
	})

	assert.Equal(t, 8100, stats.TotalSeconds)
	assert.Equal(t, 3600, stats.ExactSeconds)
	assert.Equal(t, 2700, stats.InferredSeconds)
	assert.Equal(t, 1800, stats.UnattributedSeconds)
	assert.InDelta(t, 0.777, stats.AttributedShare(), 0.01)
}

func TestAttributedShareOnEmptyStats(t *testing.T) {
	assert.Zero(t, AllocationStats{}.AttributedShare())
}

// sampleDataset builds a small dataset covering both sources.
func sampleDataset(t *testing.T) Dataset {
	t.Helper()

	books := []db.BookListening{
		{
			Source: listening.SourceAudible, SourceKey: "A1", ASIN: "A1",
			Title: "Example Chronicle", Author: "An Author",
			RuntimeSeconds: 7200, PercentComplete: 100, IsFinished: true,
			StatusChangedAt: "2026-01-03T12:00:00Z", LastPositionAt: "2026-01-03T12:00:00Z",
			AddedDate: "2026-01-01T00:00:00Z", PurchaseDate: "2026-01-01T00:00:00Z",
			Genres: "Fantasy",
		},
		{
			Source: listening.SourceABS, SourceKey: "item-1", ASIN: "A1",
			Title: "Example Chronicle", Author: "An Author",
			RuntimeSeconds: 7200, SecondsListened: 1800, Genres: "Fantasy,Epic",
		},
		{
			Source: listening.SourceABS, SourceKey: "item-2",
			Title: "Only In Audiobookshelf", Author: "Another Author",
			RuntimeSeconds: 3600, SecondsListened: 900,
		},
	}

	identities := bookidentity.Resolve([]bookidentity.Record{
		{Source: listening.SourceAudible, SourceKey: "A1", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: listening.SourceABS, SourceKey: "item-1", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: listening.SourceABS, SourceKey: "item-2", Title: "Only In Audiobookshelf", Author: "Another Author"},
	}, bookidentity.DefaultOptions())

	return Dataset{
		Books: books,
		Days: []db.ListeningDay{
			{Day: "2026-01-02", Source: listening.SourceAudible, Seconds: 3600},
			{Day: "2026-01-03", Source: listening.SourceAudible, Seconds: 3600},
			{Day: "2026-02-01", Source: listening.SourceABS, Seconds: 2700},
		},
		Sessions: []db.ListeningSession{
			{ID: "s1", LibraryItemID: "item-1", Day: "2026-02-01", Seconds: 1800,
				Title: "Example Chronicle", Author: "An Author", DurationSeconds: 7200,
				StartedAt: "2026-02-01T10:00:00Z", UpdatedAt: "2026-02-01T11:00:00Z"},
			{ID: "s2", LibraryItemID: "item-2", Day: "2026-02-01", Seconds: 900,
				Title: "Only In Audiobookshelf", Author: "Another Author", DurationSeconds: 3600,
				StartedAt: "2026-02-01T12:00:00Z", UpdatedAt: "2026-02-01T12:30:00Z"},
		},
		Identities: identities,
	}
}

func readCSV(t *testing.T, path string) ([]string, [][]string) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, records)
	return records[0], records[1:]
}

func columnIndex(t *testing.T, header []string, name string) int {
	t.Helper()
	for i, h := range header {
		if h == name {
			return i
		}
	}
	t.Fatalf("column %q not found in %v", name, header)
	return -1
}

func TestExportWritesAllFiles(t *testing.T) {
	dir := t.TempDir()
	bucket, err := listening.NewBucketer("UTC")
	require.NoError(t, err)

	res, err := Export(sampleDataset(t), Options{Dir: dir, Timeline: true, Bucket: bucket})
	require.NoError(t, err)

	for _, name := range []string{BooksFile, DaysFile, SessionsFile, TimelineFile, ReadmeFile} {
		_, statErr := os.Stat(filepath.Join(dir, name))
		assert.NoError(t, statErr, "%s should be written", name)
	}
	assert.Contains(t, res.Files, TimelineFile)
}

func TestExportOmitsTimelineByDefault(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(dir, TimelineFile))
	assert.True(t, os.IsNotExist(statErr), "timeline is opt-in")
}

func TestExportMergesSourcesIntoOneBookRow(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	header, rows := readCSV(t, filepath.Join(dir, BooksFile))
	assert.Len(t, rows, 2, "the ASIN-matched pair merges; the single-source book stands alone")

	titleIdx := columnIndex(t, header, "title")
	sourcesIdx := columnIndex(t, header, "sources")
	secondsIdx := columnIndex(t, header, "seconds_listened")

	var merged, single []string
	for _, r := range rows {
		if strings.Contains(r[titleIdx], "Example Chronicle") {
			merged = r
		} else {
			single = r
		}
	}
	require.NotNil(t, merged)
	require.NotNil(t, single)

	assert.Equal(t, "abs+audible", merged[sourcesIdx])
	assert.Equal(t, "1800", merged[secondsIdx], "listening sums across sources")
	assert.Equal(t, "abs", single[sourcesIdx])
}

// A book in only one source is a normal part of a listening history and must
// not be dropped by the merge.
func TestExportRetainsSingleSourceBooks(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	header, rows := readCSV(t, filepath.Join(dir, BooksFile))
	titleIdx := columnIndex(t, header, "title")

	found := false
	for _, r := range rows {
		if r[titleIdx] == "Only In Audiobookshelf" {
			found = true
		}
	}
	assert.True(t, found, "an unmatched book must still appear in the export")
}

func TestExportLabelsEveryDayRowWithAttribution(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	header, rows := readCSV(t, filepath.Join(dir, DaysFile))
	attrIdx := columnIndex(t, header, "attribution")

	valid := map[string]bool{
		AttributionExact: true, AttributionInferredSingle: true,
		AttributionInferredSplit: true, AttributionUnattributed: true,
	}
	require.NotEmpty(t, rows)
	for _, r := range rows {
		assert.True(t, valid[r[attrIdx]], "unexpected attribution %q", r[attrIdx])
	}
}

func TestExportMarksSessionDaysExact(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	header, rows := readCSV(t, filepath.Join(dir, DaysFile))
	dayIdx := columnIndex(t, header, "day")
	attrIdx := columnIndex(t, header, "attribution")
	srcIdx := columnIndex(t, header, "source")

	var exact int
	for _, r := range rows {
		if r[dayIdx] == "2026-02-01" {
			assert.Equal(t, listening.SourceABS, r[srcIdx])
			assert.Equal(t, AttributionExact, r[attrIdx],
				"a day backed by session data is measured, not inferred")
			exact++
		}
	}
	assert.Equal(t, 2, exact, "one row per book listened that day")
}

func TestExportMarksAudibleDaysAsInferred(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	header, rows := readCSV(t, filepath.Join(dir, DaysFile))
	dayIdx := columnIndex(t, header, "day")
	attrIdx := columnIndex(t, header, "attribution")

	for _, r := range rows {
		if r[dayIdx] == "2026-01-02" || r[dayIdx] == "2026-01-03" {
			assert.NotEqual(t, AttributionExact, r[attrIdx],
				"Audible cannot say which book a day belongs to, so it is never exact")
		}
	}
}

func TestExportSessionsCarryIdentity(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	header, rows := readCSV(t, filepath.Join(dir, SessionsFile))
	require.Len(t, rows, 2)

	idIdx := columnIndex(t, header, "identity_id")
	for _, r := range rows {
		assert.NotEmpty(t, r[idIdx])
	}
}

func TestExportBookRowCarriesBulkFinishFlag(t *testing.T) {
	data := sampleDataset(t)
	data.Books[0].StatusIsBulk = true

	dir := t.TempDir()
	_, err := Export(data, Options{Dir: dir})
	require.NoError(t, err)

	header, rows := readCSV(t, filepath.Join(dir, BooksFile))
	bulkIdx := columnIndex(t, header, "finish_is_bulk")

	var sawTrue bool
	for _, r := range rows {
		if r[bulkIdx] == "true" {
			sawTrue = true
		}
	}
	assert.True(t, sawTrue, "a bulk-marked finish must be flagged in the export")
}

// The derived file must agree with the normalised files it is built from.
func TestTimelineAgreesWithDayRows(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir, Timeline: true})
	require.NoError(t, err)

	dayHeader, dayRows := readCSV(t, filepath.Join(dir, DaysFile))
	tlHeader, tlRows := readCSV(t, filepath.Join(dir, TimelineFile))

	assert.Len(t, tlRows, len(dayRows), "timeline is a denormalisation, not a different dataset")

	sum := func(header []string, rows [][]string) int {
		idx := columnIndex(t, header, "seconds")
		total := 0
		for _, r := range rows {
			var n int
			for _, c := range r[idx] {
				n = n*10 + int(c-'0')
			}
			total += n
		}
		return total
	}
	assert.Equal(t, sum(dayHeader, dayRows), sum(tlHeader, tlRows),
		"total seconds must match between the two views")
}

func TestExportRequiresDirectory(t *testing.T) {
	_, err := Export(sampleDataset(t), Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory is not set")
}

func TestExportCreatesMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	res, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)
	assert.Equal(t, dir, res.Dir)

	_, statErr := os.Stat(filepath.Join(dir, BooksFile))
	assert.NoError(t, statErr)
}

// The README is how a reader learns that some rows are reconstructions.
func TestReadmeExplainsAttribution(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(sampleDataset(t), Options{Dir: dir})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ReadmeFile))
	require.NoError(t, err)
	text := string(content)

	assert.Contains(t, text, AttributionExact)
	assert.Contains(t, text, AttributionInferredSingle)
	assert.Contains(t, text, AttributionUnattributed)
	assert.Contains(t, text, "Reconstructed, not observed")
	assert.Contains(t, text, "finish_is_bulk")
}

func TestExportEmptyDataset(t *testing.T) {
	dir := t.TempDir()
	res, err := Export(Dataset{}, Options{Dir: dir, Timeline: true})
	require.NoError(t, err)

	assert.Zero(t, res.Books)
	assert.Zero(t, res.Sessions)

	// Headers must still be written so the files are valid CSV.
	header, rows := readCSV(t, filepath.Join(dir, BooksFile))
	assert.Equal(t, bookHeader, header)
	assert.Empty(t, rows)
}

func TestParseAnyTimeHandlesStoredLayouts(t *testing.T) {
	assert.Equal(t, 2026, parseAnyTime("2026-01-06T17:20:37.562Z").Year())
	assert.Equal(t, 2026, parseAnyTime("2026-07-19 13:58:39.503").Year())
	assert.Equal(t, 2026, parseAnyTime("2026-05-19").Year())
	assert.True(t, parseAnyTime("").IsZero())
	assert.True(t, parseAnyTime("nonsense").IsZero())
}

func TestLatestTimePicksMostRecent(t *testing.T) {
	got := latestTime("2026-01-01T00:00:00Z", "2026-06-01T00:00:00Z", "")
	assert.Equal(t, time.June, got.Month())
	assert.True(t, latestTime("", "").IsZero())
}

func TestFirstNonEmptyHelper(t *testing.T) {
	assert.Equal(t, "b", firstNonEmpty("", "b"))
	assert.Empty(t, firstNonEmpty("", ""))
}
