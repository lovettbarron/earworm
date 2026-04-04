package cli

import (
	"testing"

	"github.com/lovettbarron/earworm/internal/audible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuth_Success(t *testing.T) {
	// Override newAudibleClient to inject a fake that succeeds on Quickstart
	fake := &fakeAudibleClient{}

	origClient := newAudibleClient
	newAudibleClient = func() audible.AudibleClient { return fake }
	t.Cleanup(func() { newAudibleClient = origClient })

	out, err := executeCommand(t, "auth")
	require.NoError(t, err)
	assert.Contains(t, out, "Authentication successful")
}

func TestAuth_CommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "auth" {
			found = true
			break
		}
	}
	assert.True(t, found, "auth command should be registered on rootCmd")
}
