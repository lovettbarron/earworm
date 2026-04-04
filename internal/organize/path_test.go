package organize

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"strips colon", "Hello: World", "Hello World"},
		{"strips all illegal chars", "A/B\\C*D?E\"F<G>H|I", "ABCDEFGHI"},
		{"trims whitespace", "  spaces  ", "spaces"},
		{"all chars illegal returns empty", "???", ""},
		{"no change for normal title", "Normal Title", "Normal Title"},
		{"truncates to 255 bytes", strings.Repeat("A", 300), strings.Repeat("A", 255)},
		{"truncates at rune boundary", strings.Repeat("a", 253) + "\U0001F600", strings.Repeat("a", 253)},
		// 4-byte emoji at position 253 would exceed 255 bytes, so it should be dropped
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeName(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), 255, "result must not exceed 255 bytes")
		})
	}
}

func TestFirstAuthor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single author", "Stephen King", "Stephen King"},
		{"comma separated", "Author One, Author Two", "Author One"},
		{"semicolon separated", "Author One; Author Two", "Author One"},
		{"ampersand separated", "Author One & Author Two", "Author One"},
		{"trimmed", "  Spaced Author , Other", "Spaced Author"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, FirstAuthor(tt.input))
		})
	}
}

func TestBuildBookPath(t *testing.T) {
	tests := []struct {
		name      string
		author    string
		title     string
		asin      string
		expected  string
		expectErr bool
	}{
		{
			name:     "basic path",
			author:   "Stephen King",
			title:    "The Shining",
			asin:     "B000000001",
			expected: "Stephen King/The Shining [B000000001]",
		},
		{
			name:     "multi-author uses first",
			author:   "Author One, Author Two",
			title:    "Title",
			asin:     "B123456789",
			expected: "Author One/Title [B123456789]",
		},
		{
			name:      "empty author returns error",
			author:    "",
			title:     "Title",
			asin:      "B123",
			expectErr: true,
		},
		{
			name:      "empty title returns error",
			author:    "Author",
			title:     "",
			asin:      "B123",
			expectErr: true,
		},
		{
			name:      "whitespace-only author returns error",
			author:    "  ",
			title:     "Title",
			asin:      "B123",
			expectErr: true,
		},
		{
			name:      "all-illegal author returns error",
			author:    "???",
			title:     "Title",
			asin:      "B123",
			expectErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildBookPath(tt.author, tt.title, tt.asin)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestRenameM4AFile(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{"normal title", "The Shining", "The Shining.m4a"},
		{"sanitizes colon", "Title: With Colon", "Title With Colon.m4a"},
		{"empty fallback", "", "audiobook.m4a"},
		{"long title truncated", strings.Repeat("A", 300), strings.Repeat("A", 251) + ".m4a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenameM4AFile(tt.title)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), 255)
		})
	}
}
