package bookidentity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTitleStripsCataloguingNoise(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"unabridged suffix", "Second Example Tale (Unabridged)", "second example tale"},
		{"abridged suffix", "Example Story [Abridged]", "example story"},
		{"leading series number", "(#17) Example Chronicle", "example chronicle"},
		{"bare series number", "#3 Example Chronicle", "example chronicle"},
		{"trailing series number", "Example Chronicle (#26)", "example chronicle"},
		{"book number", "Example Tale: Book 22", "example tale"},
		{"volume number", "Example Tale, Volume 3", "example tale"},
		{"subtitle after colon", "Example Saga: A Story of Things", "example saga"},
		{"subtitle after dash", "Example Saga - Third Movement", "example saga"},
		{"leading article", "The Example Chronicle", "example chronicle"},
		{"punctuation", "Example's Chronicle!", "examples chronicle"},
		{"case and spacing", "  EXAMPLE   Chronicle  ", "example chronicle"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeTitle(tt.in))
		})
	}
}

// Normalization is the whole point: these pairs must collapse to the same key.
func TestNormalizeTitleCollapsesRealWorldVariants(t *testing.T) {
	pairs := [][2]string{
		{"Second Example Tale (Unabridged)", "Second Example Tale"},
		{"(#17) Example Chronicle", "Example Chronicle"},
		{"Example Saga - Third Movement", "Example Saga"},
		{"The Example Chronicle", "Example Chronicle"},
		{"Example Tale: Book 22", "Example Tale"},
	}
	for _, p := range pairs {
		assert.Equal(t, NormalizeTitle(p[0]), NormalizeTitle(p[1]),
			"%q and %q should normalize alike", p[0], p[1])
	}
}

func TestNormalizeAuthorIsOrderInsensitive(t *testing.T) {
	assert.Equal(t, NormalizeAuthor("A. One, B. Two"), NormalizeAuthor("B. Two / A. One"))
	assert.Equal(t, "", NormalizeAuthor(""))
	assert.Equal(t, "one", NormalizeAuthor("One"))
}

func TestSimilarity(t *testing.T) {
	assert.Equal(t, 1.0, Similarity("example chronicle", "example chronicle"))
	assert.Equal(t, 0.0, Similarity("", "anything"))
	assert.Equal(t, 0.0, Similarity("anything", ""))

	// Word reordering should score highly, which is why token overlap is used
	// rather than edit distance.
	assert.Greater(t, Similarity("example saga third movement", "third movement example saga"), 0.99)

	// Unrelated titles should score low.
	assert.Less(t, Similarity("example chronicle", "entirely different book"), 0.3)
}

func TestResolveMatchesByASIN(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "audible", SourceKey: "A1", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: "abs", SourceKey: "item-1", ASIN: "A1", Title: "Totally Different Words", Author: "Someone Else"},
	}, DefaultOptions())

	require.Len(t, ids, 1, "a shared ASIN is authoritative even when titles disagree")
	assert.Equal(t, MatchASIN, ids[0].Method())
	assert.Equal(t, []string{"abs", "audible"}, ids[0].Sources())
}

func TestResolveMatchesByNormalizedTitleAndAuthor(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "audible", SourceKey: "A1", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: "abs", SourceKey: "item-1", Title: "The Example Chronicle (Unabridged)", Author: "An Author"},
	}, DefaultOptions())

	require.Len(t, ids, 1)
	assert.Equal(t, MatchTitle, ids[0].Method())
	assert.Equal(t, "A1", ids[0].ASIN, "the identity should inherit the known ASIN")
}

func TestResolveMatchesTitleOnlyWhenAuthorMissing(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "audible", SourceKey: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: "abs", SourceKey: "item-1", Title: "Example Chronicle"},
	}, DefaultOptions())

	require.Len(t, ids, 1)
	assert.Equal(t, MatchTitleOnly, ids[0].Method())
}

// A false merge destroys two books' histories and is hard to notice, so
// matching errs toward leaving things separate.
func TestResolveKeepsSameTitleDifferentAuthorApart(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "audible", SourceKey: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: "abs", SourceKey: "item-1", Title: "Example Chronicle", Author: "A Wholly Unrelated Person"},
	}, DefaultOptions())

	assert.Len(t, ids, 2, "same title with a different author must not merge")
}

// Books present in only one source are a large, legitimate part of a listening
// history and must survive resolution intact.
func TestResolveRetainsUnmatchedBooks(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "abs", SourceKey: "item-1", Title: "Only In One Place", Author: "An Author"},
		{Source: "abs", SourceKey: "item-2", Title: "Also Only Here", Author: "Another Author"},
	}, DefaultOptions())

	require.Len(t, ids, 2)
	for _, id := range ids {
		assert.Equal(t, MatchNone, id.Method())
		assert.Equal(t, []string{"abs"}, id.Sources())
	}
}

