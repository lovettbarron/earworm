package download

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Voucher represents the audible-cli voucher JSON structure containing
// decryption keys for AAXC files.
type Voucher struct {
	ContentLicense struct {
		ASIN            string `json:"asin"`
		LicenseResponse struct {
			Key string `json:"key"`
			IV  string `json:"iv"`
		} `json:"license_response"`
	} `json:"content_license"`
}

// ParseVoucher reads and parses a voucher JSON file, returning the
// decryption key and IV needed to decrypt an AAXC file.
func ParseVoucher(path string) (*Voucher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading voucher file %q: %w", path, err)
	}

	var v Voucher
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing voucher JSON %q: %w", path, err)
	}

	if v.ContentLicense.LicenseResponse.Key == "" {
		return nil, fmt.Errorf("voucher %q missing decryption key", path)
	}
	if v.ContentLicense.LicenseResponse.IV == "" {
		return nil, fmt.Errorf("voucher %q missing decryption IV", path)
	}

	return &v, nil
}

// CmdFactory creates exec.Cmd instances. Allows test injection.
type CmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

// DefaultCmdFactory uses os/exec to create commands.
func DefaultCmdFactory(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// DecryptAAXC decrypts an AAXC file to M4B using ffmpeg with the Audible
// decryption key and IV from the voucher. The output file replaces the .aaxc
// extension with .m4b. Returns the path to the decrypted M4B file.
//
// ffmpeg command: ffmpeg -audible_key KEY -audible_iv IV -i input.aaxc -c copy output.m4b
func DecryptAAXC(ctx context.Context, aaxcPath string, voucher *Voucher, cmdFactory CmdFactory) (string, error) {
	if cmdFactory == nil {
		cmdFactory = DefaultCmdFactory
	}

	key := voucher.ContentLicense.LicenseResponse.Key
	iv := voucher.ContentLicense.LicenseResponse.IV

	// Build output path: replace .aaxc with .m4b
	ext := filepath.Ext(aaxcPath)
	m4bPath := strings.TrimSuffix(aaxcPath, ext) + ".m4b"

	cmd := cmdFactory(ctx, "ffmpeg",
		"-audible_key", key,
		"-audible_iv", iv,
		"-i", aaxcPath,
		"-c", "copy",
		m4bPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg decrypt %q: %w\noutput: %s", aaxcPath, err, string(output))
	}

	// Verify output file exists and is non-empty
	info, err := os.Stat(m4bPath)
	if err != nil {
		return "", fmt.Errorf("decrypted file not found at %q: %w", m4bPath, err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("decrypted file %q has zero size", m4bPath)
	}

	return m4bPath, nil
}

// FindAAXCAndVoucher locates the .aaxc audio file and its matching .voucher
// file in the given staging directory. Returns paths to both files.
func FindAAXCAndVoucher(stagingDir string) (aaxcPath string, voucherPath string, err error) {
	aaxcFiles, err := filepath.Glob(filepath.Join(stagingDir, "*.aaxc"))
	if err != nil {
		return "", "", fmt.Errorf("globbing for .aaxc files in %q: %w", stagingDir, err)
	}
	if len(aaxcFiles) == 0 {
		return "", "", nil // no AAXC files -- not an error, may be a different format
	}

	aaxcPath = aaxcFiles[0] // use first match

	voucherFiles, err := filepath.Glob(filepath.Join(stagingDir, "*.voucher"))
	if err != nil {
		return "", "", fmt.Errorf("globbing for .voucher files in %q: %w", stagingDir, err)
	}
	if len(voucherFiles) == 0 {
		return "", "", fmt.Errorf("found .aaxc file %q but no matching .voucher file in %q", filepath.Base(aaxcPath), stagingDir)
	}

	voucherPath = voucherFiles[0]
	return aaxcPath, voucherPath, nil
}

// DecryptStaged finds and decrypts any AAXC file in the staging directory,
// removing the original .aaxc and .voucher files after successful decryption.
// If no AAXC files are found, returns nil (no-op for M4A downloads).
func DecryptStaged(ctx context.Context, stagingDir string, cmdFactory CmdFactory) error {
	aaxcPath, voucherPath, err := FindAAXCAndVoucher(stagingDir)
	if err != nil {
		return err
	}
	if aaxcPath == "" {
		return nil // no AAXC files, nothing to decrypt
	}

	voucher, err := ParseVoucher(voucherPath)
	if err != nil {
		return err
	}

	m4bPath, err := DecryptAAXC(ctx, aaxcPath, voucher, cmdFactory)
	if err != nil {
		return err
	}

	// Verify the M4B was created before removing originals
	if _, err := os.Stat(m4bPath); err != nil {
		return fmt.Errorf("decrypted file missing after decrypt: %w", err)
	}

	// Remove original .aaxc and .voucher files
	os.Remove(aaxcPath)
	os.Remove(voucherPath)

	return nil
}
