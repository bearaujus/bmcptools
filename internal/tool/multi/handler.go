package multi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
	filetool "github.com/bearaujus/bmcptools/internal/tool/file"
)

const (
	defaultPathExistsBatchLimit      = 500
	defaultMultipleFileInfoPathLimit = 100
	defaultReadMultipleTotalBytes    = 512 * 1024
	defaultFindReplaceMaxFileSize    = 10 * 1024 * 1024
	defaultFindReplaceTotalDiffBytes = 256 * 1024
	defaultWriteDiffFileMaxBytes     = 2 * 1024 * 1024
)

type multiReadSpec struct {
	path    string
	options filetool.ReadOptions
}

type multiReadResult struct {
	path string
	size int64
	text string
	err  error
}

type writeEntry struct {
	index   int
	label   string
	path    string
	absPath string
	content string
}

type writeResult struct {
	path string
	size int
	diff string
	note string
	err  error
}

type infoResult struct {
	text          string
	fileCount     int
	dirCount      int
	symlinkCount  int
	errorCount    int
	skippedCounts int
}

type deleteBatchResult struct {
	path  string
	kind  string
	size  int64
	err   error
	valid bool
}

type transferEntry struct {
	index       int
	source      string
	destination string
	absSource   string
	absDest     string
	overwrite   bool
}

type copyBatchResult struct {
	source      string
	destination string
	stats       filetool.CopyStats
	err         error
	valid       bool
}

type moveBatchResult struct {
	source      string
	destination string
	stats       filetool.MoveStats
	err         error
	valid       bool
}

func writeLimitedPathList(sb *strings.Builder, paths []string, limit int) {
	if limit <= 0 || len(paths) <= limit {
		for _, p := range paths {
			fmt.Fprintf(sb, "  %s\n", p)
		}
		return
	}
	for _, p := range paths[:limit] {
		fmt.Fprintf(sb, "  %s\n", p)
	}
	fmt.Fprintf(sb, "  ... and %s more\n", helper.Pluralize(len(paths)-limit, "path"))
}

func batchLimit(req mcp.CallToolRequest, defaultLimit int) int {
	limit := int(req.GetFloat("limit", float64(defaultLimit)))
	if limit < 0 {
		return defaultLimit
	}
	return limit
}

func pathExistsBatchHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	paths := req.GetStringSlice("paths", nil)
	if len(paths) == 0 {
		return mcp.NewToolResultError("paths is required (array of file/directory paths)"), nil
	}

	limit := batchLimit(req, defaultPathExistsBatchLimit)
	total := len(paths)
	truncated := false
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
		truncated = true
	}

	var sb strings.Builder
	existsCount := 0
	missingCount := 0
	type pathResult struct {
		info os.FileInfo
		err  error
	}
	results := make([]pathResult, len(paths))
	helper.RunBoundedParallel(len(paths), func(i int) {
		info, err := os.Lstat(paths[i])
		results[i] = pathResult{info: info, err: err}
	})
	for i, p := range paths {
		if i > 0 {
			sb.WriteString("\n")
		}
		if results[i].err != nil {
			fmt.Fprintf(&sb, "%s: false", p)
			missingCount++
			continue
		}
		existsCount++
		info := results[i].info
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if meta, err := helper.ReadSymlinkMetadata(p); err == nil {
				fmt.Fprintf(&sb, "%s: symlink %s", p, helper.FormatSymlinkCompact(meta))
				continue
			}
			kind = "symlink"
		}
		fmt.Fprintf(&sb, "%s: %s (%s)", p, kind, helper.HumanizeBytes(info.Size()))
	}
	fmt.Fprintf(&sb, "\n\nSummary: checked %d of %d path(s); %d exist, %d missing/inaccessible.",
		len(paths), total, existsCount, missingCount)
	if truncated {
		fmt.Fprintf(&sb, " Output truncated after %d paths; increase limit or set limit=0 for all.", limit)
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func readMultipleFilesHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	specs, err := parseReadMultipleSpecs(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	totalMaxBytes := int(req.GetFloat("total_max_bytes", float64(defaultReadMultipleTotalBytes)))
	if totalMaxBytes < 0 {
		totalMaxBytes = defaultReadMultipleTotalBytes
	}

	var sb strings.Builder
	successCount := 0
	failCount := 0
	processedCount := 0
	omittedCount := 0
	sectionTruncated := false
	stoppedByContext := false
	var totalBytes int64
	renderedBytes := 0
	for i, spec := range specs {
		if totalMaxBytes > 0 && renderedBytes >= totalMaxBytes {
			omittedCount = len(specs) - i
			break
		}
		if ctx != nil && ctx.Err() != nil {
			omittedCount = len(specs) - i
			stoppedByContext = true
			break
		}
		out, readErr := filetool.ReadPathWithOptions(spec.path, spec.options)
		result := multiReadResult{
			path: spec.path,
			size: out.FileSize,
			text: out.Text,
			err:  readErr,
		}
		processedCount++
		if result.err != nil {
			failCount++
		} else {
			successCount++
			totalBytes += result.size
		}

		sectionText := formatMultiReadSection(result, i > 0)
		if totalMaxBytes > 0 && renderedBytes+len(sectionText) > totalMaxBytes {
			remaining := totalMaxBytes - renderedBytes
			if remaining < multiReadSectionHeaderBytes(sectionText) {
				omittedCount = len(specs) - i
				break
			}
			sb.WriteString(truncateUTF8Bytes(sectionText, remaining))
			renderedBytes = totalMaxBytes
			sectionTruncated = true
			omittedCount = len(specs) - i - 1
			break
		}
		sb.WriteString(sectionText)
		renderedBytes += len(sectionText)
	}

	if stoppedByContext {
		sb.WriteByte('\n')
		sb.WriteString("[Stopped before reading ")
		sb.WriteString(helper.Pluralize(omittedCount, "remaining file"))
		sb.WriteString(" because the request context ended. ")
		sb.WriteString(readMultipleBatchHint(specs))
		sb.WriteString("]\n\n")
	}
	if !stoppedByContext && (sectionTruncated || omittedCount > 0) {
		sb.WriteByte('\n')
		sb.WriteString("[Output capped at total_max_bytes=")
		sb.WriteString(helper.HumanizeBytes(int64(totalMaxBytes)))
		switch {
		case sectionTruncated && omittedCount > 0:
			sb.WriteString(" — partially rendered the current file section and omitted ")
			sb.WriteString(helper.Pluralize(omittedCount, "more file"))
		case sectionTruncated:
			sb.WriteString(" — partially rendered the final file section")
		default:
			sb.WriteString(" — ")
			sb.WriteString(helper.Pluralize(omittedCount, "file"))
			sb.WriteString(" omitted")
		}
		sb.WriteString(". ")
		sb.WriteString(readMultipleBatchHint(specs))
		sb.WriteString("]\n\n")
	}
	fmt.Fprintf(&sb, "\nSummary: processed %d of %d requested files; read %s", processedCount, len(specs), helper.Pluralize(successCount, "file"))
	if failCount > 0 {
		fmt.Fprintf(&sb, ", %s failed", helper.Pluralize(failCount, "file"))
	}
	if omittedCount > 0 && !stoppedByContext {
		fmt.Fprintf(&sb, ", %s omitted by total_max_bytes", helper.Pluralize(omittedCount, "file"))
	}
	if omittedCount > 0 && stoppedByContext {
		fmt.Fprintf(&sb, ", %s not started after context cancellation", helper.Pluralize(omittedCount, "file"))
	}
	if sectionTruncated {
		sb.WriteString(", final section truncated by total_max_bytes")
	}
	if totalBytes > 0 {
		fmt.Fprintf(&sb, " (%s total source data)", helper.HumanizeBytes(totalBytes))
	}
	sb.WriteByte('.')
	return mcp.NewToolResultText(sb.String()), nil
}

func formatMultiReadSection(result multiReadResult, includeLeadingNewline bool) string {
	var section strings.Builder
	if includeLeadingNewline {
		section.WriteByte('\n')
	}
	if result.size > 0 {
		fmt.Fprintf(&section, "--- %s (%s) ---\n", result.path, helper.HumanizeBytes(result.size))
	} else {
		fmt.Fprintf(&section, "--- %s ---\n", result.path)
	}
	if result.err != nil {
		fmt.Fprintf(&section, "[ERROR] %v\n", result.err)
		return section.String()
	}
	section.WriteString(result.text)
	if !strings.HasSuffix(result.text, "\n") {
		section.WriteByte('\n')
	}
	return section.String()
}

func multiReadSectionHeaderBytes(sectionText string) int {
	start := 0
	if strings.HasPrefix(sectionText, "\n") {
		start = 1
	}
	if idx := strings.IndexByte(sectionText[start:], '\n'); idx >= 0 {
		return start + idx + 1
	}
	return len(sectionText)
}

func readMultipleBatchHint(specs []multiReadSpec) string {
	if readMultipleNeedsNarrowing(specs) {
		return "Use grep_files first, or add ranges/head/tail, or split into smaller batches."
	}
	return "Raise total_max_bytes or split into smaller batches."
}

func readMultipleNeedsNarrowing(specs []multiReadSpec) bool {
	for _, spec := range specs {
		opts := spec.options
		if len(opts.Ranges) > 0 || opts.Head > 0 || opts.Tail > 0 || opts.StartLine > 0 || opts.EndLine > 0 {
			continue
		}
		return true
	}
	return false
}

func writeMultipleFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawFiles, _ := req.GetArguments()["files"].([]any)
	if len(rawFiles) == 0 {
		return mcp.NewToolResultError("files must be a non-empty array of {path, content} objects"), nil
	}

	createDirs := req.GetBool("create_dirs", true)
	showDiff := req.GetBool("show_diff", false)
	allOrNothing := req.GetBool("all_or_nothing", false)

	results := make([]writeResult, len(rawFiles))
	entries, validationErrors := parseWriteEntries(rawFiles, results)
	if allOrNothing && validationErrors > 0 {
		msg := formatWriteResults(results, len(rawFiles), true, false, false)
		return mcp.NewToolResultError(msg), nil
	}

	if allOrNothing {
		txResults, txErr := performTransactionalWrites(entries, createDirs, showDiff)
		for idx, result := range txResults {
			results[idx] = result
		}
		if txErr != nil {
			msg := formatWriteResults(results, len(rawFiles), true, false, txErr != nil)
			if !strings.Contains(msg, "No files were written") {
				msg += "\nNo files were written because all_or_nothing=true."
			}
			if txErr.Error() != "" {
				msg += "\n" + txErr.Error()
			}
			return mcp.NewToolResultError(msg), nil
		}
		msg := formatWriteResults(results, len(rawFiles), true, true, false)
		return mcp.NewToolResultText(msg), nil
	}

	helper.RunBoundedParallel(len(entries), func(i int) {
		entry := entries[i]
		results[entry.index] = writeOneFile(entry.path, entry.content, createDirs, showDiff)
	})

	msg := formatWriteResults(results, len(rawFiles), false, true, false)
	successCount := 0
	for _, result := range results {
		if result.err == nil && result.path != "" {
			successCount++
		}
	}
	if successCount < len(rawFiles) {
		return mcp.NewToolResultError(msg), nil
	}
	return mcp.NewToolResultText(msg), nil
}

func findReplaceInFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	root := req.GetString("path", "")
	if root == "" {
		return mcp.NewToolResultError("path is required"), nil
	}
	oldStr := req.GetString("old_str", "")
	if oldStr == "" {
		return mcp.NewToolResultError("old_str is required"), nil
	}
	newStr := req.GetString("new_str", "")
	useRegex := req.GetBool("use_regex", false)
	recursive := req.GetBool("recursive", true)
	globPattern := req.GetString("glob", "")
	dryRun := req.GetBool("dry_run", false)
	showDiff := req.GetBool("show_diff", false)
	showHidden := req.GetBool("show_hidden", false)
	excludePatterns := req.GetStringSlice("exclude_patterns", nil)
	showUnmodified := req.GetBool("show_unmodified", false)
	maxFileSize := int64(req.GetFloat("max_file_size", float64(defaultFindReplaceMaxFileSize)))
	if maxFileSize < 0 {
		maxFileSize = defaultFindReplaceMaxFileSize
	}
	maxTotalDiffBytes := int(req.GetFloat("max_total_diff_bytes", float64(defaultFindReplaceTotalDiffBytes)))
	if maxTotalDiffBytes < 0 {
		maxTotalDiffBytes = defaultFindReplaceTotalDiffBytes
	}

	if useRegex {
		if _, _, err := helper.ApplyEdit("test", oldStr, newStr, true, false); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	files, err := helper.CollectFileEntries(root, recursive, globPattern, showHidden, excludePatterns)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	type replaceResult struct {
		path      string
		count     int
		diff      string
		skipped   bool
		oversized bool
		err       error
	}
	results := make([]replaceResult, len(files))
	helper.RunIOBoundedParallelWithLimit(len(files), findReplaceWorkerLimit(maxFileSize), func(i int) {
		filePath := files[i].Path
		results[i].path = filePath
		if maxFileSize > 0 {
			if files[i].Info != nil && files[i].Info.Size() > maxFileSize {
				results[i].oversized = true
				return
			}
			if files[i].Info == nil {
				if info, statErr := os.Stat(filePath); statErr == nil && info.Size() > maxFileSize {
					results[i].oversized = true
					return
				}
			}
		}
		count, diff, skipped, replaceErr := helper.ApplyReplaceToFile(filePath, oldStr, newStr, useRegex, dryRun, showDiff)
		results[i].count = count
		results[i].diff = diff
		results[i].skipped = skipped
		results[i].err = replaceErr
	})

	type fileResult struct {
		path  string
		count int
		diff  string
	}
	var changed []fileResult
	var unmodified []string
	var skipped []string
	var oversized []string
	totalCount := 0
	totalScanned := len(files)
	var errBuf strings.Builder
	remainingDiffBytes := maxTotalDiffBytes
	diffBudgetExhausted := false
	diffFilesOmitted := 0
	for _, result := range results {
		switch {
		case result.oversized:
			oversized = append(oversized, result.path)
		case result.err != nil:
			fmt.Fprintf(&errBuf, "[ERROR] %s: %v\n", result.path, result.err)
		case result.skipped:
			skipped = append(skipped, result.path)
		case result.count > 0:
			changed = append(changed, fileResult{path: result.path, count: result.count, diff: result.diff})
			totalCount += result.count
		default:
			unmodified = append(unmodified, result.path)
		}
	}

	if len(changed) == 0 {
		msg := fmt.Sprintf("No matches for %q found under %s (scanned %s)\n%s",
			oldStr, root, helper.Pluralize(totalScanned, "file"), errBuf.String())
		if len(skipped) > 0 {
			msg += fmt.Sprintf("\nSkipped %s (binary).", helper.Pluralize(len(skipped), "file"))
		}
		if len(oversized) > 0 {
			msg += fmt.Sprintf("\nSkipped %s over max_file_size=%s.", helper.Pluralize(len(oversized), "file"), helper.HumanizeBytes(maxFileSize))
		}
		return mcp.NewToolResultText(msg), nil
	}

	action := "Replaced"
	if dryRun {
		action = "[DRY RUN] Would replace"
	}
	var sb strings.Builder
	sb.WriteString(errBuf.String())
	fmt.Fprintf(&sb, "%s %s across %s (scanned %s):\n\n",
		action,
		helper.Pluralize(totalCount, "occurrence"),
		helper.Pluralize(len(changed), "file"),
		helper.Pluralize(totalScanned, "file"),
	)
	for _, result := range changed {
		fmt.Fprintf(&sb, "  %s (%s)\n", result.path, helper.Pluralize(result.count, "occurrence"))
		if showDiff && result.diff != "" && !diffBudgetExhausted {
			if maxTotalDiffBytes > 0 && len(result.diff) > remainingDiffBytes {
				diffBudgetExhausted = true
				diffFilesOmitted++
				continue
			}
			sb.WriteString(result.diff)
			if !strings.HasSuffix(result.diff, "\n") {
				sb.WriteByte('\n')
			}
			if maxTotalDiffBytes > 0 {
				remainingDiffBytes -= len(result.diff)
			}
			continue
		}
		if showDiff && result.diff != "" && diffBudgetExhausted {
			diffFilesOmitted++
		}
	}
	if showDiff && diffBudgetExhausted {
		fmt.Fprintf(&sb, "\n[Diff output capped at max_total_diff_bytes=%s — omitted diffs for %s. Raise max_total_diff_bytes or rerun with a narrower scope.]\n",
			helper.HumanizeBytes(int64(maxTotalDiffBytes)),
			helper.Pluralize(diffFilesOmitted, "file"),
		)
	}
	if len(skipped) > 0 {
		fmt.Fprintf(&sb, "\nSkipped %s (binary):\n", helper.Pluralize(len(skipped), "file"))
		writeLimitedPathList(&sb, skipped, 20)
	}
	if len(oversized) > 0 {
		fmt.Fprintf(&sb, "\nSkipped %s over max_file_size=%s:\n", helper.Pluralize(len(oversized), "file"), helper.HumanizeBytes(maxFileSize))
		writeLimitedPathList(&sb, oversized, 20)
	}
	if len(unmodified) > 0 {
		if showUnmodified {
			fmt.Fprintf(&sb, "\nNo match in %s:\n", helper.Pluralize(len(unmodified), "file"))
			writeLimitedPathList(&sb, unmodified, 50)
		} else {
			fmt.Fprintf(&sb, "\nNo match in %s. Set show_unmodified=true to list them.", helper.Pluralize(len(unmodified), "file"))
		}
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func getMultipleFileInfoHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	paths := req.GetStringSlice("paths", nil)
	if len(paths) == 0 {
		return mcp.NewToolResultError("paths is required (array of file/directory paths)"), nil
	}

	limit := batchLimit(req, defaultMultipleFileInfoPathLimit)
	outputMode := req.GetString("output_mode", "compact")
	detailsMode := outputMode == "details"
	total := len(paths)
	truncated := false
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
		truncated = true
	}

	countLines := req.GetBool("count_lines", false)

	results := make([]infoResult, len(paths))
	helper.RunBoundedParallel(len(paths), func(i int) {
		results[i] = inspectPathInfo(paths[i], detailsMode, countLines)
	})

	var sb strings.Builder
	fileCount := 0
	dirCount := 0
	symlinkCount := 0
	errorCount := 0
	for i, result := range results {
		if i > 0 {
			if detailsMode {
				sb.WriteString("\n\n")
			} else {
				sb.WriteByte('\n')
			}
		}
		sb.WriteString(result.text)
		fileCount += result.fileCount
		dirCount += result.dirCount
		symlinkCount += result.symlinkCount
		errorCount += result.errorCount
	}
	fmt.Fprintf(&sb, "\n\nSummary: %d/%d shown; files=%d dirs=%d symlinks=%d errors=%d.",
		len(paths), total, fileCount, dirCount, symlinkCount, errorCount)
	if truncated {
		fmt.Fprintf(&sb, " Output truncated after %d paths; increase limit or set limit=0 for all.", limit)
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func deleteFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawPaths, ok := req.GetArguments()["paths"].([]any)
	if !ok || len(rawPaths) == 0 {
		return mcp.NewToolResultError("paths must be a non-empty array of file paths"), nil
	}

	results := make([]deleteBatchResult, len(rawPaths))
	paths := make([]string, 0, len(rawPaths))
	indexes := make([]int, 0, len(rawPaths))
	seen := make(map[string]int, len(rawPaths))
	for i, raw := range rawPaths {
		path, ok := raw.(string)
		if !ok || strings.TrimSpace(path) == "" {
			results[i] = deleteBatchResult{path: fmt.Sprintf("[entry %d]", i+1), err: fmt.Errorf("path is required"), valid: true}
			continue
		}
		absPath, _ := filepath.Abs(path)
		key := normalizedPathKey(absPath)
		if prev, exists := seen[key]; exists {
			results[i] = deleteBatchResult{path: path, err: fmt.Errorf("duplicates entry %d in the same batch", prev+1), valid: true}
			continue
		}
		seen[key] = i
		paths = append(paths, path)
		indexes = append(indexes, i)
	}

	helper.RunBoundedParallel(len(paths), func(i int) {
		stats, err := filetool.DeleteEntry(paths[i])
		results[indexes[i]] = deleteBatchResult{
			path:  paths[i],
			kind:  stats.Kind,
			size:  stats.Size,
			err:   err,
			valid: true,
		}
	})

	msg, hasErrors := formatDeleteBatchResults(results)
	if hasErrors {
		return mcp.NewToolResultError(msg), nil
	}
	return mcp.NewToolResultText(msg), nil
}

func copyPathsHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entries, results, _ := parseTransferEntries(req, "entries")
	if len(results) == 0 {
		return mcp.NewToolResultError("entries must be a non-empty array of {source, destination} objects"), nil
	}

	helper.RunBoundedParallel(len(entries), func(i int) {
		stats, err := filetool.CopyPath(entries[i].source, entries[i].destination, entries[i].overwrite)
		results[entries[i].index] = copyBatchResult{
			source:      entries[i].source,
			destination: entries[i].destination,
			stats:       stats,
			err:         err,
			valid:       true,
		}
	})

	msg, hasErrors := formatCopyBatchResults(results)
	if hasErrors {
		return mcp.NewToolResultError(msg), nil
	}
	return mcp.NewToolResultText(msg), nil
}

func movePathsHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entries, _, results := parseTransferEntries(req, "entries")
	if len(results) == 0 {
		return mcp.NewToolResultError("entries must be a non-empty array of {source, destination} objects"), nil
	}

	helper.RunBoundedParallel(len(entries), func(i int) {
		stats, err := filetool.MovePath(entries[i].source, entries[i].destination, entries[i].overwrite)
		results[entries[i].index] = moveBatchResult{
			source:      entries[i].source,
			destination: entries[i].destination,
			stats:       stats,
			err:         err,
			valid:       true,
		}
	})

	msg, hasErrors := formatMoveBatchResults(results)
	if hasErrors {
		return mcp.NewToolResultError(msg), nil
	}
	return mcp.NewToolResultText(msg), nil
}

func parseReadMultipleSpecs(req mcp.CallToolRequest) ([]multiReadSpec, error) {
	rawPaths, pathsProvided := req.GetArguments()["paths"].([]any)
	globPattern := strings.TrimSpace(req.GetString("glob", ""))
	if (!pathsProvided || len(rawPaths) == 0) && globPattern == "" {
		return nil, fmt.Errorf("provide a non-empty paths array or a glob pattern")
	}

	base := filetool.ReadOptions{
		MaxBytes:        helper.DefaultMaxReadBytes,
		IncludeBase64:   req.GetBool("include_base64", false),
		ShowLineNumbers: req.GetBool("show_line_numbers", false),
	}
	if mb := req.GetFloat("max_bytes_per_file", 0); mb > 0 {
		base.MaxBytes = int(mb)
	}
	if sl := req.GetFloat("start_line", 0); sl >= 1 {
		base.StartLine = int(sl)
	}
	if el := req.GetFloat("end_line", 0); el >= 1 {
		base.EndLine = int(el)
	}
	if head := req.GetFloat("head", 0); head >= 1 {
		base.Head = int(head)
	}
	if tail := req.GetFloat("tail", 0); tail >= 1 {
		base.Tail = int(tail)
	}
	if rawRanges, ok := req.GetArguments()["ranges"]; ok && rawRanges != nil {
		ranges, err := filetool.ParseReadLineRanges(rawRanges)
		if err != nil {
			return nil, err
		}
		base.Ranges = ranges
	}
	if err := filetool.ValidateReadOptions(base); err != nil {
		return nil, err
	}

	specs := make([]multiReadSpec, 0, len(rawPaths))
	for idx, raw := range rawPaths {
		spec, err := parseOneReadSpec(raw, idx, base)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	if globPattern != "" {
		root := req.GetString("root", "")
		if root == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("cannot get working directory: %w", err)
			}
			root = cwd
		}
		recursive := req.GetBool("recursive", true)
		showHidden := req.GetBool("show_hidden", false)
		excludePatterns := req.GetStringSlice("exclude_patterns", nil)
		entries, err := helper.CollectFileEntries(root, recursive, globPattern, showHidden, excludePatterns)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			specs = append(specs, multiReadSpec{
				path:    entry.Path,
				options: base,
			})
		}
	}

	if len(specs) == 0 {
		return nil, fmt.Errorf("no files matched the requested read set")
	}
	return specs, nil
}

