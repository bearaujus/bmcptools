package search

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

var errBinaryFile = errors.New("binary file")

const (
	defaultMultilineMaxFileSize = 2 * 1024 * 1024
	maxDeepContentOffset        = 5000
	defaultMaxMatchesPerFile    = 20
)

func effectiveExcludePatterns(req mcp.CallToolRequest) []string {
	if _, provided := req.GetArguments()["exclude_patterns"]; provided {
		return req.GetStringSlice("exclude_patterns", nil)
	}
	return helper.DefaultTraversalExcludePatterns()
}

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
	excludePatterns := effectiveExcludePatterns(req)
	useRegex := req.GetBool("use_regex", false)
	caseInsensitive := req.GetBool("case_insensitive", false)
	outputMode := req.GetString("output_mode", "paths")
	if outputMode != "details" {
		outputMode = "paths"
	}
	pathFormat := normalizePathFormat(req.GetString("path_format", "relative"))
	entryType := req.GetString("entry_type", "any")
	switch entryType {
	case "file", "dir", "any":
	default:
		entryType = "any"
	}

	maxResults := 100
	if mr := req.GetFloat("max_results", 0); mr > 0 {
		maxResults = int(mr)
	}
	offset := 0
	if o := req.GetFloat("offset", 0); o > 0 {
		offset = int(o)
	}

	rootInfo, err := os.Stat(root)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", root, err)), nil
	}
	if !rootInfo.IsDir() {
		return mcp.NewToolResultError("path must be a directory"), nil
	}

	matchPath, err := makeNameMatcher(pattern, useRegex, caseInsensitive)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var matches []string
	collectLimit := offset + maxResults + 1

	walkFn := func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(matches) >= collectLimit {
			return filepath.SkipAll
		}
		if !recursive && d.IsDir() && p != root {
			return filepath.SkipDir
		}
		if p == root {
			return nil
		}

		name := d.Name()
		if !showHidden && helper.IsHiddenPath(p, nil) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if helper.MatchesAnyGlobName(name, excludePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if (entryType == "file" && d.IsDir()) || (entryType == "dir" && !d.IsDir()) {
			return nil
		}

		relPath, relErr := filepath.Rel(root, p)
		if relErr != nil {
			relPath = name
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		matched, matchErr := matchPath(relPath)
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
		return mcp.NewToolResultText(fmt.Sprintf("No results matched pattern %q under %s", pattern, root)), nil
	}
	if offset >= len(matches) {
		return mcp.NewToolResultText(fmt.Sprintf("No results remain for pattern %q at offset=%d under %s", pattern, offset, root)), nil
	}

	end := len(matches)
	limited := false
	if maxResults > 0 && offset+maxResults < end {
		end = offset + maxResults
		limited = true
	}
	page := matches[offset:end]
	totalLabel := helper.Pluralize(len(matches), "result")
	if limited {
		totalLabel = fmt.Sprintf("%d+ results", end+1)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %s matching %q under %s:\n\n", totalLabel, pattern, root)
	for _, m := range page {
		displayPath := formatDisplayPath(root, m, pathFormat)
		if outputMode == "paths" {
			fmt.Fprintf(&sb, "  %s\n", displayPath)
			continue
		}
		info, err := os.Stat(m)
		if err != nil {
			fmt.Fprintf(&sb, "  [?   ] %s\n", displayPath)
			continue
		}
		if info.IsDir() {
			fmt.Fprintf(&sb, "  [dir ] %s\n", displayPath)
		} else {
			size := helper.HumanizeBytes(info.Size())
			mod := info.ModTime().Format("2006-01-02 15:04")
			fmt.Fprintf(&sb, "  [file] %-50s %8s  %s\n", displayPath, size, mod)
		}
	}
	if limited {
		fmt.Fprintf(&sb, "\n[Showing results %d..%d \u2014 use offset=%d to see the next page.]", offset+1, end, end)
	} else if offset > 0 {
		fmt.Fprintf(&sb, "\n[Showing results %d..%d of %d.]", offset+1, end, len(matches))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func makeNameMatcher(pattern string, useRegex, caseInsensitive bool) (func(string) (bool, error), error) {
	if useRegex {
		regexPattern := pattern
		if caseInsensitive {
			regexPattern = "(?i)" + regexPattern
		}
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
		}
		matchRelativePath := strings.Contains(pattern, "/")
		return func(relPath string) (bool, error) {
			target := relPath
			if !matchRelativePath {
				if idx := strings.LastIndexByte(relPath, '/'); idx >= 0 {
					target = relPath[idx+1:]
				}
			}
			return re.MatchString(target), nil
		}, nil
	}

	globPattern := pattern
	if caseInsensitive {
		globPattern = strings.ToLower(globPattern)
	}
	return func(relPath string) (bool, error) {
		target := relPath
		if caseInsensitive {
			target = strings.ToLower(target)
		}
		return matchGlobPath(globPattern, target)
	}, nil
}

func normalizePathFormat(value string) string {
	if value == "absolute" {
		return "absolute"
	}
	return "relative"
}

func formatDisplayPath(root, path, pathFormat string) string {
	if pathFormat == "absolute" {
		abs, err := filepath.Abs(path)
		if err == nil {
			return abs
		}
		return path
	}

	base := root
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		base = filepath.Dir(root)
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return filepath.ToSlash(filepath.Base(path))
	}
	return filepath.ToSlash(rel)
}

func matchGlobPath(pattern, relPath string) (bool, error) {
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	alts := helper.ExpandAlternation(pattern)
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
	excludePatterns := effectiveExcludePatterns(req)
	pathFormat := normalizePathFormat(req.GetString("path_format", "relative"))

	outputMode := req.GetString("output_mode", "auto")
	switch outputMode {
	case "content", "files_with_matches", "count", "auto":
	default:
		outputMode = "auto"
	}

	ctxLines := 0
	if outputMode == "content" || outputMode == "auto" {
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
	maxMatchesPerFile := 0
	if outputMode == "content" || outputMode == "auto" {
		maxMatchesPerFile = defaultMaxMatchesPerFile
		if _, provided := req.GetArguments()["max_matches_per_file"]; provided {
			switch mm := int(req.GetFloat("max_matches_per_file", float64(defaultMaxMatchesPerFile))); {
			case mm > 0:
				maxMatchesPerFile = mm
			case mm == 0:
				maxMatchesPerFile = 0
			}
		}
	}

	offset := 0
	if o := req.GetFloat("offset", 0); o > 0 {
		offset = int(o)
	}
	if (outputMode == "content" || outputMode == "auto") && offset > maxDeepContentOffset {
		return mcp.NewToolResultError(
			fmt.Sprintf(
				"offset=%d is too deep for %s paging because grep_files must rescan earlier matches on each page; narrow the search with glob/exclude_patterns or use output_mode=\"files_with_matches\"/\"count\" first",
				offset,
				outputMode,
			),
		), nil
	}

	maxFileSize := int64(0)
	if _, provided := req.GetArguments()["max_file_size"]; provided {
		if mfs := req.GetFloat("max_file_size", 0); mfs > 0 {
			maxFileSize = int64(mfs)
		}
	} else if multiline {
		maxFileSize = defaultMultilineMaxFileSize
	}

	collectLimit := offset + maxResults

	var matchFn func(string) bool
	var mlRegex *regexp.Regexp

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

	filesToSearch, err := helper.CollectFileEntries(root, recursive, globFilter, showHidden, excludePatterns)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	type grepFileResult struct {
		matches    []grepMatch
		matchCount int
		attempted  bool
		binary     bool
		oversized  bool
		err        error
	}

	results := make([]grepFileResult, len(filesToSearch))
	totalEligible := len(filesToSearch)
	limitByMatches := outputMode == "content" || outputMode == "auto"
	countOnlyMode := outputMode == "count" || outputMode == "files_with_matches"
	var reservedMatches atomic.Int64
	helper.RunIOBoundedParallelWhile(len(filesToSearch), func(i int) bool {
		if limitByMatches && reservedMatches.Load() >= int64(collectLimit) {
			return false
		}

		entry := filesToSearch[i]
		results[i].attempted = true
		if maxFileSize > 0 {
			if entry.Info != nil && entry.Info.Size() > maxFileSize {
				results[i].oversized = true
				return true
			}
			if entry.Info == nil {
				if fi, statErr := os.Stat(entry.Path); statErr == nil && fi.Size() > maxFileSize {
					results[i].oversized = true
					return true
				}
			}
		}

		remaining := collectLimit
		reserved := 0
		if limitByMatches {
			want := collectLimit
			if maxMatchesPerFile > 0 && maxMatchesPerFile < want {
				want = maxMatchesPerFile
			}
			reserved = reserveMatchBudget(&reservedMatches, collectLimit, want)
			if reserved <= 0 {
				return false
			}
			remaining = reserved
		}

		var matches []grepMatch
		matchCount := 0
		var grepErr error
		switch {
		case countOnlyMode && mlRegex != nil:
			matchCount, grepErr = grepFileMultilineCount(entry.Path, mlRegex, outputMode == "files_with_matches")
		case countOnlyMode:
			matchCount, grepErr = grepFileCount(entry.Path, matchFn, outputMode == "files_with_matches")
		case mlRegex != nil:
			matches, grepErr = grepFileMultiline(entry.Path, mlRegex, remaining)
		default:
			matches, grepErr = grepFile(entry.Path, matchFn, ctxLines, remaining)
		}
		if grepErr != nil {
			if reserved > 0 {
				releaseMatchBudget(&reservedMatches, reserved, 0)
			}
			if errors.Is(grepErr, errBinaryFile) {
				results[i].binary = true
				return true
			}
			results[i].err = grepErr
			return true
		}
		results[i].matches = matches
		results[i].matchCount = matchCount
		if reserved > 0 {
			releaseMatchBudget(&reservedMatches, reserved, len(matches))
		}
		return true
	})

	var allMatches []grepMatch
	type fileMatchSummary struct {
		file  string
		count int
	}
	var fileSummaries []fileMatchSummary
	filesAttempted := 0
	binarySkipped := 0
	oversizeSkipped := 0
	var binarySkippedPaths []string
	limited := false
	for i, result := range results {
		if !result.attempted {
			continue
		}
		filesAttempted++
		if result.oversized {
			oversizeSkipped++
			continue
		}
		if result.binary {
			binarySkipped++
			binarySkippedPaths = append(binarySkippedPaths, formatDisplayPath(root, filesToSearch[i].Path, pathFormat))
			continue
		}
		if result.err != nil {
			continue
		}
		if countOnlyMode {
			if result.matchCount == 0 {
				continue
			}
			fileSummaries = append(fileSummaries, fileMatchSummary{
				file:  formatDisplayPath(root, filesToSearch[i].Path, pathFormat),
				count: result.matchCount,
			})
			continue
		}
		for _, match := range result.matches {
			if limitByMatches && len(allMatches) >= collectLimit {
				limited = true
				break
			}
			match.file = formatDisplayPath(root, match.file, pathFormat)
			allMatches = append(allMatches, match)
		}
	}
	if limitByMatches && (len(allMatches) >= collectLimit || filesAttempted < totalEligible) {
		limited = true
	}

	textFilesSearched := filesAttempted - binarySkipped - oversizeSkipped
	var ctxParts []string
	if limited {
		ctxParts = append(ctxParts, fmt.Sprintf("searched %d of %s", textFilesSearched, helper.Pluralize(totalEligible, "eligible file")))
	} else {
		ctxParts = append(ctxParts, fmt.Sprintf("searched %s", helper.Pluralize(textFilesSearched, "file")))
	}
	if binarySkipped > 0 {
		ctxParts = append(ctxParts, fmt.Sprintf("skipped %s", helper.Pluralize(binarySkipped, "binary file")))
	}
	if oversizeSkipped > 0 {
		ctxParts = append(ctxParts, fmt.Sprintf("skipped %s (oversized)", helper.Pluralize(oversizeSkipped, "file")))
	}
	searchCtx := strings.Join(ctxParts, ", ")

	totalCollected := len(allMatches)
	if countOnlyMode {
		totalCollected = 0
		for _, summary := range fileSummaries {
			totalCollected += summary.count
		}
	}

	if outputMode == "content" || outputMode == "auto" {
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
		if countOnlyMode && len(fileSummaries) > 0 {
			// Continue into paged count/files output below.
		} else {
			msg := fmt.Sprintf("No matches for %q in %s (%s)", pattern, root, searchCtx)
			if !useRegex && containsRegexMetachars(pattern) {
				msg += "\nHint: your pattern contains regex metacharacters (|, ., *, +, ^, $, [, (, \\). Set use_regex=true to enable Go regex syntax."
			}
			msg += formatBinarySkippedFooter(binarySkippedPaths)
			return mcp.NewToolResultText(msg), nil
		}
	}

	var sb strings.Builder

	switch outputMode {
	case "auto":
		if !limited {
			matchDesc := helper.Pluralize(totalCollected, "match")
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
			sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))
		} else {
			seen := make(map[string]bool)
			var files []string
			for _, m := range allMatches {
				if !seen[m.file] {
					seen[m.file] = true
					files = append(files, m.file)
				}
			}
			fmt.Fprintf(&sb, "Found %d+ matches for %q across %s (%s) — showing file list (results were capped).\n"+
				"Use output_mode:\"content\" to see matching lines, or increase max_results.\n\n",
				totalCollected, pattern, helper.Pluralize(len(files), "file"), searchCtx)
			for _, f := range files {
				fmt.Fprintf(&sb, "  %s\n", f)
			}
			sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))
		}

	case "files_with_matches":
		totalFiles := len(fileSummaries)
		if offset >= totalFiles {
			fmt.Fprintf(&sb, "No matching files remain for %q at offset=%d (%s total matching files; %s).\n",
				pattern, offset, helper.Pluralize(totalFiles, "file"), searchCtx)
			fmt.Fprintf(&sb, "Total: %s across %s.", helper.Pluralize(totalCollected, "match"), helper.Pluralize(totalFiles, "file"))
			sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))
			break
		}
		end := totalFiles
		if maxResults > 0 && offset+maxResults < end {
			end = offset + maxResults
			limited = true
		}
		page := fileSummaries[offset:end]
		fmt.Fprintf(&sb, "Found %s containing %q (%s):\n\n",
			helper.Pluralize(totalFiles, "file"), pattern, searchCtx)
		for _, summary := range page {
			fmt.Fprintf(&sb, "  %s\n", summary.file)
		}
		fmt.Fprintf(&sb, "\nTotal: %s across %s.",
			helper.Pluralize(totalCollected, "match"), helper.Pluralize(totalFiles, "file"))
		if limited {
			fmt.Fprintf(&sb, "\n[Showing files %d..%d of %d — use offset=%d to see the next page.]",
				offset+1, end, totalFiles, end)
		}
		sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))

	case "count":
		totalFiles := len(fileSummaries)
		if offset >= totalFiles {
			fmt.Fprintf(&sb, "No matching files remain for %q at offset=%d (%s total matching files; %s).\n",
				pattern, offset, helper.Pluralize(totalFiles, "file"), searchCtx)
			fmt.Fprintf(&sb, "Total: %s across %s.", helper.Pluralize(totalCollected, "match"), helper.Pluralize(totalFiles, "file"))
			sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))
			break
		}
		end := totalFiles
		if maxResults > 0 && offset+maxResults < end {
			end = offset + maxResults
			limited = true
		}
		fmt.Fprintf(&sb, "Match counts for %q (%s):\n\n", pattern, searchCtx)
		for _, summary := range fileSummaries[offset:end] {
			fmt.Fprintf(&sb, "  %s: %s\n", summary.file, helper.Pluralize(summary.count, "match"))
		}
		fmt.Fprintf(&sb, "\nTotal: %s across %s.",
			helper.Pluralize(totalCollected, "match"), helper.Pluralize(totalFiles, "file"))
		if limited {
			fmt.Fprintf(&sb, "\n[Showing files %d..%d of %d — use offset=%d to see the next page.]",
				offset+1, end, totalFiles, end)
		}
		sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))

	default:
		matchDesc := helper.Pluralize(totalCollected, "match")
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
		if limited {
			nextOffset := offset + len(allMatches)
			fmt.Fprintf(&sb, "\n[Showing matches %d..%d of %d+ total \u2014 use offset=%d to see the next page]",
				offset+1, offset+len(allMatches), totalCollected, nextOffset)
		}
		sb.WriteString(formatBinarySkippedFooter(binarySkippedPaths))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func containsRegexMetachars(s string) bool {
	return strings.ContainsAny(s, `|.+*?^$()[]{}\`)
}

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
	scanner, closeFn, binary, err := helper.OpenTextScanner(path, 1024*1024)
	if err != nil {
		return nil, err
	}
	if closeFn != nil {
		defer closeFn()
	}
	if binary {
		return nil, errBinaryFile
	}

	before := make([]string, 0, ctxLines)
	beforeStart := 0

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

		for i := range pending {
			if pending[i].remaining > 0 {
				matches[pending[i].matchIdx].after = append(matches[pending[i].matchIdx].after, line)
				pending[i].remaining--
			}
		}
		nxt := pending[:0]
		for _, p := range pending {
			if p.remaining > 0 {
				nxt = append(nxt, p)
			}
		}
		pending = nxt

		if len(matches) < remaining && matchFn(line) {
			m := grepMatch{
				file:    path,
				lineNum: lineNum,
				line:    line,
			}
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

func grepFileCount(path string, matchFn func(string) bool, stopAfterFirst bool) (int, error) {
	scanner, closeFn, binary, err := helper.OpenTextScanner(path, 1024*1024)
	if err != nil {
		return 0, err
	}
	if closeFn != nil {
		defer closeFn()
	}
	if binary {
		return 0, errBinaryFile
	}

	count := 0
	for scanner.Scan() {
		if matchFn(scanner.Text()) {
			count++
			if stopAfterFirst {
				return count, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func grepFileMultiline(path string, re *regexp.Regexp, remaining int) ([]grepMatch, error) {
	f, _, _, binary, err := helper.SniffAndOpen(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if binary {
		return nil, errBinaryFile
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	content := helper.DecodeTextBytes(data).Text
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
		display := strings.ReplaceAll(matchedText, "\n", " \u21b5 ")
		const maxDisplay = 300
		if len(display) > maxDisplay {
			display = display[:maxDisplay] + "\u2026"
		}
		matches = append(matches, grepMatch{
			file:    path,
			lineNum: startLine,
			line:    display,
		})
	}
	return matches, nil
}

func grepFileMultilineCount(path string, re *regexp.Regexp, stopAfterFirst bool) (int, error) {
	f, _, _, binary, err := helper.SniffAndOpen(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if binary {
		return 0, errBinaryFile
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	text := helper.DecodeTextBytes(data).Text
	if stopAfterFirst {
		if re.FindStringIndex(text) != nil {
			return 1, nil
		}
		return 0, nil
	}
	return len(re.FindAllStringIndex(text, -1)), nil
}

func reserveMatchBudget(reserved *atomic.Int64, totalLimit, want int) int {
	if want <= 0 {
		return 0
	}
	for {
		current := reserved.Load()
		if current >= int64(totalLimit) {
			return 0
		}
		remaining := int64(totalLimit) - current
		take := int64(want)
		if take > remaining {
			take = remaining
		}
		if reserved.CompareAndSwap(current, current+take) {
			return int(take)
		}
	}
}

func releaseMatchBudget(reserved *atomic.Int64, reservedCount, usedCount int) {
	if reservedCount <= usedCount {
		return
	}
	reserved.Add(int64(usedCount - reservedCount))
}
