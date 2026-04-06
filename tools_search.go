package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// errBinaryFile is returned by grepFile / grepFileMultiline when the file appears binary.
var errBinaryFile = errors.New("binary file")

func registerSearchTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("search_files",
		mcp.WithDescription(td("search_files")),
		mcp.WithString("path", mcp.Description(pd("search_files", "path"))),
		mcp.WithString("pattern", mcp.Required(), mcp.Description(pd("search_files", "pattern"))),
		mcp.WithBoolean("recursive", mcp.Description(pd("search_files", "recursive"))),
		mcp.WithNumber("max_results", mcp.Description(pd("search_files", "max_results"))),
		mcp.WithBoolean("show_hidden", mcp.Description(pd("search_files", "show_hidden"))),
	), searchFilesHandler)

	s.AddTool(mcp.NewTool("grep_files",
		mcp.WithDescription(td("grep_files")),
		mcp.WithString("path", mcp.Description(pd("grep_files", "path"))),
		mcp.WithString("pattern", mcp.Required(), mcp.Description(pd("grep_files", "pattern"))),
		mcp.WithBoolean("use_regex", mcp.Description(pd("grep_files", "use_regex"))),
		mcp.WithBoolean("case_insensitive", mcp.Description(pd("grep_files", "case_insensitive"))),
		mcp.WithBoolean("recursive", mcp.Description(pd("grep_files", "recursive"))),
		mcp.WithNumber("context_lines", mcp.Description(pd("grep_files", "context_lines"))),
		mcp.WithNumber("max_results", mcp.Description(pd("grep_files", "max_results"))),
		mcp.WithNumber("offset", mcp.Description(pd("grep_files", "offset"))),
		mcp.WithString("glob",
			mcp.Description(pd("grep_files", "glob")),
		),
		mcp.WithString("output_mode",
			mcp.Description(pd("grep_files", "output_mode")),
		),
		mcp.WithBoolean("show_hidden", mcp.Description(pd("grep_files", "show_hidden"))),
		mcp.WithBoolean("multiline", mcp.Description(pd("grep_files", "multiline"))),
		mcp.WithNumber("max_file_size", mcp.Description(pd("grep_files", "max_file_size"))),
	), grepFilesHandler)
}

// ── search_files ─────────────────────────────────────────────────────────────

func searchFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	root := req.GetString("path", "")
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot get working directory: %v", err)), nil
		}
	}
	pattern := req.GetString("pattern", "")
	if pattern == "" {
		return mcp.NewToolResultError("pattern is required"), nil
	}

	recursive := req.GetBool("recursive", true)
	showHidden := req.GetBool("show_hidden", false)

	maxResults := 100
	if mr := req.GetFloat("max_results", 0); mr > 0 {
		maxResults = int(mr)
	}

	rootInfo, err := os.Stat(root)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", root, err)), nil
	}
	if !rootInfo.IsDir() {
		return mcp.NewToolResultError("path must be a directory"), nil
	}

	var matches []string

	walkFn := func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(matches) >= maxResults {
			return filepath.SkipAll
		}
		if !recursive && d.IsDir() && p != root {
			return filepath.SkipDir
		}
		if p == root {
			return nil
		}

		name := d.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, relErr := filepath.Rel(root, p)
		if relErr != nil {
			relPath = name
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		matched, matchErr := matchGlobPath(pattern, relPath)
		if matchErr != nil {
			return matchErr
		}
		if matched {
			matches = append(matches, p)
		}
		return nil
	}

	if err := filepath.WalkDir(root, walkFn); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("walk error: %v", err)), nil
	}

	if len(matches) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No files matched pattern %q under %s", pattern, root)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %s matching %q under %s:\n\n", pluralize(len(matches), "result"), pattern, root)
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			fmt.Fprintf(&sb, "  [?   ] %s\n", m)
			continue
		}
		if info.IsDir() {
			fmt.Fprintf(&sb, "  [dir ] %s\n", m)
		} else {
			size := humanizeBytes(info.Size())
			mod := info.ModTime().Format("2006-01-02 15:04")
			fmt.Fprintf(&sb, "  [file] %-50s %8s  %s\n", m, size, mod)
		}
	}
	if len(matches) == maxResults {
		fmt.Fprintf(&sb, "\n[Result limit of %d reached — refine your pattern or increase max_results]", maxResults)
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// matchGlobPath matches a glob pattern against a path relative to the search root.
// Supports *, **, ?, and {a,b} alternation (including multiple groups).
//
//   - Patterns without a path separator are matched against the basename only.
//   - Patterns containing ** are matched using recursive segment matching.
//   - Other patterns with / are matched against the full relative path.
func matchGlobPath(pattern, relPath string) (bool, error) {
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	// expandAlternation is fully recursive, so each alt has no more {} groups.
	alts := expandAlternation(pattern)
	for _, alt := range alts {
		ok, err := singleGlobPathMatch(alt, relPath)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// singleGlobPathMatch matches one pattern (no {}) against a relative path.
func singleGlobPathMatch(pattern, relPath string) (bool, error) {
	if !strings.ContainsRune(pattern, '/') {
		idx := strings.LastIndexByte(relPath, '/')
		name := relPath
		if idx >= 0 {
			name = relPath[idx+1:]
		}
		return filepath.Match(pattern, name)
	}
	if strings.Contains(pattern, "**") {
		return doubleStarMatch(strings.Split(pattern, "/"), strings.Split(relPath, "/"))
	}
	osSep := string(filepath.Separator)
	osPattern := strings.ReplaceAll(pattern, "/", osSep)
	osRelPath := strings.ReplaceAll(relPath, "/", osSep)
	return filepath.Match(osPattern, osRelPath)
}

// doubleStarMatch implements ** glob matching over path segments.
func doubleStarMatch(pats, segs []string) (bool, error) {
	for {
		if len(pats) == 0 {
			return len(segs) == 0, nil
		}
		if pats[0] == "**" {
			pats = pats[1:]
			if len(pats) == 0 {
				return true, nil
			}
			for i := 0; i <= len(segs); i++ {
				ok, err := doubleStarMatch(pats, segs[i:])
				if err != nil {
					return false, err
				}
				if ok {
					return true, nil
				}
			}
			return false, nil
		}
		if len(segs) == 0 {
			return false, nil
		}
		ok, err := filepath.Match(pats[0], segs[0])
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		pats = pats[1:]
		segs = segs[1:]
	}
}

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

// ── grep_files ───────────────────────────────────────────────────────────────

type grepMatch struct {
	file    string
	lineNum int
	line    string
	context []string
	after   []string
}

func grepFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	root := req.GetString("path", "")
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot get working directory: %v", err)), nil
		}
	}
	pattern := req.GetString("pattern", "")
	if pattern == "" {
		return mcp.NewToolResultError("pattern is required"), nil
	}

	useRegex := req.GetBool("use_regex", false)
	caseInsensitive := req.GetBool("case_insensitive", false)
	recursive := req.GetBool("recursive", true)
	globFilter := req.GetString("glob", "")
	showHidden := req.GetBool("show_hidden", false)
	multiline := req.GetBool("multiline", false)

	outputMode := req.GetString("output_mode", "content")
	switch outputMode {
	case "content", "files_with_matches", "count":
	default:
		outputMode = "content"
	}

	ctxLines := 0
	if outputMode == "content" {
		if cl := req.GetFloat("context_lines", 0); cl > 0 {
			ctxLines = int(cl)
			if ctxLines > 50 {
				ctxLines = 50
			}
		}
	}

	maxResults := 50
	if mr := req.GetFloat("max_results", 0); mr > 0 {
		maxResults = int(mr)
	}

	offset := 0
	if o := req.GetFloat("offset", 0); o > 0 {
		offset = int(o)
	}

	maxFileSize := int64(0)
	if mfs := req.GetFloat("max_file_size", 0); mfs > 0 {
		maxFileSize = int64(mfs)
	}

	// content mode: paginate by matches. count/files_with_matches: scan all (accurate totals).
	collectLimit := offset + maxResults
	if outputMode == "count" || outputMode == "files_with_matches" {
		collectLimit = 1 << 20 // effectively unlimited for aggregate modes
	}

	var matchFn func(string) bool
	var mlRegex *regexp.Regexp // non-nil when multiline=true

	if multiline {
		regexPat := pattern
		if !useRegex {
			regexPat = regexp.QuoteMeta(pattern)
		}
		flags := "(?s)"
		if caseInsensitive {
			flags = "(?si)"
		}
		var err error
		mlRegex, err = regexp.Compile(flags + regexPat)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid regex %q: %v", pattern, err)), nil
		}
	} else if useRegex {
		flags := ""
		if caseInsensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + pattern)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid regex %q: %v", pattern, err)), nil
		}
		matchFn = re.MatchString
	} else {
		needle := pattern
		if caseInsensitive {
			needle = strings.ToLower(pattern)
		}
		matchFn = func(line string) bool {
			if caseInsensitive {
				return strings.Contains(strings.ToLower(line), needle)
			}
			return strings.Contains(line, needle)
		}
	}

	filesToSearch, err := collectFiles(root, recursive, globFilter, showHidden)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var allMatches []grepMatch
	filesAttempted := 0
	binarySkipped := 0
	oversizeSkipped := 0
	var binarySkippedPaths []string
	limited := false
	totalEligible := len(filesToSearch)

	for _, filePath := range filesToSearch {
		if limited {
			break
		}
		filesAttempted++

		// Skip files larger than the user-specified size limit.
		if maxFileSize > 0 {
			if fi, statErr := os.Stat(filePath); statErr == nil && fi.Size() > maxFileSize {
				oversizeSkipped++
				continue
			}
		}

		rem := collectLimit - len(allMatches)
		if rem <= 0 {
			limited = true
			break
		}

		var matches []grepMatch
		var grepErr error
		if mlRegex != nil {
			matches, grepErr = grepFileMultiline(filePath, mlRegex, rem)
		} else {
			matches, grepErr = grepFile(filePath, matchFn, ctxLines, rem)
		}
		if grepErr != nil {
			if errors.Is(grepErr, errBinaryFile) {
				binarySkipped++
				binarySkippedPaths = append(binarySkippedPaths, filePath)
			}
			continue
		}
		allMatches = append(allMatches, matches...)
		if len(allMatches) >= collectLimit {
			limited = true
		}
	}

	// Build a concise search-context string shown in every output header.
	textFilesSearched := filesAttempted - binarySkipped - oversizeSkipped
	var ctxParts []string
	if limited {
		ctxParts = append(ctxParts, fmt.Sprintf("searched %d of %s", textFilesSearched, pluralize(totalEligible, "eligible file")))
	} else {
		ctxParts = append(ctxParts, fmt.Sprintf("searched %s", pluralize(textFilesSearched, "file")))
	}
	if binarySkipped > 0 {
		ctxParts = append(ctxParts, fmt.Sprintf("skipped %s", pluralize(binarySkipped, "binary file")))
	}
	if oversizeSkipped > 0 {
		ctxParts = append(ctxParts, fmt.Sprintf("skipped %s (oversized)", pluralize(oversizeSkipped, "file")))
	}
	searchCtx := strings.Join(ctxParts, ", ")

	// Total matches collected before any pagination slicing.
	totalCollected := len(allMatches)

	// Apply pagination offset/limit for content mode only.
	if outputMode == "content" {
		if offset > 0 {
			if offset >= len(allMatches) {
				allMatches = nil
			} else {
				allMatches = allMatches[offset:]
			}
		}
		if len(allMatches) > maxResults {
			allMatches = allMatches[:maxResults]
		}
	}

	if len(allMatches) == 0 {
		msg := fmt.Sprintf("No matches for %q in %s (%s)", pattern, root, searchCtx)
		msg += formatBinarySkippedFooter(binarySkippedPaths)
		return mcp.NewToolResultText(msg), nil
	}

	var sb strings.Builder

	switch outputMode {
	case "files_with_matches":
		seen := make(map[string]bool)
		var files []string
		for _, m := range allMatches {
			if !seen[m.file] {
				seen[m.file] = true
				files = append(files, m.file)
			}
		}
		fmt.Fprintf(&sb, "Found %s containing %q (%s):\n\n",
			pluralize(len(files), "file"), pattern, searchCtx)
		for _, f := range files {
			fmt.Fprintf(&sb, "  %s\n", f)
		}
		fmt.Fprintf(&sb, "\nTotal: %s across %s.",
			pluralize(totalCollected, "match"), pluralize(len(files), "file"))
		sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))

	case "count":
		fileCounts := make(map[string]int)
		var fileOrder []string
		for _, m := range allMatches {
			if fileCounts[m.file] == 0 {
				fileOrder = append(fileOrder, m.file)
			}
			fileCounts[m.file]++
		}
		fmt.Fprintf(&sb, "Match counts for %q (%s):\n\n", pattern, searchCtx)
		for _, f := range fileOrder {
			fmt.Fprintf(&sb, "  %s: %s\n", f, pluralize(fileCounts[f], "match"))
		}
		fmt.Fprintf(&sb, "\nTotal: %s across %s.",
			pluralize(totalCollected, "match"), pluralize(len(fileOrder), "file"))
		sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))

	default: // "content"
		// Build match count description — append "+" when limited (more may exist).
		matchDesc := pluralize(totalCollected, "match")
		if limited {
			matchDesc = fmt.Sprintf("%d+ matches", totalCollected)
		}
		fmt.Fprintf(&sb, "Found %s for %q (%s):\n\n", matchDesc, pattern, searchCtx)
		for _, m := range allMatches {
			for i, cl := range m.context {
				lineN := m.lineNum - len(m.context) + i
				fmt.Fprintf(&sb, "%s:%d- %s\n", m.file, lineN, cl)
			}
			fmt.Fprintf(&sb, "%s:%d: %s\n", m.file, m.lineNum, m.line)
			for i, al := range m.after {
				fmt.Fprintf(&sb, "%s:%d- %s\n", m.file, m.lineNum+i+1, al)
			}
			if ctxLines > 0 {
				fmt.Fprintf(&sb, "---\n")
			}
		}

		// Pagination footer — only when content was limited.
		if limited {
			nextOffset := offset + len(allMatches)
			fmt.Fprintf(&sb, "\n[Showing matches %d..%d of %d+ total — use offset=%d to see the next page]",
				offset+1, offset+len(allMatches), totalCollected, nextOffset)
		}
		sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// formatBinarySkippedFooter returns a footnote listing skipped binary files.