func parseOneReadSpec(raw any, idx int, base filetool.ReadOptions) (multiReadSpec, error) {
	spec := multiReadSpec{options: base}
	switch value := raw.(type) {
	case string:
		spec.path = value
	case map[string]any:
		path, _ := value["path"].(string)
		spec.path = path
		applyOptionalReadInt(value, "start_line", &spec.options.StartLine)
		applyOptionalReadInt(value, "end_line", &spec.options.EndLine)
		applyOptionalReadInt(value, "head", &spec.options.Head)
		applyOptionalReadInt(value, "tail", &spec.options.Tail)
		if rawShow, ok := value["show_line_numbers"]; ok {
			if show, ok := rawShow.(bool); ok {
				spec.options.ShowLineNumbers = show
			}
		}
		if rawMax, ok := value["max_bytes"]; ok {
			if maxBytes, ok := numberToInt(rawMax); ok {
				if maxBytes > 0 {
					spec.options.MaxBytes = maxBytes
				} else {
					spec.options.MaxBytes = helper.DefaultMaxReadBytes
				}
			}
		}
		if rawRanges, ok := value["ranges"]; ok {
			ranges, err := filetool.ParseReadLineRanges(rawRanges)
			if err != nil {
				return multiReadSpec{}, fmt.Errorf("paths[%d]: %w", idx, err)
			}
			spec.options.Ranges = ranges
		}
	default:
		return multiReadSpec{}, fmt.Errorf("paths[%d]: expected a string path or {path, ...} object", idx)
	}

	if strings.TrimSpace(spec.path) == "" {
		return multiReadSpec{}, fmt.Errorf("paths[%d]: path is required", idx)
	}
	if err := filetool.ValidateReadOptions(spec.options); err != nil {
		return multiReadSpec{}, fmt.Errorf("paths[%d]: %w", idx, err)
	}
	return spec, nil
}

func parseTransferEntries(req mcp.CallToolRequest, key string) ([]transferEntry, []copyBatchResult, []moveBatchResult) {
	rawEntries, ok := req.GetArguments()[key].([]any)
	if !ok || len(rawEntries) == 0 {
		return nil, nil, nil
	}

	overwriteDefault := req.GetBool("overwrite", false)
	entries := make([]transferEntry, 0, len(rawEntries))
	copyResults := make([]copyBatchResult, len(rawEntries))
	moveResults := make([]moveBatchResult, len(rawEntries))
	for i, raw := range rawEntries {
		entryLabel := fmt.Sprintf("[entry %d]", i+1)
		item, ok := raw.(map[string]any)
		if !ok {
			err := fmt.Errorf("expected {source, destination} object")
			copyResults[i] = copyBatchResult{source: entryLabel, err: err, valid: true}
			moveResults[i] = moveBatchResult{source: entryLabel, err: err, valid: true}
			continue
		}
		source, _ := item["source"].(string)
		destination, _ := item["destination"].(string)
		if strings.TrimSpace(source) == "" || strings.TrimSpace(destination) == "" {
			err := fmt.Errorf("source and destination are required")
			copyResults[i] = copyBatchResult{source: entryLabel, err: err, valid: true}
			moveResults[i] = moveBatchResult{source: entryLabel, err: err, valid: true}
			continue
		}
		overwrite := overwriteDefault
		if rawOverwrite, ok := item["overwrite"].(bool); ok {
			overwrite = rawOverwrite
		}
		absSource, _ := filepath.Abs(source)
		absDestination, _ := filepath.Abs(destination)
		entries = append(entries, transferEntry{
			index:       i,
			source:      source,
			destination: destination,
			absSource:   absSource,
			absDest:     absDestination,
			overwrite:   overwrite,
		})
	}
	return validateTransferEntries(entries, copyResults, moveResults), copyResults, moveResults
}

func validateTransferEntries(entries []transferEntry, copyResults []copyBatchResult, moveResults []moveBatchResult) []transferEntry {
	conflicts := make(map[int]error)
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			switch {
			case pathsOverlap(entries[i].absDest, entries[j].absDest):
				conflicts[entries[i].index] = fmt.Errorf("destination overlaps with entry %d; overlapping batch destinations are unsupported", entries[j].index+1)
				conflicts[entries[j].index] = fmt.Errorf("destination overlaps with entry %d; overlapping batch destinations are unsupported", entries[i].index+1)
			case pathsOverlap(entries[i].absDest, entries[j].absSource):
				conflicts[entries[i].index] = fmt.Errorf("destination overlaps entry %d source; split this overlapping transfer batch into separate calls", entries[j].index+1)
				conflicts[entries[j].index] = fmt.Errorf("source overlaps entry %d destination; split this overlapping transfer batch into separate calls", entries[i].index+1)
			case pathsOverlap(entries[i].absSource, entries[j].absDest):
				conflicts[entries[i].index] = fmt.Errorf("source overlaps entry %d destination; split this overlapping transfer batch into separate calls", entries[j].index+1)
				conflicts[entries[j].index] = fmt.Errorf("destination overlaps entry %d source; split this overlapping transfer batch into separate calls", entries[i].index+1)
			}
		}
	}

	validated := make([]transferEntry, 0, len(entries))
	for _, entry := range entries {
		if err, conflicted := conflicts[entry.index]; conflicted {
			copyResults[entry.index] = copyBatchResult{
				source:      entry.source,
				destination: entry.destination,
				err:         err,
				valid:       true,
			}
			moveResults[entry.index] = moveBatchResult{
				source:      entry.source,
				destination: entry.destination,
				err:         err,
				valid:       true,
			}
			continue
		}
		validated = append(validated, entry)
	}
	return validated
}

