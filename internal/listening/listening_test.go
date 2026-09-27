package listening

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBucketerDefaultsToUTCNotLocal(t *testing.T) {
	b, err := NewBucketer("")
	require.NoError(t, err)
	assert.Equal(t, time.UTC, b.Location(),
		"an unset timezone must resolve to UTC; defaulting to local would make stored day keys depend on the host")
}

func TestNewBucketerRejectsUnknownZone(t *testing.T) {
	_, err := NewBucketer("Mars/Olympus_Mons")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load timezone")
}

// The same instant belongs to different days depending on the zone. This is
// the off-by-one-day bug the Bucketer exists to prevent, so it is asserted
// directly rather than left implicit.
func TestDayBucketDependsOnConfiguredZone(t *testing.T) {
	// 2026-01-15T23:30:00Z is still the 15th in UTC but already the 16th in
	// Berlin (UTC+1 in January).
	instant := time.Date(2026, 1, 15, 23, 30, 0, 0, time.UTC)

	utc, err := NewBucketer("UTC")
	require.NoError(t, err)
	berlin, err := NewBucketer("Europe/Berlin")
	require.NoError(t, err)

	assert.Equal(t, "2026-01-15", utc.Day(instant))
	assert.Equal(t, "2026-01-16", berlin.Day(instant))
}

func TestDayFromEpochMillisMatchesDay(t *testing.T) {
	b, err := NewBucketer("Europe/Berlin")
	require.NoError(t, err)

	instant := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, b.Day(instant), b.DayFromEpochMillis(instant.UnixMilli()))
}

// A source that reports epoch milliseconds must be bucketed through those,
// not through any date string it also supplies in its own zone.
func TestDayFromEpochMillisIgnoresSourceReportedDate(t *testing.T) {
	// A server in UTC+13 would label this instant 2026-03-02; in UTC it is
	// still 2026-03-01.
	instant := time.Date(2026, 3, 1, 12, 30, 0, 0, time.UTC)

	utc, err := NewBucketer("UTC")
	require.NoError(t, err)
	assert.Equal(t, "2026-03-01", utc.DayFromEpochMillis(instant.UnixMilli()))

	auckland, err := NewBucketer("Pacific/Auckland")
	require.NoError(t, err)
	assert.Equal(t, "2026-03-02", auckland.DayFromEpochMillis(instant.UnixMilli()))
}

func TestDayHandlesDSTTransition(t *testing.T) {
	b, err := NewBucketer("Europe/Berlin")
	require.NoError(t, err)

	// Berlin springs forward on 2026-03-29 at 02:00 local. An instant just
	// after the jump must still land on that calendar day.
	instant := time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC) // 03:30 local
	assert.Equal(t, "2026-03-29", b.Day(instant))
}

func TestParseDayRoundTrips(t *testing.T) {
	b, err := NewBucketer("Europe/Berlin")
	require.NoError(t, err)

	parsed, err := b.ParseDay("2026-01-15")
	require.NoError(t, err)
	assert.Equal(t, "2026-01-15", b.Day(parsed))
	assert.Equal(t, b.Location(), parsed.Location())
}

func TestParseDayRejectsGarbage(t *testing.T) {
	b, err := NewBucketer("UTC")
	require.NoError(t, err)
	_, err = b.ParseDay("15/01/2026")
	require.Error(t, err)
}

func TestFixedClockIsDeterministic(t *testing.T) {
	want := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	c := FixedClock{T: want}
	assert.Equal(t, want, c.Now())
	assert.Equal(t, c.Now(), c.Now())
}

func TestSystemClockAdvances(t *testing.T) {
	var c Clock = SystemClock{}
	assert.False(t, c.Now().IsZero())
}

func TestDetectBulkClustersFlagsMassMarking(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var events []StatusEvent
	// Eighty books stamped within a few milliseconds: a migration artifact.
	for i := 0; i < 80; i++ {
		events = append(events, StatusEvent{
			Key:        string(rune('a'+i%26)) + time.Duration(i).String(),
			OccurredAt: base.Add(time.Duration(i) * time.Millisecond),
			Finished:   true,
		})
	}
	// Two genuine finishes, days apart.
	events = append(events,
		StatusEvent{Key: "real-one", OccurredAt: base.AddDate(0, 0, 10), Finished: true},
		StatusEvent{Key: "real-two", OccurredAt: base.AddDate(0, 0, 20), Finished: true},
	)

	flagged := DetectBulkClusters(events, DefaultBulkClusterOptions)

	assert.Len(t, flagged, 80, "every member of the dense cluster should be flagged")
	assert.False(t, flagged["real-one"], "isolated finishes must survive")
	assert.False(t, flagged["real-two"])
}

