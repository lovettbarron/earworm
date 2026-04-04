package organize

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// MoveFile moves a file from src to dst. It first attempts os.Rename for
// same-filesystem moves (fast path). If the rename fails with EXDEV (cross-
// device link), it falls back to copy+verify+delete. Parent directories for
// dst are created automatically.
func MoveFile(src, dst string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Check for cross-filesystem error
	if errors.Is(err, syscall.EXDEV) {
		return copyVerifyDelete(src, dst)
	}

	return fmt.Errorf("rename %s -> %s: %w", src, dst, err)
}

// copyVerifyDelete copies src to dst, verifies the sizes match, then deletes
// the source. If the copy fails, any partial destination file is cleaned up.
// If the sizes don't match after copy, the destination is removed and an error
// is returned.
func copyVerifyDelete(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		// Clean up partial destination on failure (D-09)
		os.Remove(dst)
		return fmt.Errorf("copy failed: %w", err)
	}

	// Size verification before deleting source (D-10)
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source after copy: %w", err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return fmt.Errorf("stat destination after copy: %w", err)
	}

	if srcInfo.Size() != dstInfo.Size() {
		os.Remove(dst)
		return fmt.Errorf("size mismatch: src=%d dst=%d", srcInfo.Size(), dstInfo.Size())
	}

	// Sizes match -- safe to remove source
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("removing source after verified copy: %w", err)
	}

	return nil
}

// copyFile copies the file at src to dst, preserving the source file's
// permissions. The destination is created with O_CREATE|O_WRONLY|O_TRUNC.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copying data: %w", err)
	}

	return dstFile.Close()
}
