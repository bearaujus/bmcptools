package main

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	pluralizelib "github.com/gertd/go-pluralize"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// ── MIME helpers ──────────────────────────────────────────────────────────────

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

// ── pluralization ─────────────────────────────────────────────────────────────

var pluralClient = pluralizelib.NewClient()

// pluralize returns "1 word" or "N words" using production-grade English pluralisation.
func pluralize(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %s", n, pluralClient.Plural(word))
}

// ── file read helpers ─────────────────────────────────────────────────────────

// readBinaryFile reads a binary file and returns a base64-encoded text summary.
func readBinaryFile(f *os.File, info os.FileInfo, contentType string, limit int) (string, error) {
	readN := limit
	if int(info.Size()) < readN {
		readN = int(info.Size())
	}
	data := make([]byte, readN)
	n, err := io.ReadFull(f, data)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("read error: %w", err)
	}
	data = data[:n]

	encoded := base64.StdEncoding.EncodeToString(data)
	msg := fmt.Sprintf(
		"[BINARY FILE]\nPath: %s\nSize: %s\nContent-Type: %s\n\nBase64:\n%s",
		info.Name(), humanizeBytes(info.Size()), contentType, encoded,
	)
	if int(info.Size()) > limit {
		msg += fmt.Sprintf(
			"\n\n[Truncated: first %s of %s shown]",
			humanizeBytes(int64(limit)), humanizeBytes(info.Size()),
		)
	}
	return msg, nil
}

// readFullText reads the entire file as text up to limit bytes.
// Returns (text, truncated, error). When truncated=true, the text includes a
// notice with total line count for navigation.
func readFullText(f *os.File, info os.FileInfo, limit int) (string, bool, error) {
	limited := io.LimitReader(f, int64(limit)+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", false, fmt.Errorf("read error: %w", err)
	}

	truncated := len(raw) > limit
	if truncated {
		raw = raw[:limit]
	}

	raw = stripBOM(raw)
	if !utf8.Valid(raw) {
		raw = []byte(strings.ToValidUTF8(string(raw), "\uFFFD"))
	}

	result := string(raw)
	if truncated {
		// Count total lines so the caller knows the file extent for navigation.
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr == nil {
			scanner := bufio.NewScanner(f)
			scanBuf := make([]byte, 64*1024)
			scanner.Buffer(scanBuf, 10*1024*1024)
			totalLines := 0
			for scanner.Scan() {
				totalLines++
			}
			result += fmt.Sprintf(
				"\n\n[TRUNCATED — showing first %s of %s (%s total). Use start_line/end_line to read specific sections.]",
				humanizeBytes(int64(limit)), humanizeBytes(info.Size()), pluralize(totalLines, "line"),
			)
		} else {
			result += fmt.Sprintf(
				"\n\n[TRUNCATED — showing first %s of %s. Use start_line/end_line to read specific sections.]",
				humanizeBytes(int64(limit)), humanizeBytes(info.Size()),
			)
		}
	}
	return result, truncated, nil
}

// sniffAndOpen opens the file at p, sniffs binary content, then returns an
// open file (seeked back to 0), the file info, contentType, and isBinary.
// The caller is responsible for closing the file.
func sniffAndOpen(p string) (f *os.File, info os.FileInfo, contentType string, binary bool, err error) {
	info, err = os.Stat(p)
	if err != nil {
		return nil, nil, "", false, fmt.Errorf("cannot stat: %w", err)
	}
	if info.IsDir() {
		return nil, nil, "", false, fmt.Errorf("path is a directory; use list_directory instead")
	}

	f, err = os.Open(p)
	if err != nil {
		return nil, nil, "", false, fmt.Errorf("cannot open: %w", err)
	}

	header := make([]byte, 512)
	n, _ := f.Read(header)
	header = header[:n]
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		f.Close()
		return nil, nil, "", false, fmt.Errorf("seek error: %w", seekErr)
	}

	contentType = http.DetectContentType(header)
	binary = isBinaryContent(header, contentType)
	return f, info, contentType, binary, nil
}

// ── edit helper ───────────────────────────────────────────────────────────────

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

