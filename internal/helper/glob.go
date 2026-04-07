package helper

import "strings"

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
