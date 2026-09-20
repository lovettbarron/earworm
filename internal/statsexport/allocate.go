// Package statsexport turns stored listening data into files for analysis.
//
// It draws a hard line between measured and inferred. Audiobookshelf sessions
// say which book was played on which day; Audible reports a day's total with no
// book attached. The gap is filled by an allocation that is a reconstruction,
// not a record, so it is computed here at export time and never written back to
// the database as though it were observed.
package statsexport

import (
	"sort"
	"time"
)

// Attribution describes how confident a row's book assignment is.
const (
	// AttributionExact means a session recorded this book on this day.
	AttributionExact = "exact"
	// AttributionInferredSingle means one candidate book plausibly accounts
	// for the day's listening.
	AttributionInferredSingle = "inferred-single"
	// AttributionInferredSplit means several candidates shared the day.
	AttributionInferredSplit = "inferred-split"
	// AttributionUnattributed means time was recorded with no book to assign.
	AttributionUnattributed = "unattributed"
)

// AllocCandidate is a book that could account for some unattributed listening.
type AllocCandidate struct {
	IdentityID string
	Title      string
	// EndDate is the last day there is evidence the book was being listened
	// to: a finish event or a last-playback-position timestamp.
	EndDate time.Time
	// EarliestDate bounds how far back allocation may reach, normally the
	// purchase or library-add date. Zero means unbounded.
	EarliestDate time.Time
	// Seconds is the estimated listening this book accounts for.
	Seconds int
}

// DayTotal is one day's measured listening from a source.
type DayTotal struct {
	Day     string
	Seconds int
}

// Allocation assigns part of a day's listening to a book.
type Allocation struct {
	Day         string
	IdentityID  string
	Title       string
	Seconds     int
	Attribution string
}

// maxWalkBackDays bounds how far allocation reaches for one book, so a single
// candidate with a bad end date cannot consume an unbounded stretch of history.
const maxWalkBackDays = 400

// Allocate distributes measured daily totals across candidate books.
//
// Each book is walked backwards from its end date, consuming unallocated time
// until its estimated listening is accounted for. Books are processed
// newest-first so that recent, better-evidenced books claim their days before
// older ones reach back into the same stretch.
//
// This is inference. Every result is labelled as such, and days that no
// candidate can explain are returned as unattributed rather than being forced
// onto the nearest book.
func Allocate(days []DayTotal, candidates []AllocCandidate) []Allocation {
	remaining := make(map[string]int, len(days))
	dayOrder := make([]string, 0, len(days))
	for _, d := range days {
		if d.Seconds <= 0 {
			continue
		}
		remaining[d.Day] = d.Seconds
		dayOrder = append(dayOrder, d.Day)
	}
	sort.Strings(dayOrder)

	ordered := make([]AllocCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Seconds > 0 && !c.EndDate.IsZero() {
			ordered = append(ordered, c)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].EndDate.Equal(ordered[j].EndDate) {
			return ordered[i].EndDate.After(ordered[j].EndDate)
		}
		// Deterministic tie-break so the same input always allocates the same
		// way; without it map iteration order would leak into the output.
		return ordered[i].IdentityID < ordered[j].IdentityID
	})

	// day -> identity -> seconds
	assigned := make(map[string]map[string]int)
	titles := make(map[string]string)

	for _, c := range ordered {
		titles[c.IdentityID] = c.Title
		need := c.Seconds
		cursor := c.EndDate

		for walked := 0; need > 0 && walked < maxWalkBackDays; walked++ {
			if !c.EarliestDate.IsZero() && cursor.Before(c.EarliestDate) {
				break
			}
			key := cursor.Format("2006-01-02")
			if avail, ok := remaining[key]; ok && avail > 0 {
				take := need
				if take > avail {
					take = avail
				}
				remaining[key] = avail - take
				need -= take

				if assigned[key] == nil {
					assigned[key] = make(map[string]int)
				}
				assigned[key][c.IdentityID] += take
			}
			cursor = cursor.AddDate(0, 0, -1)
		}
	}

	var out []Allocation
	for _, day := range dayOrder {
		books := assigned[day]

		if len(books) == 0 {
			out = append(out, Allocation{
				Day:         day,
				Seconds:     remaining[day],
				Attribution: AttributionUnattributed,
			})
			continue
		}

		attribution := AttributionInferredSingle
		if len(books) > 1 {
			attribution = AttributionInferredSplit
		}

		ids := make([]string, 0, len(books))
		for id := range books {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		for _, id := range ids {
			out = append(out, Allocation{
				Day:         day,
				IdentityID:  id,
				Title:       titles[id],
				Seconds:     books[id],
				Attribution: attribution,
			})
		}

		// Whatever the candidates could not explain stays visibly unassigned
		// rather than being absorbed into one of them.
		if leftover := remaining[day]; leftover > 0 {
			out = append(out, Allocation{
				Day:         day,
				Seconds:     leftover,
				Attribution: AttributionUnattributed,
			})
		}
	}

	return out
}

// AllocationStats summarises how much of a dataset could be attributed.
type AllocationStats struct {
	TotalSeconds        int
	ExactSeconds        int
	InferredSeconds     int
	UnattributedSeconds int
}

// AttributedShare returns the fraction of time assigned to some book.
func (s AllocationStats) AttributedShare() float64 {
	if s.TotalSeconds == 0 {
		return 0
	}
	return float64(s.ExactSeconds+s.InferredSeconds) / float64(s.TotalSeconds)
}

// Summarise totals allocations by attribution quality.
func Summarise(allocations []Allocation) AllocationStats {
	var s AllocationStats
	for _, a := range allocations {
		s.TotalSeconds += a.Seconds
		switch a.Attribution {
		case AttributionExact:
			s.ExactSeconds += a.Seconds
		case AttributionInferredSingle, AttributionInferredSplit:
			s.InferredSeconds += a.Seconds
		default:
			s.UnattributedSeconds += a.Seconds
		}
	}
	return s
}