func TestResolveIsDeterministic(t *testing.T) {
	records := []Record{
		{Source: "abs", SourceKey: "item-2", Title: "Second Book", Author: "B Author"},
		{Source: "audible", SourceKey: "A1", ASIN: "A1", Title: "First Book", Author: "A Author"},
		{Source: "abs", SourceKey: "item-1", Title: "First Book", Author: "A Author"},
	}

	first := Resolve(records, DefaultOptions())
	for i := 0; i < 5; i++ {
		again := Resolve(records, DefaultOptions())
		require.Equal(t, len(first), len(again))
		for j := range first {
			assert.Equal(t, first[j].ID, again[j].ID, "identity ids must be stable across runs")
		}
	}
}

func TestResolveOrderIndependence(t *testing.T) {
	a := Record{Source: "audible", SourceKey: "A1", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"}
	b := Record{Source: "abs", SourceKey: "item-1", Title: "Example Chronicle (Unabridged)", Author: "An Author"}

	first := Resolve([]Record{a, b}, DefaultOptions())
	second := Resolve([]Record{b, a}, DefaultOptions())

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, first[0].ID, second[0].ID, "input order must not change the outcome")
}

func TestResolveEmptyInput(t *testing.T) {
	assert.Empty(t, Resolve(nil, DefaultOptions()))
}

func TestResolveHandlesUntitledRecords(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "abs", SourceKey: "item-1"},
		{Source: "abs", SourceKey: "item-2"},
	}, DefaultOptions())

	assert.Len(t, ids, 2, "records without titles must not all collapse together")
	assert.NotEqual(t, ids[0].ID, ids[1].ID)
}

func TestResolveAppliesDefaultThresholdsForZeroOptions(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "audible", SourceKey: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: "abs", SourceKey: "item-1", Title: "Example Chronicle", Author: "An Author"},
	}, Options{})
	assert.Len(t, ids, 1)
}

func TestIdentityMethodPrefersStrongestEvidence(t *testing.T) {
	id := Identity{Mappings: []Mapping{
		{Method: MatchNone}, {Method: MatchTitleOnly}, {Method: MatchASIN}, {Method: MatchTitle},
	}}
	assert.Equal(t, MatchASIN, id.Method())
}

func TestIdentityIDIsReadable(t *testing.T) {
	assert.Equal(t, "asin:SYNTH0001",
		identityID(Record{ASIN: "SYNTH0001", Title: "Anything"}))
	assert.Equal(t, "title:example-chronicle|an-author",
		identityID(Record{Title: "The Example Chronicle", Author: "An Author"}))
	assert.Equal(t, "title:example-chronicle",
		identityID(Record{Title: "Example Chronicle"}))
	assert.Equal(t, "abs:item-1",
		identityID(Record{Source: "abs", SourceKey: "item-1"}))
}

// Three records describing one book converge when each links to the growing
// identity, whether by ASIN or by title.
func TestResolveChainsMatchesAcrossSources(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "audible", SourceKey: "A1", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"},
		{Source: "abs", SourceKey: "item-1", Title: "The Example Chronicle (Unabridged)", Author: "An Author"},
		{Source: "abs", SourceKey: "item-2", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"},
	}, DefaultOptions())

	require.Len(t, ids, 1)
	assert.Len(t, ids[0].Mappings, 3)
	assert.Equal(t, MatchASIN, ids[0].Method())
}

// Known limitation, asserted so a future change to it is deliberate rather
// than accidental.
//
// Matching is single-pass: each record joins an existing identity or starts
// one, and identities are never merged with each other afterwards. So a
// record whose ASIN links it to one identity while its title would link it to
// another leaves the two separate. Reaching this requires a record carrying a
// correct ASIN alongside a title that resembles no other record of the same
// book, which does not occur in practice.
//
// The failure mode is a book split across two identities. That is much safer
// than the alternative: transitive merging would let one bad ASIN silently
// fuse unrelated books, and a split is visible in the export while a false
// merge is not.
func TestResolveDoesNotMergeIdentitiesTransitively(t *testing.T) {
	ids := Resolve([]Record{
		{Source: "abs", SourceKey: "item-1", Title: "Example Chronicle", Author: "An Author"},
		{Source: "abs", SourceKey: "item-2", ASIN: "A1", Title: "Nothing Like The Other Title", Author: "Nobody At All"},
		{Source: "audible", SourceKey: "A1", ASIN: "A1", Title: "Example Chronicle", Author: "An Author"},
	}, DefaultOptions())

	assert.Len(t, ids, 2,
		"the ASIN link wins for the audible record, leaving the title-matched one separate")
}
