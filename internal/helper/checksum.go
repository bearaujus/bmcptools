package helper

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
)

// HashFile computes the checksum of the file at path using the given algorithm
// ("md5", "sha1", or "sha256") and returns the hex digest and file size.
func HashFile(path, algorithm string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("cannot open: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("cannot stat: %w", err)
	}
	if info.IsDir() {
		return "", 0, fmt.Errorf("path is a directory")
	}

	var h hash.Hash
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	default:
		h = sha256.New()
	}

	if _, err := io.Copy(h, f); err != nil {
		return "", 0, fmt.Errorf("read error: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}
