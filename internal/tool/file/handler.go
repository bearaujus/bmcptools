package file

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

const (
	defaultDiffMaxBytes     = 256 * 1024
	defaultDiffFileMaxBytes = 2 * 1024 * 1024
	scannerMaxTokenBytes    = 10 * 1024 * 1024
)

func readFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	limitBytes := helper.DefaultMaxReadBytes
	if mb := req.GetFloat("max_bytes", 0); mb > 0 {
		limitBytes = int(mb)
	}

	var startLine, endLine int
	if sl := req.GetFloat("start_line", 0); sl >= 1 {
		startLine = int(sl)
	}
	if el := req.GetFloat("end_line", 0); el >= 1 {
		endLine = int(el)
	}

	headN := req.GetFloat("head", 0)
	tailN := req.GetFloat("tail", 0)
	showLineNums := req.GetBool("show_line_numbers", false)

	f, info, contentType, binary, err := helper.SniffAndOpen(path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer f.Close()

	if binary {
		text, err := helper.ReadBinaryFile(f, info, contentType, limitBytes)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(text), nil
	}

	// Handle multi-range reads.
	if rawRanges, ok := req.GetArguments()["ranges"]; ok && rawRanges != nil {
		if rangeSlice, ok := rawRanges.([]any); ok && len(rangeSlice) > 0 {
			return readFileMultiRange(f, info, rangeSlice, limitBytes, showLineNums)
		}
	}

	if headN > 0 {
		total, countErr := helper.CountLines(f)
		if countErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read error: %v", countErr)), nil
		}
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("seek error: %v", seekErr)), nil
		}
		startLine = 1
		endLine = int(headN)
		return readFileLineRange(f, info, startLine, endLine, total, limitBytes, showLineNums)
	}

	if tailN > 0 {
		total, countErr := helper.CountLines(f)
		if countErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read error: %v", countErr)), nil
		}
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("seek error: %v", seekErr)), nil
		}
		startLine = max(1, total-int(tailN)+1)
		endLine = 0
		return readFileLineRange(f, info, startLine, endLine, total, limitBytes, showLineNums)
	}

	if startLine > 0 || endLine > 0 {
		total, countErr := helper.CountLines(f)
		if countErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read error: %v", countErr)), nil
		}
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("seek error: %v", seekErr)), nil
		}
		return readFileLineRange(f, info, startLine, endLine, total, limitBytes, showLineNums)
	}

	if showLineNums {
		return readFileWithLineNumbers(f, info, limitBytes)
	}

	text, truncated, err := helper.ReadFullText(f, info, limitBytes)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !truncated {
		lineCount := helper.CountContentLines(text)
		header := fmt.Sprintf("[%s \u2014 %s]\n", info.Name(), helper.Pluralize(lineCount, "line"))
		return mcp.NewToolResultText(header + text), nil
	}
	return mcp.NewToolResultText(text), nil
}

