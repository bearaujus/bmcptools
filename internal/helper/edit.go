package helper

import (
	"fmt"
	"regexp"
	"strings"
)

// ApplyEdit performs one find-replace operation on original and returns
// (modified, replacementCount, error).
func ApplyEdit(original, oldStr, newStr string, useRegex, replaceAll bool) (string, int, error) {
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

// NormalizeCRLF replaces all \r\n sequences with \n and reports whether any
// were found.
func NormalizeCRLF(s string) (normalized string, hasCRLF bool) {
	if !strings.Contains(s, "\r\n") {
		return s, false
	}
	return strings.ReplaceAll(s, "\r\n", "\n"), true
}

// RestoreCRLF converts all \n sequences back to \r\n.
func RestoreCRLF(s string) string {
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// FindNearbyContext tries to locate the first non-empty line of oldStr in
// content and returns a formatted context block showing ±3 surrounding lines.
func FindNearbyContext(content, oldStr string) string {
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
