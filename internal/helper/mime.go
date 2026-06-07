package helper

import (
	"fmt"
	"strings"

	pluralizelib "github.com/gertd/go-pluralize"
)

var textMIMEPrefixes = []string{
	"text/",
	"application/json",
	"application/xml",
	"application/javascript",
	"application/x-javascript",
	"application/typescript",
	"application/x-yaml",
	"application/toml",
	"application/x-sh",
}

// HumanizeBytes converts a byte count to a human-readable string.
func HumanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// StripBOM removes any UTF-8, UTF-16 BE, or UTF-16 LE byte order mark from b.
func StripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return b[2:]
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return b[2:]
	}
	return b
}

// IsBinaryContent returns true when the file header suggests binary content.
func IsBinaryContent(header []byte, contentType string) bool {
	if _, hasBOM := DetectTextEncoding(header); hasBOM {
		return false
	}
	for _, b := range header {
		if b == 0 {
			return true
		}
	}
	for _, t := range textMIMEPrefixes {
		if strings.HasPrefix(contentType, t) {
			return false
		}
	}
	return contentType == "application/octet-stream"
}

var pluralClient = pluralizelib.NewClient()

// Pluralize returns "1 word" or "N words" using English pluralisation.
func Pluralize(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %s", n, pluralClient.Plural(word))
}
