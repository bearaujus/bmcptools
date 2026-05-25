package helper

import (
	"path/filepath"
	"strings"
)

// ExpandAlternation fully expands all {a,b} groups in pattern recursively.
func ExpandAlternation(pattern string) []string {
	start := strings.Index(pattern, "{")
	end := strings.Index(pattern, "}")
	if start == -1 || end == -1 || end < start {
		return []string{pattern}
	}
	prefix := pattern[:start]
	suffix := pattern[end+1:]
	alternatives := strings.Split(pattern[start+1:end], ",")

	var result []string
	for _, a := range alternatives {
		expanded := ExpandAlternation(prefix + strings.TrimSpace(a) + suffix)
		result = append(result, expanded...)
	}
	return result
}

// MatchesAnyGlobName reports whether name matches any basename glob pattern.
// Patterns support the same simple {a,b} alternation expansion as other helpers.
func MatchesAnyGlobName(name string, patterns []string) bool {
	for _, pattern := range patterns {
		for _, alt := range ExpandAlternation(pattern) {
			if ok, _ := filepath.Match(alt, name); ok {
				return true
			}
		}
	}
	return false
}
