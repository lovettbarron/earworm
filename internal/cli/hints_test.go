package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHint_PrintsWhenNotQuiet(t *testing.T) {
	quiet = false
	var buf bytes.Buffer
	hint(&buf, "earworm download          # get %d books", 3)
	assert.Contains(t, buf.String(), "Next: earworm download")
	assert.Contains(t, buf.String(), "3 books")
}

func TestHint_SuppressedWhenQuiet(t *testing.T) {
	quiet = true
	defer func() { quiet = false }()

	var buf bytes.Buffer
	hint(&buf, "earworm download")
	assert.Empty(t, buf.String())
}
