package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const defaultMaxReadBytes = 10 * 1024 * 1024 // 10 MB

func registerFileTools(s ToolRegistrar) {
	s.AddTool(mcp.NewTool("read_file",
		mcp.WithDescription(td("read_file")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("read_file", "path"))),
		mcp.WithNumber("start_line", mcp.Description(pd("read_file", "start_line"))),
		mcp.WithNumber("end_line", mcp.Description(pd("read_file", "end_line"))),
		mcp.WithNumber("max_bytes", mcp.Description(pd("read_file", "max_bytes"))),
		mcp.WithNumber("head", mcp.Description(pd("read_file", "head"))),
		mcp.WithNumber("tail", mcp.Description(pd("read_file", "tail"))),
		mcp.WithBoolean("show_line_numbers", mcp.Description(pd("read_file", "show_line_numbers"))),
	), readFileHandler)

	s.AddTool(mcp.NewTool("write_file",
		mcp.WithDescription(td("write_file")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("write_file", "path"))),
		mcp.WithString("content", mcp.Required(), mcp.Description(pd("write_file", "content"))),
		mcp.WithBoolean("create_dirs", mcp.Description(pd("write_file", "create_dirs"))),
		mcp.WithBoolean("show_diff", mcp.Description(pd("write_file", "show_diff"))),
	), writeFileHandler)

	s.AddTool(mcp.NewTool("append_to_file",
		mcp.WithDescription(td("append_to_file")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("append_to_file", "path"))),
		mcp.WithString("content", mcp.Required(), mcp.Description(pd("append_to_file", "content"))),
	), appendFileHandler)

	s.AddTool(mcp.NewTool("edit_file",
		mcp.WithDescription(td("edit_file")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("edit_file", "path"))),
		mcp.WithString("old_str", mcp.Description(pd("edit_file", "old_str"))),
		mcp.WithString("new_str", mcp.Description(pd("edit_file", "new_str"))),
		mcp.WithBoolean("use_regex", mcp.Description(pd("edit_file", "use_regex"))),
		mcp.WithBoolean("replace_all", mcp.Description(pd("edit_file", "replace_all"))),
		mcp.WithBoolean("dry_run", mcp.Description(pd("edit_file", "dry_run"))),
		mcp.WithNumber("context_lines", mcp.Description(pd("edit_file", "context_lines"))),
		mcp.WithArray("edits",
			mcp.Description(pd("edit_file", "edits")),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"old_str":     map[string]any{"type": "string", "description": "Text (or regex) to find"},
					"new_str":     map[string]any{"type": "string", "description": "Replacement text"},
					"use_regex":   map[string]any{"type": "boolean", "description": "Treat old_str as Go regex. Default: false"},
					"replace_all": map[string]any{"type": "boolean", "description": "Replace all occurrences. Default: false"},
				},
				"required": []string{"old_str", "new_str"},
			}),
		),
	), editFileHandler)

	s.AddTool(mcp.NewTool("delete_file",
		mcp.WithDescription(td("delete_file")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("delete_file", "path"))),
	), deleteFileHandler)

	s.AddTool(mcp.NewTool("copy_file",
		mcp.WithDescription(td("copy_file")),
		mcp.WithString("source", mcp.Required(), mcp.Description(pd("copy_file", "source"))),
		mcp.WithString("destination", mcp.Required(), mcp.Description(pd("copy_file", "destination"))),
		mcp.WithBoolean("overwrite", mcp.Description(pd("copy_file", "overwrite"))),
	), copyFileHandler)

	s.AddTool(mcp.NewTool("move_file",
		mcp.WithDescription(td("move_file")),
		mcp.WithString("source", mcp.Required(), mcp.Description(pd("move_file", "source"))),
		mcp.WithString("destination", mcp.Required(), mcp.Description(pd("move_file", "destination"))),
		mcp.WithBoolean("overwrite", mcp.Description(pd("move_file", "overwrite"))),
	), moveFileHandler)

	s.AddTool(mcp.NewTool("get_file_info",
		mcp.WithDescription(td("get_file_info")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("get_file_info", "path"))),
	), getFileInfoHandler)

	s.AddTool(mcp.NewTool("path_exists",
		mcp.WithDescription(td("path_exists")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("path_exists", "path"))),
	), pathExistsHandler)

	s.AddTool(mcp.NewTool("diff_files",
		mcp.WithDescription(td("diff_files")),
		mcp.WithString("path_a", mcp.Required(), mcp.Description(pd("diff_files", "path_a"))),
		mcp.WithString("path_b", mcp.Required(), mcp.Description(pd("diff_files", "path_b"))),
		mcp.WithNumber("context_lines", mcp.Description(pd("diff_files", "context_lines"))),
	), diffFilesHandler)

	s.AddTool(mcp.NewTool("calculate_checksum",
		mcp.WithDescription(td("calculate_checksum")),
		mcp.WithArray("paths",
			mcp.Required(),
			mcp.Description(pd("calculate_checksum", "paths")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("algorithm",
			mcp.Description(pd("calculate_checksum", "algorithm")),
		),
	), calculateChecksumHandler)
}

// ── read_file ────────────────────────────────────────────────────────────────

func readFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	limitBytes := defaultMaxReadBytes
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

	f, info, contentType, binary, err := sniffAndOpen(path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer f.Close()

	if binary {
		text, err := readBinaryFile(f, info, contentType, limitBytes)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(text), nil
	}

	// head shortcut — count total lines for the informative header.
	if headN > 0 {
		total, countErr := countLines(f)
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

	// tail shortcut — requires a line count pass; total is reused in the header.
	if tailN > 0 {
		total, countErr := countLines(f)
		if countErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read error: %v", countErr)), nil
		}
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("seek error: %v", seekErr)), nil
		}
		startLine = max(1, total-int(tailN)+1)
		endLine = 0 // read to EOF
		return readFileLineRange(f, info, startLine, endLine, total, limitBytes, showLineNums)
	}

	if startLine > 0 || endLine > 0 {
		total, countErr := countLines(f)
		if countErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read error: %v", countErr)), nil
		}
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("seek error: %v", seekErr)), nil
		}
		return readFileLineRange(f, info, startLine, endLine, total, limitBytes, showLineNums)
	}

	// Full file read.
	if showLineNums {
		return readFileWithLineNumbers(f, info, limitBytes)
	}

	text, truncated, err := readFullText(f, info, limitBytes)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	// Only add the line-count header for non-truncated reads.
	// Truncated reads already embed total line count in the [TRUNCATED] notice.
	if !truncated {
		lineCount := countContentLines(text)
		header := fmt.Sprintf("[%s — %s]\n", info.Name(), pluralize(lineCount, "line"))
		return mcp.NewToolResultText(header + text), nil
	}
	return mcp.NewToolResultText(text), nil
}

