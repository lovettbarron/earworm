package fileops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// VerifiedCopy copies src to dst, verifying SHA-256 integrity.
// The source file is NOT deleted (this is copy, not move).
// Parent directories for dst are created automatically.
// If the hash check fails, the destination file is removed.
func VerifiedCopy(src, dst string) error {
	srcHash, err := HashFile(src)
	if err != nil {
		return fmt.Errorf("verified copy: %w", err)
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("verified copy mkdir: %w", err)
	}

	// Open source
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("verified copy open src: %w", err)
	}
	defer srcFile.Close()

	// Create destination
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("verified copy create dst: %w", err)
	}

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(dst)
		return fmt.Errorf("verified copy: %w", err)
	}

	// Close both files before hashing
	srcFile.Close()
	dstFile.Close()

	// Verify destination hash
	dstHash, err := HashFile(dst)
	if err != nil {
		os.Remove(dst)
		return fmt.Errorf("verified copy hash dst: %w", err)
	}

	if srcHash != dstHash {
		os.Remove(dst)
		return fmt.Errorf("hash mismatch after copy: src=%s dst=%s", srcHash, dstHash)
	}

	return nil
}
