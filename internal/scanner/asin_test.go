package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractASIN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantASIN string
		wantOK   bool
	}{
		{
			name:     "brackets",
			input:    "[B08C6YJ1LS]",
			wantASIN: "B08C6YJ1LS",
			wantOK:   true,
		},
		{
			name:     "parentheses",
			input:    "(B08C6YJ1LS)",
			wantASIN: "B08C6YJ1LS",
			wantOK:   true,
		},
		{
			name:     "standalone with title",
			input:    "Some Title B08C6YJ1LS",
			wantASIN: "B08C6YJ1LS",
			wantOK:   true,
		},
		{
			name:     "no ASIN",
			input:    "No ASIN here",
			wantASIN: "",
			wantOK:   false,
		},
		{
			name:     "too short after B",
			input:    "Short B0ABC",
			wantASIN: "",
			wantOK:   false,
		},
		{
			name:     "multiple ASINs returns first",
			input:    "Multiple [B08C6YJ1LS] and B09ABCDEF1",
			wantASIN: "B08C6YJ1LS",
			wantOK:   true,
		},
		{
			name:     "lowercase should not match",
			input:    "lowercase b08c6yj1ls",
			wantASIN: "",
			wantOK:   false,
		},
		{
			name:     "ASIN in folder name with author",
			input:    "Project Hail Mary [B08C6YJ1LS]",
			wantASIN: "B08C6YJ1LS",
			wantOK:   true,
		},
		{
			name:     "older ASIN pattern",
			input:    "Old Book [BXXXXXXXXX]",
			wantASIN: "BXXXXXXXXX",
			wantOK:   true,
		},
		{
			name:     "ASIN with trailing text",
			input:    "B08C6YJ1LS - Some Extra",
			wantASIN: "B08C6YJ1LS",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asin, ok := ExtractASIN(tt.input)
			assert.Equal(t, tt.wantASIN, asin)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}