// readFileWithLineNumbers reads the full file and prefixes each line with its number.
func readFileWithLineNumbers(f *os.File, info os.FileInfo, limit int) (*mcp.CallToolResult, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, limit)

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
		// Count remaining lines for the accurate total.
		totalLines := lineNum
		for scanner.Scan() {
			totalLines++
		}
		header := fmt.Sprintf("[%s — %s]\n", info.Name(), pluralize(totalLines, "line"))
		result = header + result + fmt.Sprintf(
			"\n[TRUNCATED — showing first %s of %s (%s total). Use start_line/end_line to read specific sections.]",
			humanizeBytes(int64(sb.Len())), humanizeBytes(info.Size()), pluralize(totalLines, "line"),
		)
		return mcp.NewToolResultText(result), nil
	}
	header := fmt.Sprintf("[%s — %s]\n", info.Name(), pluralize(lineNum, "line"))
	return mcp.NewToolResultText(header + result), nil
}

func readFileLineRange(f *os.File, info os.FileInfo, startLine, endLine, totalLines, limit int, showLineNums bool) (*mcp.CallToolResult, error) {
	if startLine < 1 {
		startLine = 1
	}

	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, limit)

	var sb strings.Builder
	lineNum := 0
	firstKept := -1
	lastKept := -1

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
		header = fmt.Sprintf("[%s — lines %d..%d of %s]\n", info.Name(), firstKept, lastKept, pluralize(totalLines, "line"))
	} else {
		header = fmt.Sprintf("[%s — lines %d..%d]\n", info.Name(), firstKept, lastKept)
	}
	return mcp.NewToolResultText(header + sb.String()), nil
}

// ── write_file ───────────────────────────────────────────────────────────────

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

	// Always read existing content to detect overwrite vs new-file creation,
	// and to produce a diff when overwriting.
	var existingContent string
	fileExisted := false
	if existing, readErr := os.ReadFile(path); readErr == nil {
		existingContent = string(existing)
		fileExisted = true
	}
	// Default show_diff to true when overwriting — changes are visible without
	// having to remember the flag. For new files the whole content would show
	// as additions, which is redundant, so we skip the diff by default.
	if !showDiffExplicit {
		showDiff = fileExisted
	}

	if createDirs {
		if err := mkdirAllClear(filepath.Dir(path), 0o755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
		}
	}

	absPath, _ := filepath.Abs(path)
	unlock := lockFile(absPath)
	defer unlock()

	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot write file: %v", err)), nil
	}

	lines := countContentLines(content)
	verb := "Created"
	if fileExisted {
		verb = "Overwrote"
	}
	msg := fmt.Sprintf("%s %s (%s) → %s", verb, humanizeBytes(int64(len(content))), pluralize(lines, "line"), path)

	if showDiff {
		diff := generateDiff(existingContent, content, 3)
		if diff != "" {
			msg += "\n\n" + diff
		}
	}

	return mcp.NewToolResultText(msg), nil
}

