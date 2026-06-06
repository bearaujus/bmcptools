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

const (
	defaultPathExistsBatchLimit      = 500
	defaultMultipleFileInfoPathLimit = 100
	defaultFindReplaceMaxFileSize    = 10 * 1024 * 1024
)

func legacyReadMultipleFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawPaths := req.GetStringSlice("paths", nil)
	if len(rawPaths) == 0 {
		return mcp.NewToolResultError("paths must be a non-empty array of file paths"), nil
	}

	limitPerFile := helper.DefaultMaxReadBytes
	if mb := req.GetFloat("max_bytes_per_file", 0); mb > 0 {
		limitPerFile = int(mb)
	}
	includeBase64 := req.GetBool("include_base64", false)

	var sb strings.Builder

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

		text, readErr := helper.ReadOneFileAsText(p, limitPerFile, includeBase64)

		if sizeStr != "" && readErr == nil {
			lineCount := helper.CountContentLines(text)
			fmt.Fprintf(&sb, "--- %s (%s, %s) ---\n", p, sizeStr, helper.Pluralize(lineCount, "line"))
		} else if sizeStr != "" {
			fmt.Fprintf(&sb, "--- %s (%s) ---\n", p, sizeStr)
		} else {
			fmt.Fprintf(&sb, "--- %s ---\n", p)
		}

		if readErr != nil {
			failCount++
			fmt.Fprintf(&sb, "[ERROR] %v\n", readErr)
		} else {
			successCount++
			totalBytes += fileSize
			sb.WriteString(text)
			if !strings.HasSuffix(text, "\n") {
				sb.WriteByte('\n')
			}
		}
	}

	fmt.Fprintf(&sb, "\n[read %s", helper.Pluralize(successCount, "file"))
	if failCount > 0 {
		fmt.Fprintf(&sb, ", %s failed", helper.Pluralize(failCount, "file"))
	}
	if totalBytes > 0 {
		fmt.Fprintf(&sb, " (%s total)", helper.HumanizeBytes(totalBytes))
	}
	fmt.Fprintf(&sb, "]")

	return mcp.NewToolResultText(sb.String()), nil
}

func legacyWriteMultipleFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		absPath, _ := filepath.Abs(path)
		unlock := helper.LockFile(absPath)
		writePerm := helper.ExistingFilePerm(path, 0o644)
		var existingContent string
		if showDiff {
			if data, readErr := os.ReadFile(path); readErr == nil {
				existingContent = string(data)
			}
		}

		wErr := helper.AtomicWriteFile(path, []byte(content), writePerm)
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

func legacyFindReplaceInFilesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	excludePatterns := req.GetStringSlice("exclude_patterns", nil)
	showUnmodified := req.GetBool("show_unmodified", false)
	maxFileSize := int64(req.GetFloat("max_file_size", float64(defaultFindReplaceMaxFileSize)))
	if maxFileSize < 0 {
		maxFileSize = defaultFindReplaceMaxFileSize
	}

	if useRegex {
		if _, _, err := helper.ApplyEdit("test", oldStr, newStr, true, false); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	files, err := helper.CollectFiles(root, recursive, globPattern, showHidden, excludePatterns)
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
	var oversized []string
	totalCount := 0
	totalScanned := len(files)
	var errBuf strings.Builder

	for _, filePath := range files {
		if maxFileSize > 0 {
			if info, statErr := os.Stat(filePath); statErr == nil && info.Size() > maxFileSize {
				oversized = append(oversized, filePath)
				continue
			}
		}

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
	for i, p := range paths {
		if i > 0 {
			sb.WriteString("\n")
		}
		info, err := os.Lstat(p)
		if err != nil {
			fmt.Fprintf(&sb, "%s: false", p)
			missingCount++
			continue
		}
		existsCount++
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		if info.Mode()&os.ModeSymlink != 0 {
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

func legacyGetMultipleFileInfoHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	countLines := req.GetBool("count_lines", true)

	var sb strings.Builder
	fileCount := 0
	dirCount := 0
	symlinkCount := 0
	errorCount := 0
	for i, p := range paths {
		if i > 0 {
			if detailsMode {
				sb.WriteString("\n\n")
			} else {
				sb.WriteByte('\n')
			}
		}

		linfo, err := os.Lstat(p)
		if err != nil {
			if detailsMode {
				fmt.Fprintf(&sb, "Path:        %s\n[ERROR] %v", p, err)
			} else {
				fmt.Fprintf(&sb, "%s: ERROR %v", p, err)
			}
			errorCount++
			continue
		}

		kind := "file"
		if linfo.IsDir() {
			kind = "directory"
			dirCount++
		}
		if linfo.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
			symlinkCount++
		} else if !linfo.IsDir() {
			fileCount++
		}

		lineInfo := ""
		if countLines && !linfo.IsDir() {
			if f, _, _, binary, sniffErr := helper.SniffAndOpen(p); sniffErr == nil {
				if !binary {
					if n, countErr := helper.CountLines(f); countErr == nil {
						lineInfo = helper.Pluralize(n, "line")
					}
				}
				f.Close()
			}
		}

		if linfo.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			if detailsMode {
				abs, _ := filepath.Abs(p)
				fmt.Fprintf(&sb,
					"Path:        %s\n"+
						"Type:        %s\n"+
						"Size:        %s\n"+
						"Mode:        %s\n"+
						"Modified:    %s\n"+
						"Absolute:    %s",
					p,
					kind,
					helper.HumanizeBytes(linfo.Size()),
					linfo.Mode().String(),
					linfo.ModTime().Format("2006-01-02 15:04:05 MST"),
					abs,
				)
				if target != "" {
					fmt.Fprintf(&sb, "\nSymlink →    %s", target)
				}
				continue
			}
			if target != "" {
				fmt.Fprintf(&sb, "%s: %s -> %s, %s, mode %s, modified %s",
					p, kind, target, helper.HumanizeBytes(linfo.Size()), linfo.Mode().String(), linfo.ModTime().Format("2006-01-02 15:04 MST"))
				continue
			}
		}
		if detailsMode {
			abs, _ := filepath.Abs(p)
			fmt.Fprintf(&sb,
				"Path:        %s\n"+
					"Type:        %s\n"+
					"Size:        %s\n"+
					"Mode:        %s\n"+
					"Modified:    %s\n"+
					"Absolute:    %s",
				p,
				kind,
				helper.HumanizeBytes(linfo.Size()),
				linfo.Mode().String(),
				linfo.ModTime().Format("2006-01-02 15:04:05 MST"),
				abs,
			)
			if lineInfo != "" {
				fmt.Fprintf(&sb, "\nLines:       %s", lineInfo)
			}
			continue
		}
		fmt.Fprintf(&sb, "%s: %s %s", p, kind, helper.HumanizeBytes(linfo.Size()))
		if lineInfo != "" {
			fmt.Fprintf(&sb, ", %s", lineInfo)
		}
		fmt.Fprintf(&sb, ", mode %s, modified %s", linfo.Mode().String(), linfo.ModTime().Format("2006-01-02 15:04 MST"))
	}
	fmt.Fprintf(&sb, "\n\nSummary: %d/%d shown; files=%d dirs=%d symlinks=%d errors=%d.",
		len(paths), total, fileCount, dirCount, symlinkCount, errorCount)
	if truncated {
		fmt.Fprintf(&sb, " Output truncated after %d paths; increase limit or set limit=0 for all.", limit)
	}

	return mcp.NewToolResultText(sb.String()), nil
}
