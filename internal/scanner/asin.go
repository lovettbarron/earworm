package scanner

import "regexp"

// asinPattern matches ASINs: B followed by exactly 9 uppercase alphanumeric characters.
// This covers both B0XXXXXXXX (modern) and older BXXXXXXXXX patterns.
var asinPattern = regexp.MustCompile(`B[0-9A-Z]{9}`)

// asinBracketPattern matches ASIN in square brackets: [B08C6YJ1LS]
var asinBracketPattern = regexp.MustCompile(`\[B[0-9A-Z]{9}\]`)

// asinParenPattern matches ASIN in parentheses: (B08C6YJ1LS)
var asinParenPattern = regexp.MustCompile(`\(B[0-9A-Z]{9}\)`)

// ExtractASIN extracts the first ASIN from a folder name.
// Returns the ASIN and true if found, or empty string and false if not.
func ExtractASIN(folderName string) (string, bool) {
	match := asinPattern.FindString(folderName)
	if match == "" {
		return "", false
	}
	return match, true
}
