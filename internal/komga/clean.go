package komga

import (
	"regexp"
	"strings"
)

// Scene suffixes carry no information about the work itself. A comic or manga
// filename routinely ends with the release year, the format, and the group who
// produced the scan — "Some Title (2018-2021) (Digital) (GroupName)" — none of
// which belongs in a journal entry or a reading list.
//
// They also actively damage matching: every volume of a series repeats the
// same suffix, so different volumes share most of their tokens and score as
// near-identical.
var (
	// A bracketed group: (…) or […].
	bracketGroup = regexp.MustCompile(`[\(\[]([^)\]]*)[\)\]]?`)
	// A year or a year range, the most reliable marker that the suffix has begun.
	yearLike = regexp.MustCompile(`^\d{4}(\s*-\s*\d{4})?$`)
	// Volume/chapter markers that should survive cleaning.
	volumeLike = regexp.MustCompile(`(?i)^(v|vol|volume|ch|chapter|book|part)\.?\s*\d+$`)
	spaces     = regexp.MustCompile(`\s+`)
)

// formatTags mark the start of the scene suffix. Anything from the first such
// marker to the end of the string is release metadata.
var formatTags = map[string]bool{
	"digital": true, "webtoon": true, "webrip": true, "scan": true,
	"scanlation": true, "c2c": true, "f": true, "hd": true, "web": true,
	"digital-empire": true, "danke": true, "kindle": true, "comixology": true,
	"repack": true, "fixed": true, "re-edition": true, "reedition": true,
	"covers": true, "extras": true, "omnibus edition": true,
}

// CleanTitle removes release-group and format suffixes from a title.
//
// It truncates at the FIRST bracketed group that marks the suffix — a year, a
// year range, or a known format tag — rather than stripping from the right.
// Group names are arbitrary ("LuCaZ", "1r0n", "XRA-Empire") and cannot be
// recognised on their own, but everything after the first marker is suffix, so
// the marker is what has to be found.
//
// A bracketed group that is neither is left alone, so a title whose own name
// contains parentheses survives. If cleaning would empty the string, the
// original is returned: a stripped title is worse than an ugly one.
func CleanTitle(title string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		return s
	}

	cut := -1
	for _, loc := range bracketGroup.FindAllStringSubmatchIndex(s, -1) {
		inner := strings.ToLower(strings.TrimSpace(s[loc[2]:loc[3]]))
		if inner == "" {
			continue
		}
		if volumeLike.MatchString(inner) {
			// "(v02)" is part of the work, not the release.
			continue
		}
		if yearLike.MatchString(inner) || formatTags[inner] {
			cut = loc[0]
			break
		}
	}

	if cut > 0 {
		s = s[:cut]
	}

	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, " -–—_,;:")
	s = spaces.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	if s == "" {
		return strings.TrimSpace(title)
	}
	return s
}