func pathsOverlap(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		a = strings.ToLower(a)
		b = strings.ToLower(b)
	}
	return a == b ||
		strings.HasPrefix(a, b+string(filepath.Separator)) ||
		strings.HasPrefix(b, a+string(filepath.Separator))
}

func formatDeleteBatchResults(results []deleteBatchResult) (string, bool) {
	var sb strings.Builder
	successes := 0
	failures := 0
	for _, result := range results {
		if !result.valid {
			continue
		}
		if result.err != nil {
			failures++
			fmt.Fprintf(&sb, "✗ %s: %v\n", result.path, result.err)
			continue
		}
		successes++
		fmt.Fprintf(&sb, "✓ %s (%s, %s)\n", result.path, result.kind, helper.HumanizeBytes(result.size))
	}
	fmt.Fprintf(&sb, "\nDeleted %s of %s.", helper.Pluralize(successes, "entry"), helper.Pluralize(successes+failures, "entry"))
	return sb.String(), failures > 0
}

func formatCopyBatchResults(results []copyBatchResult) (string, bool) {
	var sb strings.Builder
	successes := 0
	failures := 0
	for _, result := range results {
		if !result.valid {
			continue
		}
		if result.err != nil {
			failures++
			fmt.Fprintf(&sb, "✗ %s → %s: %v\n", result.source, result.destination, result.err)
			continue
		}
		successes++
		fmt.Fprintf(&sb, "✓ %s → %s (%s)\n", result.source, result.destination, summarizeCopyStats(result.stats))
	}
	fmt.Fprintf(&sb, "\nCopied %s of %s.", helper.Pluralize(successes, "entry"), helper.Pluralize(successes+failures, "entry"))
	return sb.String(), failures > 0
}

func formatMoveBatchResults(results []moveBatchResult) (string, bool) {
	var sb strings.Builder
	successes := 0
	failures := 0
	for _, result := range results {
		if !result.valid {
			continue
		}
		if result.err != nil {
			failures++
			fmt.Fprintf(&sb, "✗ %s → %s: %v\n", result.source, result.destination, result.err)
			continue
		}
		successes++
		fmt.Fprintf(&sb, "✓ %s → %s (%s)\n", result.source, result.destination, summarizeMoveStats(result.stats))
	}
	fmt.Fprintf(&sb, "\nMoved %s of %s.", helper.Pluralize(successes, "entry"), helper.Pluralize(successes+failures, "entry"))
	return sb.String(), failures > 0
}

func summarizeCopyStats(stats filetool.CopyStats) string {
	if stats.SourceKind == "directory" {
		return fmt.Sprintf("%s, %s, %s",
			helper.Pluralize(stats.Files, "file"),
			helper.Pluralize(stats.Dirs, "directory"),
			helper.HumanizeBytes(stats.Bytes),
		)
	}
	return helper.HumanizeBytes(stats.Bytes)
}

func summarizeMoveStats(stats filetool.MoveStats) string {
	if stats.SourceKind == "directory" {
		summary := fmt.Sprintf("%s, %s", helper.Pluralize(stats.Files, "file"), helper.Pluralize(stats.Dirs, "directory"))
		if stats.Bytes > 0 {
			summary += ", " + helper.HumanizeBytes(stats.Bytes)
		}
		if stats.UsedFallback {
			summary += ", copied+deleted"
		}
		return summary
	}
	summary := helper.HumanizeBytes(stats.Bytes)
	if stats.UsedFallback {
		summary += ", copied+deleted"
	}
	return summary
}

func parseWriteEntries(rawFiles []any, results []writeResult) ([]writeEntry, int) {
	entries := make([]writeEntry, 0, len(rawFiles))
	seen := make(map[string]int, len(rawFiles))
	errors := 0
	for idx, raw := range rawFiles {
		label := fmt.Sprintf("[entry %d]", idx+1)
		fm, ok := raw.(map[string]any)
		if !ok {
			results[idx] = writeResult{path: label, err: fmt.Errorf("expected {path, content} object")}
			errors++
			continue
		}

		path, _ := fm["path"].(string)
		content, _ := fm["content"].(string)
		if strings.TrimSpace(path) == "" {
			results[idx] = writeResult{path: label, err: fmt.Errorf("path is required")}
			errors++
			continue
		}

		absPath, _ := filepath.Abs(path)
		key := normalizedPathKey(absPath)
		if prev, exists := seen[key]; exists {
			results[idx] = writeResult{path: path, err: fmt.Errorf("duplicates entry %d for the same destination", prev+1)}
			errors++
			continue
		}
		seen[key] = idx
		entries = append(entries, writeEntry{
			index:   idx,
			label:   label,
			path:    path,
			absPath: absPath,
			content: content,
		})
	}
	return entries, errors
}

func writeOneFile(path, content string, createDirs, showDiff bool) writeResult {
	if createDirs {
		if err := helper.MkdirAllClear(filepath.Dir(path), 0o755); err != nil {
			return writeResult{path: path, err: fmt.Errorf("cannot create parent directories: %w", err)}
		}
	}

	absPath, _ := filepath.Abs(path)
	unlock := helper.LockFile(absPath)
	defer unlock()

	writePerm := helper.ExistingFilePerm(path, 0o644)
	existingContent := ""
	diffNote := ""
	if showDiff {
		if info, err := os.Stat(path); err == nil {
			var readErr error
			existingContent, diffNote, readErr = helper.ReadExistingTextForDiff(path, info, defaultWriteDiffFileMaxBytes)
			if readErr != nil {
				return writeResult{path: path, err: fmt.Errorf("cannot read existing file to produce diff: %w", readErr)}
			}
		}
	}
	if err := helper.AtomicWriteFile(path, []byte(content), writePerm); err != nil {
		return writeResult{path: path, err: err}
	}

	diff := ""
	if showDiff && diffNote == "" {
		diff = helper.GenerateDiff(existingContent, content, 3)
	}
	return writeResult{path: path, size: len(content), diff: diff, note: diffNote}
}