func TestDetectBulkClustersLeavesGenuineSameDayFinishesAlone(t *testing.T) {
	base := time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)

	// Three books finished across one evening: real behaviour, hours apart.
	events := []StatusEvent{
		{Key: "a", OccurredAt: base},
		{Key: "b", OccurredAt: base.Add(3 * time.Hour)},
		{Key: "c", OccurredAt: base.Add(7 * time.Hour)},
	}

	flagged := DetectBulkClusters(events, DefaultBulkClusterOptions)
	assert.Empty(t, flagged, "same-day finishes spread over hours are not a bulk artifact")
}

func TestDetectBulkClustersRespectsMinSize(t *testing.T) {
	base := time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)
	events := []StatusEvent{
		{Key: "a", OccurredAt: base},
		{Key: "b", OccurredAt: base.Add(time.Millisecond)},
		{Key: "c", OccurredAt: base.Add(2 * time.Millisecond)},
	}

	// Three simultaneous events, threshold of five: not enough to call bulk.
	assert.Empty(t, DetectBulkClusters(events, BulkClusterOptions{Window: time.Second, MinSize: 5}))

	// Same data, threshold of three: now it qualifies.
	assert.Len(t, DetectBulkClusters(events, BulkClusterOptions{Window: time.Second, MinSize: 3}), 3)
}

func TestDetectBulkClustersIgnoresZeroTimestamps(t *testing.T) {
	events := []StatusEvent{
		{Key: "a"}, {Key: "b"}, {Key: "c"}, {Key: "d"}, {Key: "e"}, {Key: "f"},
	}
	flagged := DetectBulkClusters(events, DefaultBulkClusterOptions)
	assert.Empty(t, flagged, "missing timestamps are not evidence of bulk marking")
}

func TestDetectBulkClustersAppliesDefaultsForZeroOptions(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var events []StatusEvent
	for i := 0; i < 10; i++ {
		events = append(events, StatusEvent{
			Key:        time.Duration(i).String(),
			OccurredAt: base.Add(time.Duration(i) * time.Millisecond),
		})
	}
	assert.Len(t, DetectBulkClusters(events, BulkClusterOptions{}), 10)
}

func TestDetectBulkClustersEmptyInput(t *testing.T) {
	assert.Empty(t, DetectBulkClusters(nil, DefaultBulkClusterOptions))
}

func TestDetectBulkClustersHandlesTwoSeparateClusters(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var events []StatusEvent
	for i := 0; i < 6; i++ {
		events = append(events, StatusEvent{Key: "first" + time.Duration(i).String(), OccurredAt: base.Add(time.Duration(i) * time.Millisecond)})
	}
	for i := 0; i < 6; i++ {
		events = append(events, StatusEvent{Key: "second" + time.Duration(i).String(), OccurredAt: base.AddDate(0, 1, 0).Add(time.Duration(i) * time.Millisecond)})
	}
	// A lone genuine event between them.
	events = append(events, StatusEvent{Key: "lone", OccurredAt: base.AddDate(0, 0, 10)})

	flagged := DetectBulkClusters(events, DefaultBulkClusterOptions)
	assert.Len(t, flagged, 12)
	assert.False(t, flagged["lone"])
}

func TestDetectBulkClusterGroupsReturnsEachRun(t *testing.T) {
	base := time.Date(2026, 9, 22, 12, 17, 40, 0, time.UTC)
	var events []StatusEvent
	// Two dense runs, an hour apart, plus a lone event between them.
	for i := 0; i < 6; i++ {
		events = append(events, StatusEvent{Key: fmt.Sprintf("a%d", i), OccurredAt: base.Add(time.Duration(i*50) * time.Millisecond)})
	}
	events = append(events, StatusEvent{Key: "solo", OccurredAt: base.Add(30 * time.Minute)})
	for i := 0; i < 5; i++ {
		events = append(events, StatusEvent{Key: fmt.Sprintf("b%d", i), OccurredAt: base.Add(time.Hour + time.Duration(i*50)*time.Millisecond)})
	}
	// A run that is too small to count.
	events = append(events, StatusEvent{Key: "c0", OccurredAt: base.Add(2 * time.Hour)})
	events = append(events, StatusEvent{Key: "c1", OccurredAt: base.Add(2 * time.Hour).Add(time.Millisecond)})

	groups := DetectBulkClusterGroups(events, DefaultBulkClusterOptions)
	require.Len(t, groups, 2)
	assert.Len(t, groups[0], 6)
	assert.Len(t, groups[1], 5)

	// The flat form stays consistent with the grouped one.
	flat := DetectBulkClusters(events, DefaultBulkClusterOptions)
	assert.Len(t, flat, 11)
	assert.False(t, flat["solo"])
	assert.False(t, flat["c0"])
	assert.True(t, flat["a0"])
}

func TestDetectBulkClusterGroupsIgnoresUndatedEvents(t *testing.T) {
	events := make([]StatusEvent, 0, 6)
	for i := 0; i < 6; i++ {
		events = append(events, StatusEvent{Key: fmt.Sprintf("k%d", i)})
	}
	assert.Empty(t, DetectBulkClusterGroups(events, DefaultBulkClusterOptions))
}
