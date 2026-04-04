package audible

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is invoked by fakeCommand. It routes on args after "--".
// This is the standard Go subprocess testing pattern.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}

	switch os.Getenv("GO_HELPER_SCENARIO") {
	case "auth_ok":
		os.Exit(0)
	case "auth_fail":
		fmt.Fprint(os.Stderr, "Error: unauthorized - token expired")
		os.Exit(1)
	case "rate_limit":
		fmt.Fprint(os.Stderr, "Error: rate limit exceeded")
		os.Exit(1)
	case "generic_fail":
		fmt.Fprint(os.Stderr, "Error: something went wrong")
		os.Exit(1)
	case "library_export":
		// Find the output file path -- it's the argument after --output
		outputPath := ""
		for i, arg := range args {
			if arg == "--output" && i+1 < len(args) {
				outputPath = args[i+1]
				break
			}
		}
		if outputPath == "" {
			fmt.Fprint(os.Stderr, "no --output flag found")
			os.Exit(1)
		}
		jsonData := `[{"asin":"B08C6YJ1LS","title":"Project Hail Mary","subtitle":"A Novel","authors":"Andy Weir","narrators":"Ray Porter","series_title":"","series_sequence":"","runtime_length_min":970,"purchase_date":"2021-05-04","release_date":"2021-05-04","is_finished":true,"percent_complete":100,"genres":"Science Fiction","rating":"5","num_ratings":12345,"cover_url":"https://example.com/cover.jpg"},{"asin":"B09NRGLL4G","title":"The Kaiju Preservation Society","subtitle":"","authors":"John Scalzi","narrators":"Wil Wheaton","series_title":"","series_sequence":"","runtime_length_min":null,"purchase_date":"2022-03-15","release_date":"2022-03-15","is_finished":false,"percent_complete":45.5,"genres":"Science Fiction","rating":"4","num_ratings":null,"cover_url":""}]`
		if err := os.WriteFile(outputPath, []byte(jsonData), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "library_export_fail":
		fmt.Fprint(os.Stderr, "Error: unauthorized")
		os.Exit(1)
	case "slow":
		// Sleep long enough for context cancellation to take effect
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	os.Exit(0)
}

// fakeCommand returns a command factory that routes to TestHelperProcess with the given scenario.
func fakeCommand(scenario string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--"}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"GO_HELPER_SCENARIO="+scenario,
		)
		return cmd
	}
}

func TestCheckAuth_Success(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("auth_ok")))
	err := c.CheckAuth(context.Background())
	assert.NoError(t, err)
}

func TestCheckAuth_AuthError(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("auth_fail")))
	err := c.CheckAuth(context.Background())
	require.Error(t, err)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr), "expected *AuthError, got %T", err)
	assert.Contains(t, authErr.Message, "unauthorized")
}

func TestLibraryExport_Success(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("library_export")))
	items, err := c.LibraryExport(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)

	// First item: all fields populated
	assert.Equal(t, "B08C6YJ1LS", items[0].ASIN)
	assert.Equal(t, "Project Hail Mary", items[0].Title)
	assert.Equal(t, "Andy Weir", items[0].Authors)
	assert.Equal(t, "Ray Porter", items[0].Narrators)
	require.NotNil(t, items[0].RuntimeLengthMin)
	assert.Equal(t, 970, *items[0].RuntimeLengthMin)
	assert.Equal(t, 970, items[0].RuntimeMinutes())
	assert.True(t, items[0].IsFinished)
	assert.Equal(t, float64(100), items[0].PercentComplete)
	require.NotNil(t, items[0].NumRatings)
	assert.Equal(t, 12345, *items[0].NumRatings)

	// Second item: null runtime_length_min and num_ratings
	assert.Equal(t, "B09NRGLL4G", items[1].ASIN)
	assert.Nil(t, items[1].RuntimeLengthMin)
	assert.Equal(t, 0, items[1].RuntimeMinutes())
	assert.Nil(t, items[1].NumRatings)
	assert.False(t, items[1].IsFinished)
	assert.Equal(t, 45.5, items[1].PercentComplete)
}