type stagedWrite struct {
	entry      writeEntry
	diff       string
	note       string
	size       int
	tempPath   string
	backupPath string
	existed    bool
}

func performTransactionalWrites(entries []writeEntry, createDirs, showDiff bool) (map[int]writeResult, error) {
	results := make(map[int]writeResult, len(entries))
	if len(entries) == 0 {
		return results, fmt.Errorf("no valid write entries remain after validation")
	}

	ordered := make([]writeEntry, len(entries))
	copy(ordered, entries)
	sort.Slice(ordered, func(i, j int) bool {
		return normalizedPathKey(ordered[i].absPath) < normalizedPathKey(ordered[j].absPath)
	})

	unlocks := make([]func(), 0, len(ordered))
	for _, entry := range ordered {
		unlocks = append(unlocks, helper.LockFile(entry.absPath))
	}
	defer func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}()

	staged := make([]stagedWrite, 0, len(entries))
	for _, entry := range entries {
		if createDirs {
			if err := helper.MkdirAllClear(filepath.Dir(entry.path), 0o755); err != nil {
				cleanupStagedWrites(staged)
				return results, fmt.Errorf("%s: cannot create parent directories: %w", entry.path, err)
			}
		}

		_, statErr := os.Stat(entry.path)
		existed := statErr == nil
		writePerm := helper.ExistingFilePerm(entry.path, 0o644)
		existingContent := ""
		diffNote := ""
		if showDiff {
			if existed {
				info, err := os.Stat(entry.path)
				if err != nil {
					cleanupStagedWrites(staged)
					return results, fmt.Errorf("%s: cannot stat existing file for diff: %w", entry.path, err)
				}
				var readErr error
				existingContent, diffNote, readErr = helper.ReadExistingTextForDiff(entry.path, info, defaultWriteDiffFileMaxBytes)
				if readErr != nil {
					cleanupStagedWrites(staged)
					return results, fmt.Errorf("%s: cannot read existing file to produce diff: %w", entry.path, readErr)
				}
			}
		}

		tempPath, err := stageWrite(entry.path, []byte(entry.content), writePerm)
		if err != nil {
			cleanupStagedWrites(staged)
			return results, fmt.Errorf("%s: cannot stage write: %w", entry.path, err)
		}

		diff := ""
		if showDiff && diffNote == "" {
			diff = helper.GenerateDiff(existingContent, entry.content, 3)
		}
		staged = append(staged, stagedWrite{
			entry:    entry,
			diff:     diff,
			note:     diffNote,
			size:     len(entry.content),
			tempPath: tempPath,
			existed:  existed,
		})
	}

	for i := range staged {
		if !staged[i].existed {
			continue
		}
		backupPath, err := reserveTempPath(filepath.Dir(staged[i].entry.path), ".bmcptools-backup-*")
		if err != nil {
			restoreBackups(staged[:i])
			cleanupStagedWrites(staged)
			return results, fmt.Errorf("%s: cannot reserve rollback path: %w", staged[i].entry.path, err)
		}
		if err := os.Rename(staged[i].entry.path, backupPath); err != nil {
			restoreBackups(staged[:i])
			cleanupStagedWrites(staged)
			return results, fmt.Errorf("%s: cannot stage rollback backup: %w", staged[i].entry.path, err)
		}
		staged[i].backupPath = backupPath
	}

	committed := 0
	for i := range staged {
		if err := os.Rename(staged[i].tempPath, staged[i].entry.path); err != nil {
			rollbackErr := rollbackTransactionalWrites(staged, committed)
			if rollbackErr != nil {
				return results, fmt.Errorf("commit failed for %s: %v; rollback may be incomplete: %v", staged[i].entry.path, err, rollbackErr)
			}
			return results, fmt.Errorf("commit failed for %s: %w", staged[i].entry.path, err)
		}
		staged[i].tempPath = ""
		results[staged[i].entry.index] = writeResult{
			path: staged[i].entry.path,
			size: staged[i].size,
			diff: staged[i].diff,
			note: staged[i].note,
		}
		committed++
	}

	for _, stage := range staged {
		if stage.backupPath != "" {
			_ = os.Remove(stage.backupPath)
		}
	}
	return results, nil
}

func inspectPathInfo(path string, detailsMode, countLines bool) infoResult {
	linfo, err := os.Lstat(path)
	if err != nil {
		if detailsMode {
			return infoResult{text: fmt.Sprintf("Path:        %s\n[ERROR] %v", path, err), errorCount: 1}
		}
		return infoResult{text: fmt.Sprintf("%s: ERROR %v", path, err), errorCount: 1}
	}

	kind := "file"
	result := infoResult{}
	if linfo.IsDir() {
		kind = "directory"
		result.dirCount = 1
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		kind = "symlink"
		result.symlinkCount = 1
	} else if !linfo.IsDir() {
		result.fileCount = 1
	}

	lineInfo := ""
	if countLines && !linfo.IsDir() && linfo.Mode()&os.ModeSymlink == 0 {
		if n, counted, _, countErr := helper.CountTextFileLinesWithInfo(path, linfo, true); countErr == nil {
			if counted {
				lineInfo = helper.Pluralize(n, "line")
			}
		}
	}

	if linfo.Mode()&os.ModeSymlink != 0 {
		meta, metaErr := helper.ReadSymlinkMetadata(path)
		if detailsMode {
			abs, _ := filepath.Abs(path)
			text := fmt.Sprintf(
				"Path:        %s\nType:        %s\nMode:        %s\nModified:    %s\nAbsolute:    %s",
				path,
				kind,
				linfo.Mode().String(),
				linfo.ModTime().Format("2006-01-02 15:04:05 MST"),
				abs,
			)
			if metaErr == nil {
				text += fmt.Sprintf("\nSymlink →    %s", meta.Target)
				if meta.Dangling {
					text += "\nTarget:      dangling"
				} else {
					text += fmt.Sprintf("\nTarget:      %s", meta.TargetKind)
					text += fmt.Sprintf("\nTarget Size: %s", helper.HumanizeBytes(meta.TargetSize))
				}
			}
			result.text = text
			return result
		}
		if metaErr == nil {
			result.text = fmt.Sprintf("%s: symlink %s, mode %s, modified %s",
				path, helper.FormatSymlinkCompact(meta), linfo.Mode().String(), linfo.ModTime().Format("2006-01-02 15:04 MST"))
			return result
		}
	}

	if detailsMode {
		abs, _ := filepath.Abs(path)
		text := fmt.Sprintf(
			"Path:        %s\nType:        %s\nSize:        %s\nMode:        %s\nModified:    %s\nAbsolute:    %s",
			path,
			kind,
			helper.HumanizeBytes(linfo.Size()),
			linfo.Mode().String(),
			linfo.ModTime().Format("2006-01-02 15:04:05 MST"),
			abs,
		)
		if lineInfo != "" {
			text += fmt.Sprintf("\nLines:       %s", lineInfo)
		}
		result.text = text
		return result
	}

	text := fmt.Sprintf("%s: %s %s", path, kind, helper.HumanizeBytes(linfo.Size()))
	if lineInfo != "" {
		text += fmt.Sprintf(", %s", lineInfo)
	}
	text += fmt.Sprintf(", mode %s, modified %s", linfo.Mode().String(), linfo.ModTime().Format("2006-01-02 15:04 MST"))
	result.text = text
	return result
}

