// Package organize handles file organization for the audiobook library,
// constructing Libation-compatible paths and moving files across filesystems.
package organize

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// illegalChars matches filesystem-illegal characters: : / \ * ? " < > |
var illegalChars = regexp.MustCompile(`[:/\\*?"<>|]`)

// maxNameBytes is the maximum length of a filename component in bytes.
const maxNameBytes = 255

// truncateToBytes truncates s to at most maxBytes, cutting at a valid UTF-8
// rune boundary rather than splitting a multi-byte character.
func truncateToBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from maxBytes to find a valid rune start
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// SanitizeName strips illegal filesystem characters, trims whitespace, and
// truncates the result to 255 bytes at a valid UTF-8 rune boundary.
func SanitizeName(name string) string {
	name = illegalChars.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)
	return truncateToBytes(name, maxNameBytes)
}

// FirstAuthor extracts the first author from a potentially multi-author string.
// It splits on comma, semicolon, or " & " (checked in that order) and returns
// the first trimmed segment.
func FirstAuthor(authors string) string {
	authors = strings.TrimSpace(authors)
	if authors == "" {
		return ""
	}

	// Split by comma first, then semicolon, then " & "
	for _, sep := range []string{",", ";", " & "} {
		if strings.Contains(authors, sep) {
			parts := strings.SplitN(authors, sep, 2)
			return strings.TrimSpace(parts[0])
		}
	}
	return authors
}

// BuildBookPath constructs a Libation-compatible relative path in the form
// "Author/Title [ASIN]" from the given book metadata fields. It returns an
// error if author or title is empty before or after sanitization.
func BuildBookPath(author, title, asin string) (string, error) {
	if strings.TrimSpace(author) == "" {
		return "", fmt.Errorf("author is required")
	}
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("title is required")
	}

	sanitizedAuthor := SanitizeName(FirstAuthor(author))
	if sanitizedAuthor == "" {
		return "", fmt.Errorf("author is empty after sanitization")
	}

	titleWithASIN := fmt.Sprintf("%s [%s]", title, asin)
	sanitizedTitle := SanitizeName(titleWithASIN)
	if sanitizedTitle == "" {
		return "", fmt.Errorf("title is empty after sanitization")
	}

	return filepath.Join(sanitizedAuthor, sanitizedTitle), nil
}

// RenameM4AFile constructs an M4A filename from the book title. It sanitizes
// the title, falls back to "audiobook" if empty, and truncates to ensure the
// total filename (including .m4a extension) does not exceed 255 bytes.
func RenameM4AFile(title string) string {
	name := SanitizeName(title)
	if name == "" {
		name = "audiobook"
	}
	// Reserve 4 bytes for ".m4a"
	name = truncateToBytes(name, maxNameBytes-4)
	return name + ".m4a"
}