func TestLibraryExport_AuthFailure(t *testing.T) {
	c := NewClient("audible", WithCmdFactory(fakeCommand("library_export_fail")))
	items, err := c.LibraryExport(context.Background())
	require.Error(t, err)
	assert.Nil(t, items)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr), "expected *AuthError, got %T", err)
}

func TestClassifyError(t *testing.T) {
	baseErr := fmt.Errorf("exit status 1")

	tests := []struct {
		name     string
		stderr   string
		wantType interface{}
	}{
		{
			name:     "unauthorized returns AuthError",
			stderr:   "Error: unauthorized - token expired",
			wantType: &AuthError{},
		},
		{
			name:     "expired returns AuthError",
			stderr:   "Error: session expired please re-authenticate",
			wantType: &AuthError{},
		},
		{
			name:     "auth keyword returns AuthError",
			stderr:   "Error: auth failure",
			wantType: &AuthError{},
		},
		{
			name:     "rate limit returns RateLimitError",
			stderr:   "Error: rate limit exceeded",
			wantType: &RateLimitError{},
		},
		{
			name:     "too many requests returns RateLimitError",
			stderr:   "Error: too many requests",
			wantType: &RateLimitError{},
		},
		{
			name:     "generic error returns CommandError",
			stderr:   "Error: something went wrong",
			wantType: &CommandError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyError("test-cmd", tt.stderr, 1, baseErr)
			require.Error(t, err)

			switch tt.wantType.(type) {
			case *AuthError:
				var target *AuthError
				assert.True(t, errors.As(err, &target), "expected *AuthError, got %T", err)
			case *RateLimitError:
				var target *RateLimitError
				assert.True(t, errors.As(err, &target), "expected *RateLimitError, got %T", err)
			case *CommandError:
				var target *CommandError
				assert.True(t, errors.As(err, &target), "expected *CommandError, got %T", err)
			}
		})
	}
}

func TestParseLibraryExport_ValidJSON(t *testing.T) {
	data := []byte(`[{"asin":"B001","title":"Test Book","runtime_length_min":120}]`)
	items, err := ParseLibraryExport(data)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "B001", items[0].ASIN)
	assert.Equal(t, "Test Book", items[0].Title)
	require.NotNil(t, items[0].RuntimeLengthMin)
	assert.Equal(t, 120, *items[0].RuntimeLengthMin)
}

func TestParseLibraryExport_NullFields(t *testing.T) {
	data := []byte(`[{"asin":"B002","title":"Podcast Episode","runtime_length_min":null,"num_ratings":null}]`)
	items, err := ParseLibraryExport(data)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Nil(t, items[0].RuntimeLengthMin)
	assert.Equal(t, 0, items[0].RuntimeMinutes())
	assert.Nil(t, items[0].NumRatings)
}

func TestParseLibraryExport_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	items, err := ParseLibraryExport(data)
	assert.Error(t, err)
	assert.Nil(t, items)
	assert.Contains(t, err.Error(), "parse library export")
}

func TestParseLibraryExport_EmptyArray(t *testing.T) {
	data := []byte(`[]`)
	items, err := ParseLibraryExport(data)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestAudibleStatus(t *testing.T) {
	tests := []struct {
		name     string
		item     LibraryItem
		expected string
	}{
		{
			name:     "finished book",
			item:     LibraryItem{IsFinished: true, PercentComplete: 100},
			expected: "finished",
		},
		{
			name:     "in progress book",
			item:     LibraryItem{IsFinished: false, PercentComplete: 45.5},
			expected: "in_progress",
		},
		{
			name:     "new book",
			item:     LibraryItem{IsFinished: false, PercentComplete: 0},
			expected: "new",
		},
		{
			name:     "finished takes priority over percent",
			item:     LibraryItem{IsFinished: true, PercentComplete: 50},
			expected: "finished",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.item.AudibleStatus())
		})
	}
}

func TestNewClient_WithProfilePath(t *testing.T) {
	c := NewClient("audible", WithProfilePath("/home/user/.audible"))
	// Verify the client is created (interface satisfaction)
	assert.NotNil(t, c)
}

func TestNewClient_WithCmdFactory(t *testing.T) {
	factory := fakeCommand("auth_ok")
	c := NewClient("audible", WithCmdFactory(factory))
	assert.NotNil(t, c)
}