func applyOptionalReadInt(values map[string]any, key string, dst *int) {
	raw, ok := values[key]
	if !ok {
		return
	}
	if n, ok := numberToInt(raw); ok {
		if n > 0 {
			*dst = n
			return
		}
		*dst = 0
	}
}

func numberToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case int32:
		return int(n), true
	default:
		return 0, false
	}
}

func normalizedPathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func truncateUTF8Bytes(text string, maxBytes int) string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}
	for maxBytes > 0 && !utf8.ValidString(text[:maxBytes]) {
		maxBytes--
	}
	if maxBytes <= 0 {
		return ""
	}
	return text[:maxBytes]
}

func findReplaceWorkerLimit(maxFileSize int64) int {
	const targetWorkingSetBytes = 64 * 1024 * 1024
	ioLimit := runtime.NumCPU() * 2
	if ioLimit < 2 {
		ioLimit = 2
	}
	if ioLimit > 16 {
		ioLimit = 16
	}
	if maxFileSize <= 0 {
		return ioLimit
	}
	workers := int(targetWorkingSetBytes / maxFileSize)
	if workers < 1 {
		return 1
	}
	if workers > ioLimit {
		workers = ioLimit
	}
	if workers < 2 && maxFileSize <= 4*1024*1024 && ioLimit >= 2 {
		workers = 2
	}
	return workers
}

func stageWrite(path string, content []byte, perm os.FileMode) (string, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bmcptools-write-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	cleanup = false
	return tmpPath, nil
}

func reserveTempPath(dir, pattern string) (string, error) {
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return name, nil
}

func cleanupStagedWrites(staged []stagedWrite) {
	for _, stage := range staged {
		if stage.tempPath != "" {
			_ = os.Remove(stage.tempPath)
		}
	}
}

func restoreBackups(staged []stagedWrite) {
	for i := len(staged) - 1; i >= 0; i-- {
		if staged[i].backupPath != "" {
			_ = os.Rename(staged[i].backupPath, staged[i].entry.path)
		}
	}
}

func rollbackTransactionalWrites(staged []stagedWrite, committed int) error {
	var errs []string
	for i := committed - 1; i >= 0; i-- {
		if staged[i].backupPath != "" {
			if err := os.Remove(staged[i].entry.path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("%s: remove new file: %v", staged[i].entry.path, err))
			}
			if err := os.Rename(staged[i].backupPath, staged[i].entry.path); err != nil {
				errs = append(errs, fmt.Sprintf("%s: restore backup: %v", staged[i].entry.path, err))
			}
			continue
		}
		if err := os.Remove(staged[i].entry.path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("%s: remove new file: %v", staged[i].entry.path, err))
		}
	}
	for i := committed; i < len(staged); i++ {
		if staged[i].backupPath != "" {
			if err := os.Rename(staged[i].backupPath, staged[i].entry.path); err != nil {
				errs = append(errs, fmt.Sprintf("%s: restore backup: %v", staged[i].entry.path, err))
			}
		}
		if staged[i].tempPath != "" {
			_ = os.Remove(staged[i].tempPath)
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func formatWriteResults(results []writeResult, total int, allOrNothing, committed, partialFailure bool) string {
	var sb strings.Builder
	successCount := 0
	for _, result := range results {
		if result.path == "" && result.err == nil {
			continue
		}
		if result.err == nil {
			successCount++
			fmt.Fprintf(&sb, "✓ %s (%s)\n", result.path, helper.HumanizeBytes(int64(result.size)))
			if result.diff != "" {
				sb.WriteString(result.diff)
				if !strings.HasSuffix(result.diff, "\n") {
					sb.WriteByte('\n')
				}
			}
			if result.note != "" {
				sb.WriteString(result.note)
				if !strings.HasSuffix(result.note, "\n") {
					sb.WriteByte('\n')
				}
			}
		} else {
			fmt.Fprintf(&sb, "✗ %s: %v\n", result.path, result.err)
		}
	}

	switch {
	case allOrNothing && committed:
		fmt.Fprintf(&sb, "\nWrote %s of %s. all_or_nothing commit succeeded.", helper.Pluralize(successCount, "file"), helper.Pluralize(total, "file"))
	case allOrNothing:
		fmt.Fprintf(&sb, "\nWrote 0 files of %s. No files were written because all_or_nothing=true.", helper.Pluralize(total, "file"))
	case partialFailure || successCount < total:
		fmt.Fprintf(&sb, "\nWrote %s of %s. Partial write: successful files were already committed before the failure(s).", helper.Pluralize(successCount, "file"), helper.Pluralize(total, "file"))
	default:
		fmt.Fprintf(&sb, "\nWrote %s of %s.", helper.Pluralize(successCount, "file"), helper.Pluralize(total, "file"))
	}
	return sb.String()
}
