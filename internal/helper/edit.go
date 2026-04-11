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
		// Exact match failed — try trailing-whitespace-normalized fallback.
		if replaceAll {
			result, count := applyAllNormalized(original, oldStr, newStr)
			return result, count, nil
		}
		start, end := findNormalizedMatch(original, oldStr)
		if start < 0 {
			return original, 0, nil
		}
		return original[:start] + newStr + original[end:], 1, nil
	}
	if replaceAll {
		count := strings.Count(original, oldStr)
		return strings.ReplaceAll(original, oldStr, newStr), count, nil
	}
	idx := strings.Index(original, oldStr)
	result := original[:idx] + newStr + original[idx+len(oldStr):]
	return result, 1, nil
}

// stripTrailingPerLine strips trailing whitespace (spaces and tabs) from the
// end of each line. Leading whitespace (indentation) is preserved.
func stripTrailingPerLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// lineByteOffset returns the byte offset of the start of line lineIdx (0-based)
// in content. Returns len(content) if lineIdx >= number of lines.
func lineByteOffset(content string, lineIdx int) int {
	if lineIdx == 0 {
		return 0
	}
	count := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			count++
			if count == lineIdx {
				return i + 1
			}
		}
	}
	return len(content)
}

// CountNormalizedMatches counts how many times oldStr appears in content when
// trailing whitespace on each line is ignored. If content already contains
// oldStr as an exact substring, returns strings.Count(content, oldStr) instead
// (fast path).
func CountNormalizedMatches(content, oldStr string) int {
	if strings.Contains(content, oldStr) {
		return strings.Count(content, oldStr)
	}
	patLines := strings.Split(oldStr, "\n")
	contentLines := strings.Split(content, "\n")
	patLen := len(patLines)
	if patLen > len(contentLines) {
		return 0
	}
	count := 0
	for i := 0; i <= len(contentLines)-patLen; i++ {
		match := true
		for j, pl := range patLines {
			if strings.TrimRight(contentLines[i+j], " \t") != strings.TrimRight(pl, " \t") {
				match = false
				break
			}
		}
		if match {
			count++
			i += patLen - 1
		}
	}
	return count
}

// findNormalizedMatch finds the first occurrence of oldStr in content using
// trailing-whitespace-normalized line matching. Returns the byte range [start,
// end) in the ORIGINAL content (preserving its trailing whitespace). Returns
// (-1, -1) if no match is found. Should only be called when the exact
// strings.Contains check has already failed.
func findNormalizedMatch(content, oldStr string) (start, end int) {
	patLines := strings.Split(oldStr, "\n")
	contentLines := strings.Split(content, "\n")
	patLen := len(patLines)
	if patLen > len(contentLines) {
		return -1, -1
	}
	for i := 0; i <= len(contentLines)-patLen; i++ {
		match := true
		for j, pl := range patLines {
			if strings.TrimRight(contentLines[i+j], " \t") != strings.TrimRight(pl, " \t") {
				match = false
				break
			}
		}
		if match {
			s := lineByteOffset(content, i)
			matchedOrig := strings.Join(contentLines[i:i+patLen], "\n")
			return s, s + len(matchedOrig)
		}
	}
	return -1, -1
}

// applyAllNormalized replaces all trailing-whitespace-normalized matches of
// oldStr in content with newStr. Replacements are applied right-to-left to
// preserve offsets. Returns the modified string and number of replacements made.
func applyAllNormalized(content, oldStr, newStr string) (string, int) {
	patLines := strings.Split(oldStr, "\n")
	contentLines := strings.Split(content, "\n")
	patLen := len(patLines)
	if patLen > len(contentLines) {
		return content, 0
	}
	type matchRange struct{ start, end int }
	var ranges []matchRange
	for i := 0; i <= len(contentLines)-patLen; i++ {
		match := true
		for j, pl := range patLines {
			if strings.TrimRight(contentLines[i+j], " \t") != strings.TrimRight(pl, " \t") {
				match = false
				break
			}
		}
		if match {
			s := lineByteOffset(content, i)
			matchedOrig := strings.Join(contentLines[i:i+patLen], "\n")
			ranges = append(ranges, matchRange{s, s + len(matchedOrig)})
			i += patLen - 1
		}
	}
	if len(ranges) == 0 {
		return content, 0
	}
	// Apply right-to-left to preserve byte offsets.
	result := content
	for k := len(ranges) - 1; k >= 0; k-- {
		r := ranges[k]
		result = result[:r.start] + newStr + result[r.end:]
	}
	return result, len(ranges)
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
