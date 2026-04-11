package fileops

import (
	"fmt"
	"syscall"
)

// FreeSpace returns the available bytes on the filesystem containing path.
func FreeSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	// Available blocks * block size
	return stat.Bavail * uint64(stat.Bsize), nil
}

// CheckFreeSpace verifies the filesystem at path has at least requiredBytes available.
// Returns nil if sufficient space, or an error describing the shortfall.
func CheckFreeSpace(path string, requiredBytes uint64) error {
	available, err := FreeSpace(path)
	if err != nil {
		return fmt.Errorf("check free space: %w", err)
	}
	if available < requiredBytes {
		return fmt.Errorf("insufficient disk space at %s: need %d bytes, have %d bytes (%.1f GB short)",
			path, requiredBytes, available, float64(requiredBytes-available)/(1024*1024*1024))
	}
	return nil
}