// ── append_to_file ───────────────────────────────────────────────────────────

func appendFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}
	content := req.GetString("content", "")
	if _, exists := req.GetArguments()["content"]; !exists {
		return mcp.NewToolResultError("content is required"), nil
	}

	if err := mkdirAllClear(filepath.Dir(path), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
	}

	absPath, _ := filepath.Abs(path)
	unlock := lockFile(absPath)
	defer unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot open file: %v", err)), nil
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write error: %v", err)), nil
	}

	msg := fmt.Sprintf("Appended %s (%s) to %s", humanizeBytes(int64(len(content))), pluralize(countContentLines(content), "line"), path)
	if newInfo, statErr := os.Stat(path); statErr == nil {
		msg += fmt.Sprintf(" (new size: %s)", humanizeBytes(newInfo.Size()))
	}
	return mcp.NewToolResultText(msg), nil
}

// ── edit_file ────────────────────────────────────────────────────────────────

// editSpec represents one find-replace operation.
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

	// Build the list of edits — batch mode takes precedence over single-edit params.
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
		// Single-edit mode (backward compatible).
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
	unlock := lockFile(absPath)
	defer unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot read file: %v", err)), nil
	}
	original, hasCRLF := normalizeCRLF(string(data))
	current := original
	totalCount := 0
	var missed []string
	var multipleMatchWarnings []string

	for i, spec := range specs {
		// For plain-text patterns without replace_all, detect multiple occurrences
		// before applying so we can warn about the ambiguity.
		if !spec.replaceAll && !spec.useRegex {
			if strings.Count(current, spec.oldStr) > 1 {
				multipleMatchWarnings = append(multipleMatchWarnings, spec.oldStr)
			}
		}

		modified, count, editErr := applyEdit(current, spec.oldStr, spec.newStr, spec.useRegex, spec.replaceAll)
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

	if totalCount == 0 {
		prefix := ""
		if dryRun {
			prefix = "[DRY RUN] "
		}
		msg := prefix + "Pattern(s) not found in file; no changes made"
		if len(specs) == 1 {
			msg = fmt.Sprintf("%sPattern not found in file: %q", prefix, specs[0].oldStr)
			if hint := findNearbyContext(original, specs[0].oldStr); hint != "" {
				msg += hint
			}
		}
		return mcp.NewToolResultText(msg), nil
	}

	if dryRun {
		diff := generateDiff(original, current, ctxLines)
		preview := fmt.Sprintf("[DRY RUN] %s would be replaced in %s — file not modified.",
			pluralize(totalCount, "occurrence"), path)
		if diff != "" {
			preview += "\n\n" + diff
		}
		return mcp.NewToolResultText(preview), nil
	}

	writeCurrent := current
	if hasCRLF {
		writeCurrent = restoreCRLF(current)
	}
	if err := atomicWriteFile(path, []byte(writeCurrent), 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot write file: %v", err)), nil
	}

	msg := fmt.Sprintf("Replaced %s in %s", pluralize(totalCount, "occurrence"), path)
	if len(specs) > 1 {
		msg += fmt.Sprintf(" (%s applied)", pluralize(len(specs)-len(missed), "edit"))
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
			"\nWarning: %d pattern(s) matched multiple times — only the first occurrence was replaced each time."+
				" Use replace_all=true to replace all, or add more context to old_str to make it unique.",
			len(multipleMatchWarnings),
		)
	}
	if diff := generateDiff(original, current, ctxLines); diff != "" {
		msg += "\n\n" + diff
	}
	return mcp.NewToolResultText(msg), nil
}

// ── delete_file ──────────────────────────────────────────────────────────────

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

	return mcp.NewToolResultText(fmt.Sprintf("Deleted file: %s (%s)", path, humanizeBytes(size))), nil
}

// ── copy_file ────────────────────────────────────────────────────────────────

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

	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("destination %q already exists; set overwrite=true to replace it", dst),
			), nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create destination directory: %v", err)), nil
	}

	n, err := copyFileDataN(src, dst, srcInfo.Mode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("copy error: %v", err)), nil
	}

	return mcp.NewToolResultText(
		fmt.Sprintf("Copied %s → %s (%s)", src, dst, humanizeBytes(n)),
	), nil
}