// ── directory entry helper ────────────────────────────────────────────────────

// entryWithInfo pairs a directory entry with its cached FileInfo.
type entryWithInfo struct {
	entry os.DirEntry
	info  os.FileInfo
}

// ── diff helpers ──────────────────────────────────────────────────────────────

var dmp = diffmatchpatch.New()

// generateDiff returns a standard unified diff of changes between original and
// modified text. ctxLines unchanged lines surround each changed region.
// Returns an empty string when the texts are identical.
// Hunk headers use the full "@@ -a,b +c,d @@" format.
// Uses Myers diff algorithm via go-diff — no line-count limit.
func generateDiff(original, modified string, ctxLines int) string {
	if original == modified {
		return ""
	}

	// Compute line-level diffs using Myers algorithm.
	a, b, lineArray := dmp.DiffLinesToChars(original, modified)
	rawDiffs := dmp.DiffMain(a, b, false)
	lineDiffs := dmp.DiffCharsToLines(rawDiffs, lineArray)

	// Build a flat script with per-line original/new line numbers.
	type scriptLine struct {
		op    byte   // '=' unchanged  '-' removed  '+' added
		text  string
		oLine int    // 1-based line in original (0 for pure insertions)
		nLine int    // 1-based line in new      (0 for pure deletions)
	}
	var script []scriptLine
	oLine, nLine := 0, 0
	for _, d := range lineDiffs {
		lines := strings.Split(d.Text, "\n")
		// DiffLinesToChars always terminates segments with \n; drop the trailing empty.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for _, l := range lines {
			switch d.Type {
			case diffmatchpatch.DiffEqual:
				oLine++
				nLine++
				script = append(script, scriptLine{'=', l, oLine, nLine})
			case diffmatchpatch.DiffDelete:
				oLine++
				script = append(script, scriptLine{'-', l, oLine, 0})
			case diffmatchpatch.DiffInsert:
				nLine++
				script = append(script, scriptLine{'+', l, 0, nLine})
			}
		}
	}

	// Determine which lines to show (changed ± ctxLines context).
	show := make([]bool, len(script))
	for i, sl := range script {
		if sl.op != '=' {
			from := i - ctxLines
			if from < 0 {
				from = 0
			}
			to := i + ctxLines
			if to >= len(script) {
				to = len(script) - 1
			}
			for k := from; k <= to; k++ {
				show[k] = true
			}
		}
	}

	// Collect hunks — each hunk is a contiguous run of shown lines.
	type hunkSpec struct {
		oStart, nStart int
		oCount, nCount int
		lines          []string
	}
	var hunks []hunkSpec
	var cur *hunkSpec
	lastOLine, lastNLine := 0, 0

	for i, sl := range script {
		if !show[i] {
			if cur != nil {
				hunks = append(hunks, *cur)
				cur = nil
			}
			if sl.oLine > 0 {
				lastOLine = sl.oLine
			}
			if sl.nLine > 0 {
				lastNLine = sl.nLine
			}
			continue
		}

		if cur == nil {
			cur = &hunkSpec{}
			// Determine hunk start: scan ahead within the contiguous shown window
			// to find the first origLine and newLine in this hunk.
			for j := i; j < len(script) && show[j]; j++ {
				if cur.oStart == 0 && script[j].oLine > 0 {
					cur.oStart = script[j].oLine
				}
				if cur.nStart == 0 && script[j].nLine > 0 {
					cur.nStart = script[j].nLine
				}
				if cur.oStart > 0 && cur.nStart > 0 {
					break
				}
			}
			// Fallback for pure-insertion hunk (no orig lines in window).
			if cur.oStart == 0 {
				cur.oStart = lastOLine
			}
			// Fallback for pure-deletion hunk (no new lines in window).
			if cur.nStart == 0 {
				cur.nStart = lastNLine + 1
			}
		}

		switch sl.op {
		case '=':
			cur.lines = append(cur.lines, " "+sl.text)
			cur.oCount++
			cur.nCount++
			lastOLine = sl.oLine
			lastNLine = sl.nLine
		case '-':
			cur.lines = append(cur.lines, "-"+sl.text)
			cur.oCount++
			lastOLine = sl.oLine
		case '+':
			cur.lines = append(cur.lines, "+"+sl.text)
			cur.nCount++
			lastNLine = sl.nLine
		}
	}
	if cur != nil {
		hunks = append(hunks, *cur)
	}

	var sb strings.Builder
	for i, h := range hunks {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", h.oStart, h.oCount, h.nStart, h.nCount)
		for _, l := range h.lines {
			sb.WriteString(l)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// ── file helpers ──────────────────────────────────────────────────────────────

// readOneFileAsText reads a single file as text, returning base64 summary for binary files.
func readOneFileAsText(p string, limitBytes int) (string, error) {
	f, info, contentType, binary, err := sniffAndOpen(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if binary {
		return readBinaryFile(f, info, contentType, limitBytes)
	}
	text, _, err := readFullText(f, info, limitBytes)
	return text, err
}

// countLines counts the total number of lines in an open file.
func countLines(f *os.File) (int, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 10*1024*1024)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// collectFiles walks root and returns all matching file paths.
// Hidden files and directories (names starting with '.') are skipped unless showHidden is true.
func collectFiles(root string, recursive bool, globPattern string, showHidden bool) ([]string, error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot stat %q: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return []string{root}, nil
	}

	var files []string
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		name := d.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !recursive && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if globPattern != "" {
			matched := false
			for _, alt := range expandAlternation(globPattern) {
				if ok, _ := filepath.Match(alt, name); ok {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}
		files = append(files, p)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk error: %w", walkErr)
	}
	return files, nil
}

// copyFileData copies src to dst with mode, discarding byte count.
func copyFileData(src, dst string, mode os.FileMode) error {
	_, err := copyFileDataN(src, dst, mode)
	return err
}

// copyFileDataN copies src to dst with mode and returns bytes copied.
func copyFileDataN(src, dst string, mode os.FileMode) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("cannot open source: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		in.Close()
		return 0, fmt.Errorf("cannot create destination: %w", err)
	}

	n, copyErr := io.Copy(out, in)
	in.Close()
	if closeErr := out.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	return n, copyErr
}

// ── content helpers ───────────────────────────────────────────────────────────

// countContentLines returns the number of lines in content.
// An empty string has 0 lines. A string with no trailing newline counts its last
// partial line.
func countContentLines(content string) int {
	if content == "" {
		return 0
	}
	n := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		n++
	}
	return n
}

// applyReplaceToFile reads filePath, applies the replacement, and writes back.
// Returns (count, diffStr, skippedBinary, error).
func applyReplaceToFile(filePath, oldStr, newStr string, useRegex, dryRun, produceDiff bool) (int, string, bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, "", false, err
	}
	// Skip binary files — check first 512 bytes only (same heuristic as sniffAndOpen).
	sniff := data
	if len(sniff) > 512 {
		sniff = data[:512]
	}
	if isBinaryContent(sniff, http.DetectContentType(sniff)) {
		return 0, "", true, nil
	}

	original := string(data)
	modified, count, editErr := applyEdit(original, oldStr, newStr, useRegex, true)
	if editErr != nil {
		return 0, "", false, editErr
	}
	if count == 0 {
		return 0, "", false, nil
	}

	diff := ""
	if produceDiff {
		diff = generateDiff(original, modified, 2)
	}

	if !dryRun {
		if err := os.WriteFile(filePath, []byte(modified), 0o644); err != nil {
			return count, diff, false, err
		}
	}
	return count, diff, false, nil
}

// ── checksum helper ───────────────────────────────────────────────────────────

// hashFile computes the checksum of the file at path using the given algorithm
// ("md5", "sha1", or "sha256") and returns the hex digest and file size.
func hashFile(path, algorithm string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("cannot open: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("cannot stat: %w", err)
	}
	if info.IsDir() {
		return "", 0, fmt.Errorf("path is a directory")
	}

	var h hash.Hash
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	default:
		h = sha256.New()
	}

	if _, err := io.Copy(h, f); err != nil {
		return "", 0, fmt.Errorf("read error: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}
