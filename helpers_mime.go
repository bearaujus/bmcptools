package main

import (
	"fmt"
	"strings"

	pluralizelib "github.com/gertd/go-pluralize"
)

// textMIMEPrefixes lists MIME-type prefixes that indicate human-readable text.
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

// humanizeBytes converts a byte count to a human-readable string (e.g. "1.2 MB").
func humanizeBytes(b int64) string {
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

// stripBOM removes any UTF-8, UTF-16 BE, or UTF-16 LE byte order mark from b.
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:] // UTF-8 BOM
	}
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return b[2:] // UTF-16 BE BOM
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return b[2:] // UTF-16 LE BOM
	}
	return b
}

// isBinaryContent returns true when the file header suggests binary content.
func isBinaryContent(header []byte, contentType string) bool {
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

// pluralize returns "1 word" or "N words" using production-grade English pluralisation.
func pluralize(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %s", n, pluralClient.Plural(word))
}