func readFileWithLineNumbers(f *os.File, info os.FileInfo, limit int) (*mcp.CallToolResult, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	var sb strings.Builder
	lineNum := 0
	truncated := false
	for scanner.Scan() {
		lineNum++
		fmt.Fprintf(&sb, "%6d|%s\n", lineNum, scanner.Text())
		if sb.Len() >= limit {
			truncated = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read error: %v", err)), nil
	}

	result := sb.String()
	if truncated {
		totalLines := lineNum
		for scanner.Scan() {
			totalLines++
		}
		header := fmt.Sprintf("[%s \u2014 %s]\n", info.Name(), helper.Pluralize(totalLines, "line"))
		result = header + result + fmt.Sprintf(
			"\n[TRUNCATED \u2014 showing first %s of %s (%s total). Use start_line/end_line to read specific sections.]",
			helper.HumanizeBytes(int64(sb.Len())), helper.HumanizeBytes(info.Size()), helper.Pluralize(totalLines, "line"),
		)
		return mcp.NewToolResultText(result), nil
	}
	header := fmt.Sprintf("[%s \u2014 %s]\n", info.Name(), helper.Pluralize(lineNum, "line"))
	return mcp.NewToolResultText(header + result), nil
}

func readFileLineRange(f *os.File, info os.FileInfo, startLine, endLine, totalLines, limit int, showLineNums bool) (*mcp.CallToolResult, error) {
	if startLine < 1 {
		startLine = 1
	}

	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	var sb strings.Builder
	lineNum := 0
	firstKept := -1
	lastKept := -1
	truncated := false

	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if endLine > 0 && lineNum > endLine {
			break
		}
		if firstKept == -1 {
			firstKept = lineNum
		}
		lastKept = lineNum
		if showLineNums {
			fmt.Fprintf(&sb, "%6d|%s\n", lineNum, scanner.Text())
		} else {
			sb.WriteString(scanner.Text())
			sb.WriteByte('\n')
		}
		if sb.Len() >= limit {
			truncated = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read error: %v", err)), nil
	}

	if firstKept == -1 {
		end := "EOF"
		if endLine > 0 {
			end = fmt.Sprintf("line %d", endLine)
		}
		return mcp.NewToolResultText(
			fmt.Sprintf("[%s] No lines found between line %d and %s", info.Name(), startLine, end),
		), nil
	}

	var header string
	if totalLines > 0 {
		header = fmt.Sprintf("[%s \u2014 lines %d..%d of %s]\n", info.Name(), firstKept, lastKept, helper.Pluralize(totalLines, "line"))
	} else {
		header = fmt.Sprintf("[%s \u2014 lines %d..%d]\n", info.Name(), firstKept, lastKept)
	}
	if truncated {
		sb.WriteString(fmt.Sprintf(
			"\n[TRUNCATED \u2014 range output reached max_bytes=%s. Use a smaller line range or raise max_bytes.]",
			helper.HumanizeBytes(int64(limit)),
		))
	}
	return mcp.NewToolResultText(header + sb.String()), nil
}

// readFileMultiRange reads multiple non-contiguous line ranges in a single pass.
func readFileMultiRange(f *os.File, info os.FileInfo, rawRanges []any, limit int, showLineNums bool) (*mcp.CallToolResult, error) {
	type lineRange struct {
		start, end int
	}
	var ranges []lineRange
	for _, r := range rawRanges {
		pair, ok := r.([]any)
		if !ok || len(pair) < 2 {
			return mcp.NewToolResultError("each range must be a [start_line, end_line] pair"), nil
		}
		s, ok1 := toInt(pair[0])
		e, ok2 := toInt(pair[1])
		if !ok1 || !ok2 || s < 1 {
			return mcp.NewToolResultError("range values must be positive integers"), nil
		}
		ranges = append(ranges, lineRange{start: s, end: e})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })

	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	var sb strings.Builder
	lineNum := 0
	rangeIdx := 0
	firstInRange := true
	truncated := false

	for scanner.Scan() {
		lineNum++
		if rangeIdx >= len(ranges) {
			break
		}
		rng := ranges[rangeIdx]
		if lineNum < rng.start {
			continue
		}
		if rng.end > 0 && lineNum > rng.end {
			rangeIdx++
			firstInRange = true
			if rangeIdx >= len(ranges) {
				break
			}
			rng = ranges[rangeIdx]
			if lineNum < rng.start {
				continue
			}
		}
		if firstInRange {
			if rangeIdx > 0 {
				sb.WriteString("\n")
			}
			endStr := "EOF"
			if rng.end > 0 {
				endStr = fmt.Sprintf("%d", rng.end)
			}
			fmt.Fprintf(&sb, "--- Lines %d–%s ---\n", rng.start, endStr)
			firstInRange = false
		}
		if showLineNums {
			fmt.Fprintf(&sb, "%6d|%s\n", lineNum, scanner.Text())
		} else {
			sb.WriteString(scanner.Text())
			sb.WriteByte('\n')
		}
		if sb.Len() >= limit {
			truncated = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read error: %v", err)), nil
	}
	if sb.Len() == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("[%s] No lines found in specified ranges", info.Name())), nil
	}
	if truncated {
		sb.WriteString(fmt.Sprintf(
			"\n[TRUNCATED \u2014 range output reached max_bytes=%s. Use fewer/smaller ranges or raise max_bytes.]",
			helper.HumanizeBytes(int64(limit)),
		))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

func requestByteLimit(req mcp.CallToolRequest, name string, defaultValue int) int {
	limit := int(req.GetFloat(name, float64(defaultValue)))
	if limit < 0 {
		return defaultValue
	}
	return limit
}

func scannerBufferLimit(outputLimit int) int {
	if outputLimit > scannerMaxTokenBytes {
		return outputLimit
	}
	return scannerMaxTokenBytes
}

func limitTextBytes(label, text string, maxBytes int) string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}
	return fmt.Sprintf("%s\n\n[%s truncated at %s. Increase the relevant max_*_bytes parameter or set it to 0 for unlimited.]",
		text[:maxBytes], label, helper.HumanizeBytes(int64(maxBytes)))
}

func writeFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}
	content := req.GetString("content", "")
	if content == "" {
		if _, exists := req.GetArguments()["content"]; !exists {
			return mcp.NewToolResultError("content is required"), nil
		}
	}

	createDirs := req.GetBool("create_dirs", true)
	_, showDiffExplicit := req.GetArguments()["show_diff"]
	showDiff := req.GetBool("show_diff", false)
	maxDiffBytes := requestByteLimit(req, "max_diff_bytes", defaultDiffMaxBytes)

	if createDirs {
		if err := helper.MkdirAllClear(filepath.Dir(path), 0o755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
		}
	}

	absPath, _ := filepath.Abs(path)
	unlock := helper.LockFile(absPath)
	defer unlock()

	writePerm := os.FileMode(0o644)
	var existingContent string
	fileExisted := false
	existingReadable := false
	if info, statErr := os.Stat(path); statErr == nil {
		fileExisted = true
		writePerm = info.Mode().Perm()
		if existing, readErr := os.ReadFile(path); readErr == nil {
			existingContent = string(existing)
			existingReadable = true
		}
	}
	if !showDiffExplicit {
		showDiff = fileExisted && existingReadable
	} else if showDiff && fileExisted && !existingReadable {
		return mcp.NewToolResultError("cannot read existing file to produce diff"), nil
	}

	if err := helper.AtomicWriteFile(path, []byte(content), writePerm); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot write file: %v", err)), nil
	}

	lines := helper.CountContentLines(content)
	verb := "Created"
	if fileExisted {
		verb = "Overwrote"
	}
	msg := fmt.Sprintf("%s %s (%s) \u2192 %s", verb, helper.HumanizeBytes(int64(len(content))), helper.Pluralize(lines, "line"), path)

	if showDiff {
		diff := limitTextBytes("Diff", helper.GenerateDiff(existingContent, content, 3), maxDiffBytes)
		if diff != "" {
			msg += "\n\n" + diff
		}
	}

	return mcp.NewToolResultText(msg), nil
}

func appendFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}
	content := req.GetString("content", "")
	if _, exists := req.GetArguments()["content"]; !exists {
		return mcp.NewToolResultError("content is required"), nil
	}

	if err := helper.MkdirAllClear(filepath.Dir(path), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
	}

	absPath, _ := filepath.Abs(path)
	unlock := helper.LockFile(absPath)
	defer unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot open file: %v", err)), nil
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write error: %v", err)), nil
	}

	msg := fmt.Sprintf("Appended %s (%s) to %s", helper.HumanizeBytes(int64(len(content))), helper.Pluralize(helper.CountContentLines(content), "line"), path)
	if newInfo, statErr := os.Stat(path); statErr == nil {
		msg += fmt.Sprintf(" (new size: %s)", helper.HumanizeBytes(newInfo.Size()))
	}
	return mcp.NewToolResultText(msg), nil
}

