package fileops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/lovettbarron/earworm/internal/organize"
)

// HashFile computes the SHA-256 hex digest of the file at path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifiedMove hashes the source file, moves it via organize.MoveFile,
// then hashes the destination and returns an error if the hashes differ.
func VerifiedMove(src, dst string) error {
	srcHash, err := HashFile(src)
	if err != nil {
		return fmt.Errorf("verified move: %w", err)
	}

	if err := organize.MoveFile(src, dst); err != nil {
		return fmt.Errorf("verified move: %w", err)
	}

	dstHash, err := HashFile(dst)
	if err != nil {
		return fmt.Errorf("verified move: %w", err)
	}

	if srcHash != dstHash {
		return fmt.Errorf("hash mismatch after move: src=%s dst=%s", srcHash, dstHash)
	}

	return nil
}
