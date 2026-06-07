package helper

import (
	"fmt"
	"os"
)

// SymlinkMetadata describes a symlink target without traversing it for writes.
type SymlinkMetadata struct {
	Target      string
	TargetKind  string
	TargetSize  int64
	TargetIsDir bool
	Dangling    bool
}

// ReadSymlinkMetadata resolves a symlink's target type and size when possible.
func ReadSymlinkMetadata(path string) (SymlinkMetadata, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return SymlinkMetadata{}, err
	}
	meta := SymlinkMetadata{Target: target}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			meta.Dangling = true
			return meta, nil
		}
		return SymlinkMetadata{}, err
	}
	meta.TargetIsDir = info.IsDir()
	meta.TargetSize = info.Size()
	if info.IsDir() {
		meta.TargetKind = "directory"
	} else if info.Mode()&os.ModeSymlink != 0 {
		meta.TargetKind = "symlink"
	} else {
		meta.TargetKind = "file"
	}
	return meta, nil
}

// FormatSymlinkCompact describes a symlink target for compact tool output.
func FormatSymlinkCompact(meta SymlinkMetadata) string {
	if meta.Dangling {
		return fmt.Sprintf("-> %s (dangling)", meta.Target)
	}
	return fmt.Sprintf("-> %s (target: %s %s)", meta.Target, meta.TargetKind, HumanizeBytes(meta.TargetSize))
}
