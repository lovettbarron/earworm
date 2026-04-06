package download

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validVoucherJSON() string {
	return `{
  "content_license": {
    "asin": "B094XCNV6G",
    "license_response": {
      "key": "81ae9b20bd68a7696dde8dbfb51668d9",
      "iv": "7cb4fdc075f38fc96cb9230bd745dde1"
    }
  }
}`
}

func TestParseVoucher_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	voucherPath := filepath.Join(tmpDir, "book.voucher")
	require.NoError(t, os.WriteFile(voucherPath, []byte(validVoucherJSON()), 0644))

	v, err := ParseVoucher(voucherPath)
	require.NoError(t, err)
	assert.Equal(t, "B094XCNV6G", v.ContentLicense.ASIN)
	assert.Equal(t, "81ae9b20bd68a7696dde8dbfb51668d9", v.ContentLicense.LicenseResponse.Key)
	assert.Equal(t, "7cb4fdc075f38fc96cb9230bd745dde1", v.ContentLicense.LicenseResponse.IV)
}

func TestParseVoucher_MissingFile(t *testing.T) {
	_, err := ParseVoucher("/nonexistent/voucher.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading voucher file")
}

func TestParseVoucher_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	voucherPath := filepath.Join(tmpDir, "bad.voucher")
	require.NoError(t, os.WriteFile(voucherPath, []byte("not json"), 0644))

	_, err := ParseVoucher(voucherPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing voucher JSON")
}

func TestParseVoucher_MissingKey(t *testing.T) {
	tmpDir := t.TempDir()
	voucherPath := filepath.Join(tmpDir, "nokey.voucher")
	data := `{"content_license":{"license_response":{"key":"","iv":"abc123"}}}`
	require.NoError(t, os.WriteFile(voucherPath, []byte(data), 0644))

	_, err := ParseVoucher(voucherPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing decryption key")
}

func TestParseVoucher_MissingIV(t *testing.T) {
	tmpDir := t.TempDir()
	voucherPath := filepath.Join(tmpDir, "noiv.voucher")
	data := `{"content_license":{"license_response":{"key":"abc123","iv":""}}}`
	require.NoError(t, os.WriteFile(voucherPath, []byte(data), 0644))

	_, err := ParseVoucher(voucherPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing decryption IV")
}

// mockFfmpegFactory creates a CmdFactory that simulates ffmpeg by copying input to output.
// It uses a shell script that creates the output file (the last argument).
func mockFfmpegFactory(createOutput bool, exitCode int) CmdFactory {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if createOutput && exitCode == 0 {
			// The last argument is the output path.
			// Create a script that copies input to output.
			outputPath := args[len(args)-1]
			// Find input path (after -i flag)
			var inputPath string
			for i, arg := range args {
				if arg == "-i" && i+1 < len(args) {
					inputPath = args[i+1]
					break
				}
			}
			// Use cp to simulate ffmpeg decrypt (just copy the file)
			if inputPath != "" {
				return exec.CommandContext(ctx, "cp", inputPath, outputPath)
			}
			// Fallback: create a dummy output file
			return exec.CommandContext(ctx, "sh", "-c", "echo 'decrypted audio data' > '"+outputPath+"'")
		}
		// Simulate failure
		return exec.CommandContext(ctx, "sh", "-c", "echo 'ffmpeg error' >&2; exit "+string(rune('0'+exitCode)))
	}
}

func TestDecryptAAXC_Success(t *testing.T) {
	tmpDir := t.TempDir()
	aaxcPath := filepath.Join(tmpDir, "book.aaxc")
	require.NoError(t, os.WriteFile(aaxcPath, []byte("encrypted audio data"), 0644))

	voucher := &Voucher{}
	voucher.ContentLicense.LicenseResponse.Key = "testkey123"
	voucher.ContentLicense.LicenseResponse.IV = "testiv456"

	factory := mockFfmpegFactory(true, 0)

	m4bPath, err := DecryptAAXC(context.Background(), aaxcPath, voucher, factory)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "book.m4b"), m4bPath)
	assert.FileExists(t, m4bPath)

	// Verify output file has content
	info, err := os.Stat(m4bPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestDecryptAAXC_FfmpegFailure(t *testing.T) {
	tmpDir := t.TempDir()
	aaxcPath := filepath.Join(tmpDir, "book.aaxc")
	require.NoError(t, os.WriteFile(aaxcPath, []byte("encrypted audio data"), 0644))

	voucher := &Voucher{}
	voucher.ContentLicense.LicenseResponse.Key = "testkey123"
	voucher.ContentLicense.LicenseResponse.IV = "testiv456"

	factory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "echo 'ffmpeg error' >&2; exit 1")
	}

	_, err := DecryptAAXC(context.Background(), aaxcPath, voucher, factory)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg decrypt")
}

func TestFindAAXCAndVoucher_Found(t *testing.T) {
	tmpDir := t.TempDir()
	aaxcFile := filepath.Join(tmpDir, "Book_Title-AAX_44_128.aaxc")
	voucherFile := filepath.Join(tmpDir, "Book_Title-AAX_44_128.voucher")
	require.NoError(t, os.WriteFile(aaxcFile, []byte("aaxc"), 0644))
	require.NoError(t, os.WriteFile(voucherFile, []byte("voucher"), 0644))

	aaxc, voucher, err := FindAAXCAndVoucher(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, aaxcFile, aaxc)
	assert.Equal(t, voucherFile, voucher)
}

func TestFindAAXCAndVoucher_NoAAXC(t *testing.T) {
	tmpDir := t.TempDir()
	// Only create an m4a file, no aaxc
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "book.m4a"), []byte("audio"), 0644))

	aaxc, voucher, err := FindAAXCAndVoucher(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, aaxc)
	assert.Empty(t, voucher)
}

func TestFindAAXCAndVoucher_AAXCWithoutVoucher(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "book.aaxc"), []byte("aaxc"), 0644))

	_, _, err := FindAAXCAndVoucher(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no matching .voucher file")
}

func TestDecryptStaged_WithAAXC(t *testing.T) {
	tmpDir := t.TempDir()

	// Create AAXC file
	aaxcPath := filepath.Join(tmpDir, "Book-AAX_44_128.aaxc")
	require.NoError(t, os.WriteFile(aaxcPath, []byte("encrypted audio"), 0644))

	// Create voucher file
	voucherPath := filepath.Join(tmpDir, "Book-AAX_44_128.voucher")
	require.NoError(t, os.WriteFile(voucherPath, []byte(validVoucherJSON()), 0644))

	factory := mockFfmpegFactory(true, 0)

	err := DecryptStaged(context.Background(), tmpDir, factory)
	require.NoError(t, err)

	// M4B should exist
	m4bPath := filepath.Join(tmpDir, "Book-AAX_44_128.m4b")
	assert.FileExists(t, m4bPath)

	// Original AAXC and voucher should be removed
	assert.NoFileExists(t, aaxcPath)
	assert.NoFileExists(t, voucherPath)
}

func TestDefaultCmdFactory(t *testing.T) {
	cmd := DefaultCmdFactory(context.Background(), "echo", "hello")
	assert.NotNil(t, cmd)
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "hello")
}

func TestDecryptStaged_NoAAXC_Noop(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "book.m4a"), []byte("audio"), 0644))

	err := DecryptStaged(context.Background(), tmpDir, DefaultCmdFactory)
	require.NoError(t, err)

	// M4A should still be there, untouched
	assert.FileExists(t, filepath.Join(tmpDir, "book.m4a"))
}
