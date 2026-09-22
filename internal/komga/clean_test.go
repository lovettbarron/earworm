package komga

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanTitleStripsReleaseSuffixes(t *testing.T) {
	tests := []struct{ in, want string }{
		// Shapes taken from a real library, with the group names changed.
		{"Solo Leveling [Webtoon] (2020-2023) (Digital) (GroupName)", "Solo Leveling"},
		{"Tokyo Ghoul - re (2017-2020) (Digital) (AnotherGroup)", "Tokyo Ghoul - re"},
		{"Bocchi the Rock! (Digital) (1r0n)", "Bocchi the Rock!"},
		{"Appleseed v02 - Prometheus Unbound (2008) (Digital) (XRA-Empire)", "Appleseed v02 - Prometheus Unbound"},
		{"Demon Slayer - Kimetsu no Yaiba (2018-2021) (Digital) (danke", "Demon Slayer - Kimetsu no Yaiba"},
		{"Gantz (2018-2023) (Digital) (1r0n)", "Gantz"},
		{"Some Title (2020)", "Some Title"},
		{"Some Title (Digital)", "Some Title"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, CleanTitle(tt.in), "input: %q", tt.in)
	}
}

// Every volume of a series repeats the same suffix, which is what made
// different volumes score as near-identical during matching.
func TestCleanTitleKeepsVolumesDistinct(t *testing.T) {
	a := CleanTitle("Example Saga 001 - Prologue (2020) (Digital) (GroupName)")
	b := CleanTitle("Example Saga 002 - Chapter Two (2020) (Digital) (GroupName)")
	assert.Equal(t, "Example Saga 001 - Prologue", a)
	assert.Equal(t, "Example Saga 002 - Chapter Two", b)
	assert.NotEqual(t, a, b)
}

func TestCleanTitleLeavesOrdinaryTitlesAlone(t *testing.T) {
	for _, s := range []string{
		"Example Chronicle",
		"A Title With (Parenthetical) Meaning",
		"Example Saga - Second Movement",
		"Nineteen Eighty-Four",
	} {
		assert.Equal(t, s, CleanTitle(s), "should be untouched")
	}
}

// A volume marker is part of the work, not the release.
func TestCleanTitleKeepsVolumeMarkers(t *testing.T) {
	assert.Equal(t, "Example Saga (v02)", CleanTitle("Example Saga (v02)"))
	assert.Equal(t, "Example Saga (Vol. 3)", CleanTitle("Example Saga (Vol. 3)"))
}

// A stripped title is worse than an ugly one.
func TestCleanTitleNeverEmpties(t *testing.T) {
	assert.Equal(t, "(2020) (Digital)", CleanTitle("(2020) (Digital)"))
	assert.Equal(t, "", CleanTitle(""))
	assert.Equal(t, "", CleanTitle("   "))
}
