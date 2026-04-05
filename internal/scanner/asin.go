package scanner

import "regexp"

// asinPattern matches ASINs: either B + 9 alphanumeric (B08C6YJ1LS) or ISBN-10 (1524779261 or 178999473X).
// ISBN-10 check digits can be X, so the last character allows [0-9X].
var asinPattern = regexp.MustCompile(`B[0-9A-Z]{9}|\d{9}[0-9X]`)

// asinBracketPattern matches ASIN in square brackets: [B08C6YJ1LS] or [1524779261] or [178999473X]
var asinBracketPattern = regexp.MustCompile(`\[(?:B[0-9A-Z]{9}|\d{9}[0-9X])\]`)

// asinParenPattern matches ASIN in parentheses: (B08C6YJ1LS) or (1524779261) or (178999473X)
var asinParenPattern = regexp.MustCompile(`\((?:B[0-9A-Z]{9}|\d{9}[0-9X])\)`)

// ExtractASIN extracts the first ASIN from a folder name.
// Looks for bracketed/parenthesized ASINs first (more precise), then standalone.
// Returns the ASIN and true if found, or empty string and false if not.
func ExtractASIN(folderName string) (string, bool) {
	// Try bracketed first — most precise
	if m := asinBracketPattern.FindString(folderName); m != "" {
		return m[1 : len(m)-1], true // strip [ ]
	}
	if m := asinParenPattern.FindString(folderName); m != "" {
		return m[1 : len(m)-1], true // strip ( )
	}
	// Fallback to standalone
	match := asinPattern.FindString(folderName)
	if match == "" {
		return "", false
	}
	return match, true
}