// Shows up to 10 names; truncates with "... and N more" for larger lists.
func formatBinarySkippedFooter(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	const maxShow = 10
	var sb strings.Builder
	sb.WriteString("\nSkipped binary files:\n")
	show := paths
	extra := 0
	if len(paths) > maxShow {
		show = paths[:maxShow]
		extra = len(paths) - maxShow
	}
	for _, p := range show {
		fmt.Fprintf(&sb, "  %s\n", p)
	}
	if extra > 0 {
		fmt.Fprintf(&sb, "  ... and %d more\n", extra)
	}
	return sb.String()
}

func grepFile(path string, matchFn func(string) bool, ctxLines, remaining int) ([]grepMatch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Quick binary check — read first 512 bytes.
	header := make([]byte, 512)
	n, _ := f.Read(header)
	for _, b := range header[:n] {
		if b == 0 {
			return nil, errBinaryFile
		}
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("seek error: %w", err)
	}

	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, 1024*1024)

	// before is a circular ring buffer holding up to ctxLines lines preceding the current one.
	before := make([]string, 0, ctxLines)
	beforeStart := 0 // index of the oldest entry in the ring buffer

	// pendingAfter tracks matches that still need after-context lines appended.
	type pendingEntry struct {
		matchIdx  int
		remaining int
	}
	var pending []pendingEntry

	var matches []grepMatch
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Deliver this line as after-context to all pending matches.
		for i := range pending {
			if pending[i].remaining > 0 {
				matches[pending[i].matchIdx].after = append(matches[pending[i].matchIdx].after, line)
				pending[i].remaining--
			}
		}
		// Prune fully-satisfied pending entries.
		nxt := pending[:0]
		for _, p := range pending {
			if p.remaining > 0 {
				nxt = append(nxt, p)
			}
		}
		pending = nxt

		// Check for a match (stop collecting new matches once the limit is reached).
		if len(matches) < remaining && matchFn(line) {
			m := grepMatch{
				file:    path,
				lineNum: lineNum,
				line:    line,
			}
			// Extract before-context from ring buffer (oldest → newest).
			if ctxLines > 0 && len(before) > 0 {
				ctx := make([]string, len(before))
				for j := range before {
					ctx[j] = before[(beforeStart+j)%len(before)]
				}
				m.context = ctx
			}
			if ctxLines > 0 {
				pending = append(pending, pendingEntry{matchIdx: len(matches), remaining: ctxLines})
			}
			matches = append(matches, m)
		}

		// Advance the ring buffer — the current line becomes before-context for future matches.
		if ctxLines > 0 {
			if len(before) < ctxLines {
				before = append(before, line)
			} else {
				before[beforeStart] = line
				beforeStart = (beforeStart + 1) % ctxLines
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

// grepFileMultiline searches path for all matches of re, which may span multiple lines.
// Returns matches with the start line number. Matched text has internal newlines replaced
// by " ↵ " for display.
func grepFileMultiline(path string, re *regexp.Regexp, remaining int) ([]grepMatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Binary check — first 512 bytes.
	sniff := data
	if len(sniff) > 512 {
		sniff = data[:512]
	}
	for _, b := range sniff {
		if b == 0 {
			return nil, errBinaryFile
		}
	}

	content := string(data)
	locs := re.FindAllStringIndex(content, remaining)
	if len(locs) == 0 {
		return nil, nil
	}

	var matches []grepMatch
	for _, loc := range locs {
		if len(matches) >= remaining {
			break
		}
		startLine := strings.Count(content[:loc[0]], "\n") + 1
		matchedText := content[loc[0]:loc[1]]
		// Flatten newlines for display; truncate very long matches.
		display := strings.ReplaceAll(matchedText, "\n", " ↵ ")
		const maxDisplay = 300
		if len(display) > maxDisplay {
			display = display[:maxDisplay] + "…"
		}
		matches = append(matches, grepMatch{
			file:    path,
			lineNum: startLine,
			line:    display,
		})
	}
	return matches, nil
}