// ── move_file ────────────────────────────────────────────────────────────────

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

	if _, err := os.Stat(src); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat source: %v", err)), nil
	}

	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("destination %q already exists; set overwrite=true to replace it", dst),
			), nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create destination directory: %v", err)), nil
	}

	srcInfoForSize, _ := os.Stat(src)

	if err := os.Rename(src, dst); err == nil {
		sizeStr := ""
		if srcInfoForSize != nil && !srcInfoForSize.IsDir() {
			sizeStr = fmt.Sprintf(" (%s)", humanizeBytes(srcInfoForSize.Size()))
		}
		return mcp.NewToolResultText(fmt.Sprintf("Moved %s → %s%s", src, dst, sizeStr)), nil
	}

	// Cross-device move: copy then delete.
	srcInfo, err := os.Stat(src)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat source: %v", err)), nil
	}
	if srcInfo.IsDir() {
		return mcp.NewToolResultError(
			"cross-device directory move is not supported; copy the directory manually then delete the source",
		), nil
	}

	if err := copyFileData(src, dst, srcInfo.Mode()); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("copy error during cross-device move: %v", err)), nil
	}
	if err := os.Remove(src); err != nil {
		return mcp.NewToolResultError(
			fmt.Sprintf("file copied to %s but could not delete source %s: %v", dst, src, err),
		), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Moved %s → %s (%s, cross-device)", src, dst, humanizeBytes(srcInfo.Size()))), nil
}

// ── get_file_info ────────────────────────────────────────────────────────────

func getFileInfoHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

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
		humanizeBytes(linfo.Size()),
		linfo.Mode().String(),
		linfo.ModTime().Format("2006-01-02 15:04:05 MST"),
		abs,
	)

	if linfo.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			result += fmt.Sprintf("\nSymlink →    %s", target)
		}
	}

	// Add line count for text files — avoids a follow-up read_file call just to learn file length.
	// Use os.Stat (follows symlinks) so symlinks to text files also get a line count.
	if !linfo.IsDir() {
		if f, _, _, binary, sniffErr := sniffAndOpen(path); sniffErr == nil {
			if !binary {
				if n, countErr := countLines(f); countErr == nil {
					result += fmt.Sprintf("\nLines:       %s", pluralize(n, "line"))
				}
			}
			f.Close()
		}
	}

	return mcp.NewToolResultText(result), nil
}

// ── path_exists ───────────────────────────────────────────────────────────────

func pathExistsHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultText(fmt.Sprintf("false — %q does not exist", path)), nil
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
		fmt.Sprintf("true — %q is a %s (%s)", path, kind, humanizeBytes(info.Size())),
	), nil
}

// ── diff_files ────────────────────────────────────────────────────────────────

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

	readText := func(p string) (string, os.FileInfo, error) {
		f, info, _, binary, err := sniffAndOpen(p)
		if err != nil {
			return "", nil, err
		}
		defer f.Close()
		if binary {
			return "", info, fmt.Errorf("file is binary")
		}
		raw, err := io.ReadAll(f)
		if err != nil {
			return "", nil, fmt.Errorf("read error: %w", err)
		}
		raw = stripBOM(raw)
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

	diff := generateDiff(textA, textB, ctxLines)
	if diff == "" {
		return mcp.NewToolResultText(fmt.Sprintf(
			"Files are identical.\n  a: %s (%s, modified %s)\n  b: %s (%s, modified %s)",
			pathA, humanizeBytes(infoA.Size()), infoA.ModTime().Format("2006-01-02 15:04:05"),
			pathB, humanizeBytes(infoB.Size()), infoB.ModTime().Format("2006-01-02 15:04:05"),
		)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\t(%s, modified %s)\n", pathA, humanizeBytes(infoA.Size()), infoA.ModTime().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "+++ b/%s\t(%s, modified %s)\n", pathB, humanizeBytes(infoB.Size()), infoB.ModTime().Format("2006-01-02 15:04:05"))
	sb.WriteString(diff)
	return mcp.NewToolResultText(sb.String()), nil
}

// ── calculate_checksum ────────────────────────────────────────────────────────

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
		hash, size, err := hashFile(p, algorithm)
		if err != nil {
			fmt.Fprintf(&sb, "ERROR  %s — %v\n", p, err)
			continue
		}
		fmt.Fprintf(&sb, "%s  %s  (%s)\n", hash, p, humanizeBytes(size))
	}
	return mcp.NewToolResultText(sb.String()), nil
}
