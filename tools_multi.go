package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerMultiTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("read_multiple_files",
		mcp.WithDescription(td("read_multiple_files")),
		mcp.WithArray("paths",
			mcp.Required(),
			mcp.Description(pd("read_multiple_files", "paths")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_bytes_per_file",
			mcp.Description(pd("read_multiple_files", "max_bytes_per_file")),
		),
	), readMultipleFilesHandler)

	s.AddTool(mcp.NewTool("write_multiple_files",
		mcp.WithDescription(td("write_multiple_files")),
		mcp.WithArray("files",
			mcp.Required(),
			mcp.Description(pd("write_multiple_files", "files")),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "File path to write"},
					"content": map[string]any{"type": "string", "description": "Content to write"},
				},
				"required": []string{"path", "content"},
			}),
		),
		mcp.WithBoolean("create_dirs",
			mcp.Description(pd("write_multiple_files", "create_dirs")),
		),
		mcp.WithBoolean("show_diff",
			mcp.Description("When true, include a per-file unified diff of what changed for files that were overwritten. Default: false."),
		),
	), writeMultipleFilesHandler)

	s.AddTool(mcp.NewTool("find_replace_in_files",
		mcp.WithDescription(td("find_replace_in_files")),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description(pd("find_replace_in_files", "path")),
		),
		mcp.WithString("old_str",
			mcp.Required(),
			mcp.Description(pd("find_replace_in_files", "old_str")),
		),
		mcp.WithString("new_str",
			mcp.Required(),
			mcp.Description(pd("find_replace_in_files", "new_str")),
		),
		mcp.WithBoolean("use_regex",
			mcp.Description(pd("find_replace_in_files", "use_regex")),
		),
		mcp.WithBoolean("recursive",
			mcp.Description(pd("find_replace_in_files", "recursive")),
		),
		mcp.WithString("glob",
			mcp.Description(pd("find_replace_in_files", "glob")),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description(pd("find_replace_in_files", "dry_run")),
		),
		mcp.WithBoolean("show_diff",
			mcp.Description(pd("find_replace_in_files", "show_diff")),
		),
		mcp.WithBoolean("show_hidden",
			mcp.Description(pd("find_replace_in_files", "show_hidden")),
		),
	), findReplaceInFilesHandler)
}

// ── read_multiple_files ───────────────────────────────────────────────────────

func readMultipleFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawPaths := req.GetStringSlice("paths", nil)
	if len(rawPaths) == 0 {
		return mcp.NewToolResultError("paths must be a non-empty array of file paths"), nil
	}

	limitPerFile := defaultMaxReadBytes
	if mb := req.GetFloat("max_bytes_per_file", 0); mb > 0 {
		limitPerFile = int(mb)
	}

	var sb strings.Builder
	sep := strings.Repeat("─", 60)

	successCount := 0
	failCount := 0
	var totalBytes int64

	for i, p := range rawPaths {
		if i > 0 {
			sb.WriteString("\n")
		}

		var sizeStr string
		var fileSize int64
		if info, statErr := os.Stat(p); statErr == nil && !info.IsDir() {
			fileSize = info.Size()
			sizeStr = humanizeBytes(fileSize)
		}

		text, readErr := readOneFileAsText(p, limitPerFile)

		if sizeStr != "" && readErr == nil {
			lineCount := countContentLines(text)
			fmt.Fprintf(&sb, "--- File %d: %s (%s, %s) ---\n", i+1, p, sizeStr, pluralize(lineCount, "line"))
		} else if sizeStr != "" {
			fmt.Fprintf(&sb, "--- File %d: %s (%s) ---\n", i+1, p, sizeStr)
		} else {
			fmt.Fprintf(&sb, "--- File %d: %s ---\n", i+1, p)
		}

		if readErr != nil {
			failCount++
			fmt.Fprintf(&sb, "[ERROR] %v\n", readErr)
		} else {
			successCount++
			totalBytes += fileSize
			sb.WriteString(text)
		}
		sb.WriteString("\n")
		sb.WriteString(sep)
		sb.WriteString("\n")
	}

	// Trailing summary line.
	fmt.Fprintf(&sb, "\n--- Summary: %s read", pluralize(successCount, "file"))
	if failCount > 0 {
		fmt.Fprintf(&sb, ", %s failed", pluralize(failCount, "file"))
	}
	if totalBytes > 0 {
		fmt.Fprintf(&sb, " (%s total)", humanizeBytes(totalBytes))
	}
	fmt.Fprintf(&sb, " ---\n")

	return mcp.NewToolResultText(sb.String()), nil
}

// ── write_multiple_files ──────────────────────────────────────────────────────

func writeMultipleFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawFiles, _ := req.GetArguments()["files"].([]any)
	if len(rawFiles) == 0 {
		return mcp.NewToolResultError("files must be a non-empty array of {path, content} objects"), nil
	}

	createDirs := req.GetBool("create_dirs", true)
	showDiff := req.GetBool("show_diff", false)

	type result struct {
		path string
		size int
		diff string
		err  error
	}

	results := make([]result, 0, len(rawFiles))

	for idx, raw := range rawFiles {
		fm, ok := raw.(map[string]any)
		if !ok {
			results = append(results, result{
				path: fmt.Sprintf("[entry %d]", idx+1),
				err:  fmt.Errorf("expected {path, content} object"),
			})
			continue
		}

		path, _ := fm["path"].(string)
		content, _ := fm["content"].(string)

		if strings.TrimSpace(path) == "" {
			results = append(results, result{
				path: fmt.Sprintf("[entry %d]", idx+1),
				err:  fmt.Errorf("path is required"),
			})
			continue
		}

		if createDirs {
			if mkErr := mkdirAllClear(filepath.Dir(path), 0o755); mkErr != nil {
				results = append(results, result{path: path, err: fmt.Errorf("cannot create parent directories: %w", mkErr)})
				continue
			}
		}

		var existingContent string
		if showDiff {
			if data, readErr := os.ReadFile(path); readErr == nil {
				existingContent = string(data)
			}
		}

		absPath, _ := filepath.Abs(path)
		unlock := lockFile(absPath)
		wErr := atomicWriteFile(path, []byte(content), 0o644)
		unlock()

		if wErr != nil {
			results = append(results, result{path: path, err: wErr})
			continue
		}

		diff := ""
		if showDiff {
			diff = generateDiff(existingContent, content, 3)
		}
		results = append(results, result{path: path, size: len(content), diff: diff})
	}

	var sb strings.Builder
	successCount := 0
	for _, r := range results {
		if r.err == nil {
			successCount++
			fmt.Fprintf(&sb, "✓ %s (%s)\n", r.path, humanizeBytes(int64(r.size)))
			if showDiff && r.diff != "" {
				sb.WriteString(r.diff)
				if !strings.HasSuffix(r.diff, "\n") {
					sb.WriteByte('\n')
				}
			}
		} else {
			fmt.Fprintf(&sb, "✗ %s: %v\n", r.path, r.err)
		}
	}
	fmt.Fprintf(&sb, "\nWrote %s of %s.", pluralize(successCount, "file"), pluralize(len(results), "file"))

	if successCount < len(results) {
		return mcp.NewToolResultError(sb.String()), nil
	}
	return mcp.NewToolResultText(sb.String()), nil
}

// ── find_replace_in_files ─────────────────────────────────────────────────────

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
	showDiff := req.GetBool("show_diff", true)
	showHidden := req.GetBool("show_hidden", false)

	if useRegex {
		// Validate regex early.
		if _, _, err := applyEdit("test", oldStr, newStr, true, false); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	files, err := collectFiles(root, recursive, globPattern, showHidden)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	type fileResult struct {
		path    string
		count   int
		diff    string
		skipped bool // binary file
	}
	var changed []fileResult
	var unmodified []string
	var skipped []string
	totalCount := 0
	totalScanned := len(files)
	var errBuf strings.Builder

	for _, filePath := range files {
		count, diff, skip, werr := applyReplaceToFile(filePath, oldStr, newStr, useRegex, dryRun, showDiff)
		if werr != nil {
			fmt.Fprintf(&errBuf, "[ERROR] %s: %v\n", filePath, werr)
			continue
		}
		if skip {
			skipped = append(skipped, filePath)
			continue
		}
		if count > 0 {
			changed = append(changed, fileResult{filePath, count, diff, false})
			totalCount += count
		} else {
			unmodified = append(unmodified, filePath)
		}
	}

	if len(changed) == 0 {
		msg := fmt.Sprintf("No matches for %q found under %s (scanned %s)\n%s",
			oldStr, root, pluralize(totalScanned, "file"), errBuf.String())
		if len(skipped) > 0 {
			msg += fmt.Sprintf("\nSkipped %s (binary).", pluralize(len(skipped), "file"))
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
		pluralize(totalCount, "occurrence"),
		pluralize(len(changed), "file"),
		pluralize(totalScanned, "file"),
	)
	for _, r := range changed {
		fmt.Fprintf(&sb, "  %s (%s)\n", r.path, pluralize(r.count, "occurrence"))
		if showDiff && r.diff != "" {
			sb.WriteString(r.diff)
			if !strings.HasSuffix(r.diff, "\n") {
				sb.WriteByte('\n')
			}
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(&sb, "\nSkipped %s (binary):\n", pluralize(len(skipped), "file"))
		for _, s := range skipped {
			fmt.Fprintf(&sb, "  %s\n", s)
		}
	}
	if len(unmodified) > 0 {
		fmt.Fprintf(&sb, "\nNo match in %s:\n", pluralize(len(unmodified), "file"))
		for _, u := range unmodified {
			fmt.Fprintf(&sb, "  %s\n", u)
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}
