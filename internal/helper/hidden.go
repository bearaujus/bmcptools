package helper

import (
	"os"
	"path/filepath"
	"strings"
)

// IsHiddenPath reports whether path should be treated as hidden on this
// platform. Dotfiles remain hidden everywhere; Windows also honors hidden and
// system file attributes.
func IsHiddenPath(path string, info os.FileInfo) bool {
	name := filepath.Base(path)
	if info != nil && info.Name() != "" {
		name = info.Name()
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return pathHasHiddenAttribute(path)
}
