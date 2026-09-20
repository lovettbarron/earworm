// Package bookidentity reconciles books observed in different sources into a
// single identity.
//
// ASIN is the strongest signal but is far from universal: a listened book may
// exist in only one source, or in neither's catalogue. Identity is therefore a
// surrogate key with ASIN as an optional attribute, and books that match
// nothing are first-class rather than dropped.
package bookidentity

import (
	"regexp"
	"sort"
	"strings"
)

// Match methods, recorded per mapping so downstream consumers can weigh them.
const (
	// MatchASIN means two records shared an ASIN. Effectively certain.
	MatchASIN = "asin"
	// MatchTitle means normalized title and author agreed.
	MatchTitle = "title"
	// MatchTitleOnly means normalized titles agreed but authors could not be
	// compared, because one side had none.
	MatchTitleOnly = "title_only"
	// MatchNone means the record stands alone. This is a valid outcome.
	MatchNone = "none"
)

var (
	// Editions and formats that carry no identifying information.
	editionNoise = regexp.MustCompile(`(?i)\s*[\(\[]\s*(un)?abridged\s*[\)\]]`)
	// Leading or trailing series numbering such as "(#17)" or "#17".
	seriesNumber = regexp.MustCompile(`(?i)(^|\s)[\(\[]?#\s*\d+[\)\]]?(\s|$)`)
	// Explicit book/volume numbering, e.g. "Book 22" or ", Volume 3".
	bookNumber = regexp.MustCompile(`(?i)[,:]?\s*\b(book|volume|vol|part)\s*\.?\s*\d+\b`)
	// Apostrophes are removed outright rather than treated as separators, so
	// "Example's" normalizes to "examples" and not "example s".
	apostrophes = regexp.MustCompile(`['\x60\x{2018}\x{2019}]`)
	// Anything else that is not a letter, digit or space.
	nonAlnum = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	// Runs of whitespace.
	spaces = regexp.MustCompile(`\s+`)
	// A leading article, which sources disagree about.
	leadingArticle = regexp.MustCompile(`^(the|a|an)\s+`)
)

// NormalizeTitle reduces a title to a comparable form.
//
// Real catalogues disagree in ways no string-distance metric handles on its
// own: the same book appears with a series-number prefix, an "(Unabridged)"
// suffix, a subtitle after a colon or dash, or a leading article. Normalizing
// these away is most of the matching problem; the similarity score below only
// resolves what is left.
func NormalizeTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))

	s = editionNoise.ReplaceAllString(s, " ")
	s = seriesNumber.ReplaceAllString(s, " ")
	s = bookNumber.ReplaceAllString(s, " ")

	// Drop a subtitle after a colon or a spaced dash. Sources frequently carry
	// one where the other does not.
	if i := strings.Index(s, ":"); i > 0 {
		s = s[:i]
	}
	if i := strings.Index(s, " - "); i > 0 {
		s = s[:i]
	}

	s = apostrophes.ReplaceAllString(s, "")
	s = nonAlnum.ReplaceAllString(s, " ")
	s = spaces.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = leadingArticle.ReplaceAllString(s, "")

	return strings.TrimSpace(s)
}

// NormalizeAuthor reduces an author credit to a comparable form.
//
// Multi-author credits are split and sorted so that "A. One, B. Two" and
// "B. Two / A. One" compare equal.
func NormalizeAuthor(author string) string {
	s := strings.ToLower(strings.TrimSpace(author))
	s = apostrophes.ReplaceAllString(s, "")
	s = nonAlnum.ReplaceAllString(s, " ")
	s = spaces.ReplaceAllString(s, " ")

	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	sort.Strings(fields)
	return strings.Join(fields, " ")
}

// tokenSet returns the distinct words of a normalized string.
func tokenSet(s string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, f := range strings.Fields(s) {
		out[f] = struct{}{}
	}
	return out
}

// Similarity scores two normalized strings from 0 to 1 using token overlap
// (the Sørensen–Dice coefficient over word sets).
//
// Token overlap rather than edit distance because the failures here are word
// reordering and extra words, not typos: an edit-distance metric scores
// "example saga third movement" against "third movement an example saga" far
// worse than it deserves.
func Similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}

	ta, tb := tokenSet(a), tokenSet(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}

	shared := 0
	for tok := range ta {
		if _, ok := tb[tok]; ok {
			shared++
		}
	}
	return 2 * float64(shared) / float64(len(ta)+len(tb))
}

// DefaultTitleThreshold is the similarity a title pair must reach to match.
//
// Set high deliberately. A false match silently merges two books' listening
// histories, which is worse and much harder to notice than leaving a book
// unmatched — and an unmatched book is still exported in full.
const DefaultTitleThreshold = 0.85

// DefaultAuthorThreshold is the similarity an author pair must reach when both
// sides have an author. It is looser than the title threshold because author
// credits vary more in form than titles do.
const DefaultAuthorThreshold = 0.6