type editSpec struct {
	oldStr     string
	newStr     string
	useRegex   bool
	replaceAll bool
}

func editFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	var specs []editSpec

	if rawEdits, ok := req.GetArguments()["edits"]; ok {
		editsArr, ok := rawEdits.([]any)
		if !ok {
			return mcp.NewToolResultError("edits must be an array of {old_str, new_str, ...} objects"), nil
		}
		for i, raw := range editsArr {
			m, ok := raw.(map[string]any)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("edits[%d]: expected an object", i)), nil
			}
			oldStr, _ := m["old_str"].(string)
			if oldStr == "" {
				return mcp.NewToolResultError(fmt.Sprintf("edits[%d]: old_str is required", i)), nil
			}
			newStr, _ := m["new_str"].(string)
			useRegex, _ := m["use_regex"].(bool)
			replaceAll, _ := m["replace_all"].(bool)
			specs = append(specs, editSpec{oldStr, newStr, useRegex, replaceAll})
		}
	} else {
		oldStr := req.GetString("old_str", "")
		if oldStr == "" {
			return mcp.NewToolResultError("old_str is required (or use the edits array for batch mode)"), nil
		}
		newStr := req.GetString("new_str", "")
		useRegex := req.GetBool("use_regex", false)
		replaceAll := req.GetBool("replace_all", false)
		specs = []editSpec{{oldStr, newStr, useRegex, replaceAll}}
	}

	absPath, _ := filepath.Abs(path)
	unlock := helper.LockFile(absPath)
	defer unlock()

	writePerm := helper.ExistingFilePerm(path, 0o644)
	data, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot read file: %v", err)), nil
	}
	original, hasCRLF := helper.NormalizeCRLF(string(data))
	current := original
	totalCount := 0
	var missed []string
	var multipleMatchWarnings []string

	for i, spec := range specs {
		if !spec.replaceAll && !spec.useRegex {
			exactCount := strings.Count(current, spec.oldStr)
			if exactCount > 1 {
				multipleMatchWarnings = append(multipleMatchWarnings, spec.oldStr)
			} else if exactCount == 0 {
				if helper.CountNormalizedMatches(current, spec.oldStr) > 1 {
					multipleMatchWarnings = append(multipleMatchWarnings, spec.oldStr)
				}
			}
		}

		modified, count, editErr := helper.ApplyEdit(current, spec.oldStr, spec.newStr, spec.useRegex, spec.replaceAll)
		if editErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("edits[%d]: %v", i, editErr)), nil
		}
		if count == 0 {
			missed = append(missed, spec.oldStr)
		}
		totalCount += count
		current = modified
	}

	dryRun := req.GetBool("dry_run", false)

	ctxLines := 3
	if rawCL, ok := req.GetArguments()["context_lines"]; ok && rawCL != nil {
		if cl := req.GetFloat("context_lines", 3); cl >= 0 {
			ctxLines = int(cl)
			if ctxLines > 50 {
				ctxLines = 50
			}
		}
	}
	maxDiffBytes := requestByteLimit(req, "max_diff_bytes", defaultDiffMaxBytes)

	if totalCount == 0 {
		prefix := ""
		if dryRun {
			prefix = "[DRY RUN] "
		}
		msg := prefix + "Pattern(s) not found in file; no changes made"
		if len(specs) == 1 {
			msg = fmt.Sprintf("%sPattern not found in file: %q", prefix, specs[0].oldStr)
			if hint := helper.FindNearbyContext(original, specs[0].oldStr); hint != "" {
				msg += hint
			}
		}
		return mcp.NewToolResultText(msg), nil
	}

	if dryRun {
		diff := limitTextBytes("Diff", helper.GenerateDiff(original, current, ctxLines), maxDiffBytes)
		preview := fmt.Sprintf("[DRY RUN] %s would be replaced in %s \u2014 file not modified.",
			helper.Pluralize(totalCount, "occurrence"), path)
		if diff != "" {
			preview += "\n\n" + diff
		}
		return mcp.NewToolResultText(preview), nil
	}

	writeCurrent := current
	if hasCRLF {
		writeCurrent = helper.RestoreCRLF(current)
	}
	if err := helper.AtomicWriteFile(path, []byte(writeCurrent), writePerm); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot write file: %v", err)), nil
	}

	msg := fmt.Sprintf("Replaced %s in %s", helper.Pluralize(totalCount, "occurrence"), path)
	if len(specs) > 1 {
		msg += fmt.Sprintf(" (%s applied)", helper.Pluralize(len(specs)-len(missed), "edit"))
	}
	if len(missed) > 0 {
		msg += fmt.Sprintf("\nNot found (no change): %d pattern(s): ", len(missed))
		for i, m := range missed {
			if i > 0 {
				msg += ", "
			}
			msg += fmt.Sprintf("%q", m)
		}
	}
	if len(multipleMatchWarnings) > 0 {
		msg += fmt.Sprintf(
			"\nWarning: %d pattern(s) matched multiple times \u2014 only the first occurrence was replaced each time."+
				" Use replace_all=true to replace all, or add more context to old_str to make it unique.",
			len(multipleMatchWarnings),
		)
	}
	if diff := limitTextBytes("Diff", helper.GenerateDiff(original, current, ctxLines), maxDiffBytes); diff != "" {
		msg += "\n\n" + diff
	}
	return mcp.NewToolResultText(msg), nil
}

func deleteFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", path, err)), nil
	}
	if info.IsDir() {
		return mcp.NewToolResultError("path is a directory; use delete_directory instead"), nil
	}

	size := info.Size()
	if err := os.Remove(path); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot delete %q: %v", path, err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Deleted file: %s (%s)", path, helper.HumanizeBytes(size))), nil
}

func copyFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	src := req.GetString("source", "")
	if src == "" {
		return mcp.NewToolResultError("source is required"), nil
	}
	dst := req.GetString("destination", "")
	if dst == "" {
		return mcp.NewToolResultError("destination is required"), nil
	}
	overwrite := req.GetBool("overwrite", false)

	srcInfo, err := os.Stat(src)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat source: %v", err)), nil
	}
	if srcInfo.IsDir() {
		return mcp.NewToolResultError("source is a directory; copy_file only supports files"), nil
	}

	if dstInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return mcp.NewToolResultError("source and destination refer to the same file; refusing to copy"), nil
		}
		if dstInfo.IsDir() {
			return mcp.NewToolResultError("destination is a directory; copy_file requires a file path"), nil
		}
		if !overwrite {
			return mcp.NewToolResultError(
				fmt.Sprintf("destination %q already exists; set overwrite=true to replace it", dst),
			), nil
		}
	} else if !os.IsNotExist(err) {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat destination: %v", err)), nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create destination directory: %v", err)), nil
	}

	n, err := helper.CopyFileDataN(src, dst, srcInfo.Mode().Perm())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("copy error: %v", err)), nil
	}

	return mcp.NewToolResultText(
		fmt.Sprintf("Copied %s \u2192 %s (%s)", src, dst, helper.HumanizeBytes(n)),
	), nil
}

func moveFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	src := req.GetString("source", "")
	if src == "" {
		return mcp.NewToolResultError("source is required"), nil
	}
	dst := req.GetString("destination", "")
	if dst == "" {
		return mcp.NewToolResultError("destination is required"), nil
	}
	overwrite := req.GetBool("overwrite", false)

	srcInfo, err := os.Stat(src)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat source: %v", err)), nil
	}

	if dstInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return mcp.NewToolResultError("source and destination refer to the same file; no move performed"), nil
		}
		if !overwrite {
			return mcp.NewToolResultError(
				fmt.Sprintf("destination %q already exists; set overwrite=true to replace it", dst),
			), nil
		}
	} else if !os.IsNotExist(err) {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat destination: %v", err)), nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create destination directory: %v", err)), nil
	}

	srcInfoForSize, _ := os.Stat(src)

	if err := os.Rename(src, dst); err == nil {
		sizeStr := ""
		if srcInfoForSize != nil && !srcInfoForSize.IsDir() {
			sizeStr = fmt.Sprintf(" (%s)", helper.HumanizeBytes(srcInfoForSize.Size()))
		}
		return mcp.NewToolResultText(fmt.Sprintf("Moved %s \u2192 %s%s", src, dst, sizeStr)), nil
	}

	srcInfo, err = os.Stat(src)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat source: %v", err)), nil
	}
	if srcInfo.IsDir() {
		return mcp.NewToolResultError(
			"cross-device directory move is not supported; copy the directory manually then delete the source",
		), nil
	}

	if err := helper.CopyFileData(src, dst, srcInfo.Mode().Perm()); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("copy error during cross-device move: %v", err)), nil
	}
	if err := os.Remove(src); err != nil {
		return mcp.NewToolResultError(
			fmt.Sprintf("file copied to %s but could not delete source %s: %v", dst, src, err),
		), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Moved %s \u2192 %s (%s, cross-device)", src, dst, helper.HumanizeBytes(srcInfo.Size()))), nil
}

func getFileInfoHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}
	outputMode := req.GetString("output_mode", "compact")

	linfo, err := os.Lstat(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", path, err)), nil
	}

	kind := "file"
	if linfo.IsDir() {
		kind = "directory"
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		kind = "symlink"
	}

	lineInfo := ""
	if !linfo.IsDir() {
		if f, _, _, binary, sniffErr := helper.SniffAndOpen(path); sniffErr == nil {
			if !binary {
				if n, countErr := helper.CountLines(f); countErr == nil {
					lineInfo = helper.Pluralize(n, "line")
				}
			}
			f.Close()
		}
	}

	if outputMode != "details" {
		result := fmt.Sprintf("%s: %s %s", path, kind, helper.HumanizeBytes(linfo.Size()))
		if lineInfo != "" {
			result += fmt.Sprintf(", %s", lineInfo)
		}
		result += fmt.Sprintf(", mode %s, modified %s", linfo.Mode().String(), linfo.ModTime().Format("2006-01-02 15:04 MST"))
		if linfo.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(path); err == nil {
				result += fmt.Sprintf(", -> %s", target)
			}
		}
		return mcp.NewToolResultText(result), nil
	}

	abs, _ := filepath.Abs(path)
	result := fmt.Sprintf(
		"Path:        %s\n"+
			"Type:        %s\n"+
			"Size:        %s\n"+
			"Mode:        %s\n"+
			"Modified:    %s\n"+
			"Absolute:    %s",
		path,
		kind,
		helper.HumanizeBytes(linfo.Size()),
		linfo.Mode().String(),
		linfo.ModTime().Format("2006-01-02 15:04:05 MST"),
		abs,
	)

	if linfo.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			result += fmt.Sprintf("\nSymlink \u2192    %s", target)
		}
	}

	if lineInfo != "" {
		result += fmt.Sprintf("\nLines:       %s", lineInfo)
	}

	return mcp.NewToolResultText(result), nil
}

func pathExistsHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultText(fmt.Sprintf("false \u2014 %q does not exist", path)), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", path, err)), nil
	}

	kind := "file"
	if info.IsDir() {
		kind = "directory"
	} else if info.Mode()&os.ModeSymlink != 0 {
		kind = "symlink"
	}
	return mcp.NewToolResultText(
		fmt.Sprintf("true \u2014 %q is a %s (%s)", path, kind, helper.HumanizeBytes(info.Size())),
	), nil
}

func diffFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathA := req.GetString("path_a", "")
	pathB := req.GetString("path_b", "")
	if pathA == "" || pathB == "" {
		return mcp.NewToolResultError("path_a and path_b are required"), nil
	}

	ctxLines := int(req.GetFloat("context_lines", 3))
	if ctxLines < 0 {
		ctxLines = 0
	}
	maxFileBytes := requestByteLimit(req, "max_file_bytes", defaultDiffFileMaxBytes)
	maxDiffBytes := requestByteLimit(req, "max_diff_bytes", defaultDiffMaxBytes)

	readText := func(p string) (string, os.FileInfo, error) {
		f, info, _, binary, err := helper.SniffAndOpen(p)
		if err != nil {
			return "", nil, err
		}
		defer f.Close()
		if binary {
			return "", info, fmt.Errorf("file is binary")
		}
		if maxFileBytes > 0 && info.Size() > int64(maxFileBytes) {
			return "", info, fmt.Errorf("file is %s, which exceeds max_file_bytes=%s; raise max_file_bytes or set it to 0 for unlimited",
				helper.HumanizeBytes(info.Size()), helper.HumanizeBytes(int64(maxFileBytes)))
		}
		raw, err := io.ReadAll(f)
		if err != nil {
			return "", nil, fmt.Errorf("read error: %w", err)
		}
		raw = helper.StripBOM(raw)
		return strings.ToValidUTF8(string(raw), "\uFFFD"), info, nil
	}

	textA, infoA, errA := readText(pathA)
	if errA != nil {
		return mcp.NewToolResultError(fmt.Sprintf("path_a: %v", errA)), nil
	}
	textB, infoB, errB := readText(pathB)
	if errB != nil {
		return mcp.NewToolResultError(fmt.Sprintf("path_b: %v", errB)), nil
	}

	diff := limitTextBytes("Diff", helper.GenerateDiff(textA, textB, ctxLines), maxDiffBytes)
	if diff == "" {
		return mcp.NewToolResultText(fmt.Sprintf(
			"Files are identical.\n  a: %s (%s, modified %s)\n  b: %s (%s, modified %s)",
			pathA, helper.HumanizeBytes(infoA.Size()), infoA.ModTime().Format("2006-01-02 15:04:05"),
			pathB, helper.HumanizeBytes(infoB.Size()), infoB.ModTime().Format("2006-01-02 15:04:05"),
		)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\t(%s, modified %s)\n", pathA, helper.HumanizeBytes(infoA.Size()), infoA.ModTime().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "+++ b/%s\t(%s, modified %s)\n", pathB, helper.HumanizeBytes(infoB.Size()), infoB.ModTime().Format("2006-01-02 15:04:05"))
	sb.WriteString(diff)
	return mcp.NewToolResultText(sb.String()), nil
}

func calculateChecksumHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	paths := req.GetStringSlice("paths", nil)
	if len(paths) == 0 {
		return mcp.NewToolResultError("paths is required"), nil
	}

	algorithm := strings.ToLower(req.GetString("algorithm", "sha256"))
	switch algorithm {
	case "md5", "sha1", "sha256":
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unsupported algorithm %q; use md5, sha1, or sha256", algorithm)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Algorithm: %s\n\n", strings.ToUpper(algorithm))

	for _, p := range paths {
		hash, size, err := helper.HashFile(p, algorithm)
		if err != nil {
			fmt.Fprintf(&sb, "ERROR  %s \u2014 %v\n", p, err)
			continue
		}
		fmt.Fprintf(&sb, "%s  %s  (%s)\n", hash, p, helper.HumanizeBytes(size))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func createSymlinkHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	source := req.GetString("source", "")
	if source == "" {
		return mcp.NewToolResultError("source is required"), nil
	}
	link := req.GetString("link", "")
	if link == "" {
		return mcp.NewToolResultError("link is required"), nil
	}
	if err := os.Symlink(source, link); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create symlink: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Created symlink: %s → %s", link, source)), nil
}

func detectArchiveFormat(path, explicit string) string {
	if explicit != "" {
		return strings.ToLower(explicit)
	}
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return ""
	}
}

func compressFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	paths := req.GetStringSlice("paths", nil)
	if len(paths) == 0 {
		return mcp.NewToolResultError("paths is required"), nil
	}
	output := req.GetString("output", "")
	if output == "" {
		return mcp.NewToolResultError("output is required"), nil
	}

	format := detectArchiveFormat(output, req.GetString("format", ""))
	if format == "" {
		return mcp.NewToolResultError("cannot detect archive format from output path; specify format as \"zip\" or \"tar.gz\""), nil
	}
	if format != "zip" && format != "tar.gz" {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported format %q; use \"zip\" or \"tar.gz\"", format)), nil
	}

	if dir := filepath.Dir(output); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create output directory: %v", err)), nil
		}
	}

	var count int
	var err error
	switch format {
	case "zip":
		count, err = compressZip(paths, output)
	case "tar.gz":
		count, err = compressTarGz(paths, output)
	}
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("compression failed: %v", err)), nil
	}

	info, statErr := os.Stat(output)
	sizeStr := "unknown"
	if statErr == nil {
		sizeStr = helper.HumanizeBytes(info.Size())
	}
	return mcp.NewToolResultText(fmt.Sprintf("Compressed %d files into %s (%s, %s)", count, output, format, sizeStr)), nil
}

func compressZip(sources []string, output string) (int, error) {
	f, err := os.Create(output)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	var count int
	for _, src := range sources {
		src = filepath.Clean(src)
		base := filepath.Dir(src)
		err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = relPath
			header.Method = zip.Deflate

			writer, err := w.CreateHeader(header)
			if err != nil {
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(writer, file); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func compressTarGz(sources []string, output string) (int, error) {
	f, err := os.Create(output)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	var count int
	for _, src := range sources {
		src = filepath.Clean(src)
		base := filepath.Dir(src)
		err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)

			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func extractArchiveHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	archive := req.GetString("archive", "")
	if archive == "" {
		return mcp.NewToolResultError("archive is required"), nil
	}

	output := req.GetString("output", "")
	if output == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get working directory: %v", err)), nil
		}
		output = cwd
	}

	format := detectArchiveFormat(archive, req.GetString("format", ""))
	if format == "" {
		return mcp.NewToolResultError("cannot detect archive format from file extension; specify format as \"zip\" or \"tar.gz\""), nil
	}
	if format != "zip" && format != "tar.gz" {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported format %q; use \"zip\" or \"tar.gz\"", format)), nil
	}

	if err := os.MkdirAll(output, 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create output directory: %v", err)), nil
	}

	var count int
	var err error
	switch format {
	case "zip":
		count, err = extractZip(archive, output)
	case "tar.gz":
		count, err = extractTarGz(archive, output)
	}
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("extraction failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Extracted %d files to %s", count, output)), nil
}

// safeJoin validates that name does not escape the destination directory.
func safeJoin(dest, name string) (string, error) {
	target := filepath.Join(dest, filepath.FromSlash(name))
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: %w", name, err)
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", fmt.Errorf("invalid destination: %w", err)
	}
	if !strings.HasPrefix(absTarget, absDest+string(filepath.Separator)) && absTarget != absDest {
		return "", fmt.Errorf("path %q escapes output directory", name)
	}
	return absTarget, nil
}

func extractZip(archive, dest string) (int, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	var count int
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			target, err := safeJoin(dest, f.Name)
			if err != nil {
				return count, err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return count, err
			}
			continue
		}

		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return count, err
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return count, err
		}

		rc, err := f.Open()
		if err != nil {
			return count, err
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return count, err
		}

		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return count, err
		}
		out.Close()
		rc.Close()
		count++
	}
	return count, nil
}

func extractTarGz(archive, dest string) (int, error) {
	f, err := os.Open(archive)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var count int
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}

		target, err := safeJoin(dest, header.Name)
		if err != nil {
			return count, err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return count, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return count, err
			}
			out, err := os.Create(target)
			if err != nil {
				return count, err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return count, err
			}
			out.Close()
			count++
		}
	}
	return count, nil
}
