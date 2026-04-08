package multi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

func readMultipleFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawPaths := req.GetStringSlice("paths", nil)
	if len(rawPaths) == 0 {
		return mcp.NewToolResultError("paths must be a non-empty array of file paths"), nil
	}

	limitPerFile := helper.DefaultMaxReadBytes
	if mb := req.GetFloat("max_bytes_per_file", 0); mb > 0 {
		limitPerFile = int(mb)
	}

	var sb strings.Builder
	sep := strings.Repeat("\u2500", 60)

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
			sizeStr = helper.HumanizeBytes(fileSize)
		}

		text, readErr := helper.ReadOneFileAsText(p, limitPerFile)

		if sizeStr != "" && readErr == nil {
			lineCount := helper.CountContentLines(text)
			fmt.Fprintf(&sb, "--- File %d: %s (%s, %s) ---\n", i+1, p, sizeStr, helper.Pluralize(lineCount, "line"))
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

	fmt.Fprintf(&sb, "\n--- Summary: %s read", helper.Pluralize(successCount, "file"))
	if failCount > 0 {
		fmt.Fprintf(&sb, ", %s failed", helper.Pluralize(failCount, "file"))
	}
	if totalBytes > 0 {
		fmt.Fprintf(&sb, " (%s total)", helper.HumanizeBytes(totalBytes))
	}
	fmt.Fprintf(&sb, " ---\n")

	return mcp.NewToolResultText(sb.String()), nil
}

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
			if mkErr := helper.MkdirAllClear(filepath.Dir(path), 0o755); mkErr != nil {
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
		unlock := helper.LockFile(absPath)
		wErr := helper.AtomicWriteFile(path, []byte(content), 0o644)
		unlock()

		if wErr != nil {
			results = append(results, result{path: path, err: wErr})
			continue
		}

		diff := ""
		if showDiff {
			diff = helper.GenerateDiff(existingContent, content, 3)
		}
		results = append(results, result{path: path, size: len(content), diff: diff})
	}

	var sb strings.Builder
	successCount := 0
	for _, r := range results {
		if r.err == nil {
			successCount++
			fmt.Fprintf(&sb, "\u2713 %s (%s)\n", r.path, helper.HumanizeBytes(int64(r.size)))
			if showDiff && r.diff != "" {
				sb.WriteString(r.diff)
				if !strings.HasSuffix(r.diff, "\n") {
					sb.WriteByte('\n')
				}
			}
		} else {
			fmt.Fprintf(&sb, "\u2717 %s: %v\n", r.path, r.err)
		}
	}
	fmt.Fprintf(&sb, "\nWrote %s of %s.", helper.Pluralize(successCount, "file"), helper.Pluralize(len(results), "file"))

	if successCount < len(results) {
		return mcp.NewToolResultError(sb.String()), nil
	}
	return mcp.NewToolResultText(sb.String()), nil
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
	showDiff := req.GetBool("show_diff", true)
	showHidden := req.GetBool("show_hidden", false)

	if useRegex {
		if _, _, err := helper.ApplyEdit("test", oldStr, newStr, true, false); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	files, err := helper.CollectFiles(root, recursive, globPattern, showHidden)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	type fileResult struct {
		path    string
		count   int
		diff    string
		skipped bool
	}
	var changed []fileResult
	var unmodified []string
	var skipped []string
	totalCount := 0
	totalScanned := len(files)
	var errBuf strings.Builder

	for _, filePath := range files {
		count, diff, skip, werr := helper.ApplyReplaceToFile(filePath, oldStr, newStr, useRegex, dryRun, showDiff)
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
			oldStr, root, helper.Pluralize(totalScanned, "file"), errBuf.String())
		if len(skipped) > 0 {
			msg += fmt.Sprintf("\nSkipped %s (binary).", helper.Pluralize(len(skipped), "file"))
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
	for _, r := range changed {
		fmt.Fprintf(&sb, "  %s (%s)\n", r.path, helper.Pluralize(r.count, "occurrence"))
		if showDiff && r.diff != "" {
			sb.WriteString(r.diff)
			if !strings.HasSuffix(r.diff, "\n") {
				sb.WriteByte('\n')
			}
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(&sb, "\nSkipped %s (binary):\n", helper.Pluralize(len(skipped), "file"))
		for _, s := range skipped {
			fmt.Fprintf(&sb, "  %s\n", s)
		}
	}
	if len(unmodified) > 0 {
		fmt.Fprintf(&sb, "\nNo match in %s:\n", helper.Pluralize(len(unmodified), "file"))
		for _, u := range unmodified {
			fmt.Fprintf(&sb, "  %s\n", u)
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}
