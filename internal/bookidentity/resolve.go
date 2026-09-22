package bookidentity

import (
	"fmt"
	"sort"
	"strings"
)

// Record is one source's view of a book, as handed to the resolver.
type Record struct {
	Source    string
	SourceKey string
	ASIN      string
	Title     string
	Author    string
	// SeriesPosition is the volume or chapter number where the source has one.
	// It acts as a hard discriminator: see findTitleMatch.
	SeriesPosition string
	Seconds        int
}

// Mapping records how one source record was attached to an identity.
type Mapping struct {
	Source     string
	SourceKey  string
	Method     string
	Confidence float64
}

// Identity is one book, assembled from every source that observed it.
type Identity struct {
	ID       string
	ASIN     string
	Title    string
	Author   string
	Mappings []Mapping
}

// Sources returns the distinct source names contributing to this identity.
func (i Identity) Sources() []string {
	seen := make(map[string]bool)
	var out []string
	for _, m := range i.Mappings {
		if !seen[m.Source] {
			seen[m.Source] = true
			out = append(out, m.Source)
		}
	}
	sort.Strings(out)
	return out
}

// Method returns the strongest method by which this identity was assembled.
func (i Identity) Method() string {
	best := MatchNone
	for _, m := range i.Mappings {
		if methodRank[m.Method] > methodRank[best] {
			best = m.Method
		}
	}
	return best
}

var methodRank = map[string]int{
	MatchNone:      0,
	MatchTitleOnly: 1,
	MatchTitle:     2,
	MatchASIN:      3,
}

// Options tunes resolution.
type Options struct {
	TitleThreshold  float64
	AuthorThreshold float64
}

// DefaultOptions returns the standard thresholds.
func DefaultOptions() Options {
	return Options{
		TitleThreshold:  DefaultTitleThreshold,
		AuthorThreshold: DefaultAuthorThreshold,
	}
}

// bucket is an identity under construction, with its normalized match keys.
type bucket struct {
	identity   Identity
	normTitle  string
	normAuthor string
	position   string
}

// Resolve groups source records into identities.
//
// Records are matched first by ASIN, which is treated as authoritative, then by
// normalized title and author. A record matching nothing becomes its own
// identity: books present in only one source are a large and legitimate part of
// a listening history, not an error to be discarded.
//
// Records are processed in a deterministic order, so the same input always
// yields the same identities in the same order.
func Resolve(records []Record, opts Options) []Identity {
	if opts.TitleThreshold <= 0 {
		opts.TitleThreshold = DefaultTitleThreshold
	}
	if opts.AuthorThreshold <= 0 {
		opts.AuthorThreshold = DefaultAuthorThreshold
	}

	ordered := make([]Record, len(records))
	copy(ordered, records)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Source != ordered[j].Source {
			return ordered[i].Source < ordered[j].Source
		}
		return ordered[i].SourceKey < ordered[j].SourceKey
	})

	var buckets []*bucket
	byASIN := make(map[string]*bucket)

	for _, r := range ordered {
		nt := NormalizeTitle(r.Title)
		na := NormalizeAuthor(r.Author)

		// ASIN is authoritative when both sides carry one.
		if r.ASIN != "" {
			if b, ok := byASIN[r.ASIN]; ok {
				attach(b, r, MatchASIN, 1)
				continue
			}
		}

		if b, method, conf := findTitleMatch(buckets, nt, na, r.SeriesPosition, opts); b != nil {
			attach(b, r, method, conf)
			if r.ASIN != "" {
				byASIN[r.ASIN] = b
			}
			continue
		}

		nb := &bucket{
			position: r.SeriesPosition,
			identity: Identity{
				ID:     identityID(r),
				ASIN:   r.ASIN,
				Title:  r.Title,
				Author: r.Author,
				Mappings: []Mapping{{
					Source: r.Source, SourceKey: r.SourceKey,
					Method: MatchNone, Confidence: 1,
				}},
			},
			normTitle:  nt,
			normAuthor: na,
		}
		buckets = append(buckets, nb)
		if r.ASIN != "" {
			byASIN[r.ASIN] = nb
		}
	}

	out := make([]Identity, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, b.identity)
	}
	return out
}

// findTitleMatch returns the best-scoring bucket at or above the thresholds.
//
// The best match is taken rather than the first, so that adding an unrelated
// book earlier in the list cannot steal a better pairing.
func findTitleMatch(buckets []*bucket, nt, na, position string, opts Options) (*bucket, string, float64) {
	if nt == "" {
		return nil, "", 0
	}

	var best *bucket
	var bestMethod string
	var bestScore float64

	for _, b := range buckets {
		// Volume number is a hard discriminator. Serialized titles differ only
		// in their number while sharing every other token — a scanlation
		// suffix like "(2020) (Digital) (Group)" repeats across a whole run —
		// so similarity alone merges distinct volumes and sums their reading.
		if position != "" && b.position != "" && position != b.position {
			continue
		}

		score := Similarity(nt, b.normTitle)
		if score < opts.TitleThreshold {
			continue
		}

		method := MatchTitleOnly
		if na != "" && b.normAuthor != "" {
			authorScore := Similarity(na, b.normAuthor)
			if authorScore < opts.AuthorThreshold {
				// Same title, different author: almost certainly a different
				// book, or a different recording that should not be merged.
				continue
			}
			method = MatchTitle
			score = (score + authorScore) / 2
		}

		if score > bestScore {
			best, bestMethod, bestScore = b, method, score
		}
	}
	return best, bestMethod, bestScore
}

// attach records a mapping and fills in any fields the identity lacks.
func attach(b *bucket, r Record, method string, confidence float64) {
	b.identity.Mappings = append(b.identity.Mappings, Mapping{
		Source: r.Source, SourceKey: r.SourceKey,
		Method: method, Confidence: confidence,
	})
	if b.identity.ASIN == "" && r.ASIN != "" {
		b.identity.ASIN = r.ASIN
	}
	if b.identity.Title == "" {
		b.identity.Title = r.Title
	}
	if b.identity.Author == "" {
		b.identity.Author = r.Author
	}
	if b.normAuthor == "" {
		b.normAuthor = NormalizeAuthor(r.Author)
	}
	if b.position == "" {
		b.position = r.SeriesPosition
	}
}

// identityID builds a stable, human-readable identity key.
func identityID(r Record) string {
	if r.ASIN != "" {
		return "asin:" + r.ASIN
	}
	nt := NormalizeTitle(r.Title)
	if nt == "" {
		return fmt.Sprintf("%s:%s", r.Source, r.SourceKey)
	}
	na := NormalizeAuthor(r.Author)
	if na == "" {
		return "title:" + strings.ReplaceAll(nt, " ", "-")
	}
	return "title:" + strings.ReplaceAll(nt, " ", "-") + "|" + strings.ReplaceAll(na, " ", "-")
}
