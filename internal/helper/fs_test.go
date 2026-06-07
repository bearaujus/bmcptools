package helper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockFileReleasesRegistryEntry(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "file.txt")

	unlock := LockFile(absPath)
	fileLockRegistry.mu.Lock()
	if got := len(fileLockRegistry.locks); got != 1 {
		fileLockRegistry.mu.Unlock()
		unlock()
		t.Fatalf("expected one active lock entry, got %d", got)
	}
	fileLockRegistry.mu.Unlock()

	unlock()

	fileLockRegistry.mu.Lock()
	defer fileLockRegistry.mu.Unlock()
	if got := len(fileLockRegistry.locks); got != 0 {
		t.Fatalf("expected lock registry cleanup after unlock, got %d entries", got)
	}
}

func TestReadExistingTextForDiffOversizedSkipsRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.txt")
	content := []byte("abcdef")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	text, note, err := ReadExistingTextForDiff(path, info, 3)
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Fatalf("expected oversized diff source to skip content, got %q", text)
	}
	if note == "" {
		t.Fatal("expected oversized diff source note")
	}
}
