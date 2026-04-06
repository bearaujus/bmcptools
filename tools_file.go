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
	"github.com/mark3labs/mcp-go/server"
)

const defaultMaxReadBytes = 10 * 1024 * 1024 // 10 MB

func registerFileTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("read_file",
		mcp.WithDescription(
			"Read the contents of a file. "+
				"Auto-detects encoding; handles UTF-8/UTF-16 with or without BOM, "+
				"normalises invalid sequences, and strips BOMs. "+
				"Binary files are returned as base64-encoded data. "+
				"Use start_line / end_line to read a specific section of a large file. "+
				"File paths with Unicode characters (emoji, CJK, etc.) are fully supported.",
		),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute or relative path to the file")),
		mcp.WithNumber("start_line", mcp.Description("First line to read (1-indexed). Omit to start at the beginning.")),
		mcp.WithNumber("end_line", mcp.Description("Last line to read (inclusive). Omit to read to the end.")),
		mcp.WithNumber("max_bytes", mcp.Description(fmt.Sprintf("Maximum bytes to return (default %d = 10 MB).", defaultMaxReadBytes))),
		mcp.WithNumber("head", mcp.Description("If provided, returns only the first N lines of the file")),
		mcp.WithNumber("tail", mcp.Description("If provided, returns only the last N lines of the file")),
		mcp.WithBoolean("show_line_numbers", mcp.Description("Prefix every line with its line number. Default: false")),
	), readFileHandler)

	s.AddTool(mcp.NewTool("write_file",
		mcp.WithDescription(
			"Write content to a file, creating it if it does not exist or overwriting it if it does. "+
				"Parent directories are created automatically by default. "+
				"Content is written as UTF-8. File paths with Unicode characters are supported. "+
				"Set show_diff=true to see a unified diff of what changed when overwriting.",
		),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the file")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Content to write")),
		mcp.WithBoolean("create_dirs", mcp.Description("Create missing parent directories. Default: true")),
		mcp.WithBoolean("show_diff", mcp.Description("When overwriting an existing file, include a unified diff of the changes. Default: false. Highly recommended: set to true when editing existing files so you can verify exactly what changed.")),
	), writeFileHandler)

	s.AddTool(mcp.NewTool("append_to_file",
		mcp.WithDescription(
			"Append content to the end of a file. Creates the file (and any missing parent directories) if it does not exist. "+
				"Returns the new total file size after appending.",
		),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the file")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Content to append")),
	), appendFileHandler)

	s.AddTool(mcp.NewTool("edit_file",
		mcp.WithDescription(
			"Edit a file by finding and replacing text. Supports two modes:\n"+
				"1. Single edit: provide old_str and new_str directly.\n"+
				"2. Batch edits: provide an edits array for multiple changes in one call — "+
				"far more efficient than repeated single-edit calls.\n"+
				"Each edit supports use_regex (Go regex with $1,$2 back-references) and replace_all. "+
				"Returns a unified diff of all changes. "+
				"Use dry_run=true to preview changes without writing to disk.",
		),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the file")),
		mcp.WithString("old_str", mcp.Description("Text (or regex) to find — used for single-edit mode")),
		mcp.WithString("new_str", mcp.Description("Replacement text — used for single-edit mode")),
		mcp.WithBoolean("use_regex", mcp.Description("Treat old_str as a Go regular expression. Default: false")),
		mcp.WithBoolean("replace_all", mcp.Description("Replace every match instead of only the first. Default: false")),
		mcp.WithBoolean("dry_run", mcp.Description(
			"Preview changes without writing to disk. "+
				"Returns a unified diff of what would change. Default: false",
		)),
		mcp.WithNumber("context_lines", mcp.Description(
			"Number of unchanged lines to show around each changed region in the returned diff. Default: 3.",
		)),
		mcp.WithArray("edits",
			mcp.Description(
				"Batch mode: array of edit objects, each with {old_str, new_str, use_regex?, replace_all?}. "+
					"Applied sequentially. Prefer this over multiple single-edit calls.",
			),
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
		mcp.WithDescription("Delete a single file. Use delete_directory to remove directories."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the file")),
	), deleteFileHandler)

	s.AddTool(mcp.NewTool("copy_file",
		mcp.WithDescription(
			"Copy a file to a new location. "+
				"Parent directories of the destination are created automatically. "+
				"Set overwrite=true to replace an existing destination.",
		),
		mcp.WithString("source", mcp.Required(), mcp.Description("Source file path")),
		mcp.WithString("destination", mcp.Required(), mcp.Description("Destination file path")),
		mcp.WithBoolean("overwrite", mcp.Description("Overwrite the destination if it already exists. Default: false")),
	), copyFileHandler)

	s.AddTool(mcp.NewTool("move_file",
		mcp.WithDescription(
			"Move or rename a file or directory. "+
				"Works across directories on the same filesystem; for cross-device moves the file is "+
				"copied then the source is deleted. "+
				"Set overwrite=true to replace an existing destination (default is false — safe by default).",
		),
		mcp.WithString("source", mcp.Required(), mcp.Description("Source path")),
		mcp.WithString("destination", mcp.Required(), mcp.Description("Destination path")),
		mcp.WithBoolean("overwrite", mcp.Description("Overwrite the destination if it already exists. Default: false")),
	), moveFileHandler)

	s.AddTool(mcp.NewTool("get_file_info",
		mcp.WithDescription(
			"Return metadata for a file or directory: type, size, modification time, permissions, "+
				"and whether it is a symbolic link.",
		),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the file or directory")),
	), getFileInfoHandler)

	s.AddTool(mcp.NewTool("path_exists",
		mcp.WithDescription(
			"Quickly check whether a path exists and return its type (file/directory/symlink) without reading its contents. "+
				"Lightweight existence check — use this before read or write operations when you need "+
				"to branch on whether a path already exists. Returns a plain-English sentence: "+
				`"true — <path> is a file (N bytes)" or "false — <path> does not exist".`,
		),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to check")),
	), pathExistsHandler)
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
	showDiff := req.GetBool("show_diff", false)

	// Capture existing content before overwriting (for diff).
	var existingContent string
	if showDiff {
		if data, err := os.ReadFile(path); err == nil {
			existingContent = string(data)
		}
	}

	if createDirs {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
		}
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot write file: %v", err)), nil
	}

	lines := countContentLines(content)
	msg := fmt.Sprintf("Wrote %s (%s) → %s", humanizeBytes(int64(len(content))), pluralize(lines, "line"), path)

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

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot open file: %v", err)), nil
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write error: %v", err)), nil
	}

	msg := fmt.Sprintf("Appended %s to %s", humanizeBytes(int64(len(content))), path)
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

	data, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot read file: %v", err)), nil
	}
	original := string(data)
	current := original
	totalCount := 0
	var missed []string

	for i, spec := range specs {
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

	if err := os.WriteFile(path, []byte(current), 0o644); err != nil {
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

	if err := os.Remove(path); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot delete %q: %v", path, err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Deleted file: %s", path)), nil
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

	if err := os.Rename(src, dst); err == nil {
		return mcp.NewToolResultText(fmt.Sprintf("Moved %s → %s", src, dst)), nil
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

	return mcp.NewToolResultText(fmt.Sprintf("Moved %s → %s (cross-device)", src, dst)), nil
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
