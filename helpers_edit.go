package main

import (
	"fmt"
	"regexp"
	"strings"
)

// applyEdit performs one find-replace operation on original and returns
// (modified, replacementCount, error).
func applyEdit(original, oldStr, newStr string, useRegex, replaceAll bool) (string, int, error) {
	if useRegex {
		re, err := regexp.Compile(oldStr)
		if err != nil {
			return "", 0, fmt.Errorf("invalid regex %q: %w", oldStr, err)
		}
		if !re.MatchString(original) {
			return original, 0, nil
		}
		if replaceAll {
			count := len(re.FindAllString(original, -1))
			return re.ReplaceAllString(original, newStr), count, nil
		}
		loc := re.FindStringIndex(original)
		if loc == nil {
			return original, 0, nil
		}
		result := original[:loc[0]] + newStr + original[loc[1]:]
		return result, 1, nil
	}

	// Plain-text mode.
	if !strings.Contains(original, oldStr) {
		return original, 0, nil
	}
	if replaceAll {
		count := strings.Count(original, oldStr)
		return strings.ReplaceAll(original, oldStr, newStr), count, nil
	}
	idx := strings.Index(original, oldStr)
	result := original[:idx] + newStr + original[idx+len(oldStr):]
	return result, 1, nil
}

// normalizeCRLF replaces all \r\n sequences with \n and reports whether any
// were found. Used to make pattern matching work correctly on Windows files
// that use CRLF line endings regardless of whether the pattern contains \r.
func normalizeCRLF(s string) (normalized string, hasCRLF bool) {
	if !strings.Contains(s, "\r\n") {
		return s, false
	}
	return strings.ReplaceAll(s, "\r\n", "\n"), true
}

// restoreCRLF converts all \n sequences back to \r\n. Call this before writing
// a file whose original content had CRLF line endings.
func restoreCRLF(s string) string {
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// findNearbyContext tries to locate the first non-empty line of oldStr in
// content and returns a formatted context block showing ±3 surrounding lines.
// This is used to give diagnostic hints when an edit_file pattern is not found.
// Returns empty string when no useful context can be provided.
func findNearbyContext(content, oldStr string) string {
	// Extract the first non-empty line of oldStr as a search key.
	needle := oldStr
	if idx := strings.Index(needle, "\n"); idx >= 0 {
		needle = needle[:idx]
	}
	needle = strings.TrimSpace(needle)
	if len(needle) < 4 {
		return ""
	}
	if len(needle) > 60 {
		needle = needle[:60]
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, needle) {
			start := max(0, i-3)
			end := min(len(lines)-1, i+3)
			var sb strings.Builder
			sb.WriteString("\nFirst line of search pattern found nearby — check surrounding context:\n")
			for j := start; j <= end; j++ {
				marker := "   "
				if j == i {
					marker = "→  "
				}
				fmt.Fprintf(&sb, "%s%4d│%s\n", marker, j+1, lines[j])
			}
			return sb.String()
		}
	}
	return ""
}
