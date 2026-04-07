package main

import "strings"

// expandAlternation fully expands all {a,b} groups in pattern recursively.
// "{src,lib}/**/*.{ts,tsx}" → ["src/**/*.ts","src/**/*.tsx","lib/**/*.ts","lib/**/*.tsx"]
func expandAlternation(pattern string) []string {
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
		// Recursively expand the composed pattern to handle remaining {} groups.
		expanded := expandAlternation(prefix + strings.TrimSpace(a) + suffix)
		result = append(result, expanded...)
	}
	return result
}
