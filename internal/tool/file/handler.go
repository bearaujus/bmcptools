package file

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

const (
	defaultDiffMaxBytes     = 256 * 1024
	defaultDiffFileMaxBytes = 2 * 1024 * 1024
	defaultEditFileMaxBytes = 10 * 1024 * 1024
	scannerMaxTokenBytes    = 10 * 1024 * 1024
)

func readFileHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	opts := ReadOptions{
		MaxBytes:        helper.DefaultMaxReadBytes,
		IncludeBase64:   req.GetBool("include_base64", false),
		ShowLineNumbers: req.GetBool("show_line_numbers", false),
	}
	if mb := req.GetFloat("max_bytes", 0); mb > 0 {
		opts.MaxBytes = int(mb)
	}
	if sl := req.GetFloat("start_line", 0); sl >= 1 {
		opts.StartLine = int(sl)
	}
	if el := req.GetFloat("end_line", 0); el >= 1 {
		opts.EndLine = int(el)
	}
	if headN := req.GetFloat("head", 0); headN >= 1 {
		opts.Head = int(headN)
	}
	if tailN := req.GetFloat("tail", 0); tailN >= 1 {
		opts.Tail = int(tailN)
	}
	if rawRanges, ok := req.GetArguments()["ranges"]; ok && rawRanges != nil {
		ranges, err := ParseReadLineRanges(rawRanges)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		opts.Ranges = ranges
	}
	if err := ValidateReadOptions(opts); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	out, err := ReadPathWithOptions(path, opts)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out.Text), nil
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

func requestByteLimit64(req mcp.CallToolRequest, name string, defaultValue int64) int64 {
	limit := int64(req.GetFloat(name, float64(defaultValue)))
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
	var diffNote string
	fileExisted := false
	existingReadable := false
	var existingReadErr error
	if info, statErr := os.Stat(path); statErr == nil {
		fileExisted = true
		writePerm = info.Mode().Perm()
		if showDiff {
			existingContent, diffNote, existingReadErr = helper.ReadExistingTextForDiff(path, info, defaultDiffFileMaxBytes)
			if existingReadErr == nil && diffNote == "" {
				existingReadable = true
			}
		}
	}
	if showDiffExplicit && showDiff && fileExisted && existingReadErr != nil {
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
		if existingReadable || !fileExisted {
			diff := limitTextBytes("Diff", helper.GenerateDiff(existingContent, content, 3), maxDiffBytes)
			if diff != "" {
				msg += "\n\n" + diff
			}
		}
		if diffNote != "" {
			msg += "\n\n" + diffNote
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
	ensureLeadingNewline := req.GetBool("ensure_leading_newline", false)

	if err := helper.MkdirAllClear(filepath.Dir(path), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
	}

	absPath, _ := filepath.Abs(path)
	unlock := helper.LockFile(absPath)
	defer unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot open file: %v", err)), nil
	}
	defer f.Close()

	writeContent := content
	insertedSeparator := false
	if ensureLeadingNewline && content != "" {
		if info, statErr := f.Stat(); statErr == nil && info.Size() > 0 {
			if _, seekErr := f.Seek(-1, io.SeekEnd); seekErr == nil {
				lastByte := make([]byte, 1)
				if n, readErr := io.ReadFull(f, lastByte); readErr == nil && n == 1 && lastByte[0] != '\n' && !strings.HasPrefix(content, "\n") {
					writeContent = "\n" + content
					insertedSeparator = true
				}
				if _, seekErr := f.Seek(0, io.SeekEnd); seekErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("seek error: %v", seekErr)), nil
				}
			}
		}
	}

	if _, err := f.WriteString(writeContent); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write error: %v", err)), nil
	}

	msg := fmt.Sprintf("Appended %s (%s) to %s", helper.HumanizeBytes(int64(len(writeContent))), helper.Pluralize(helper.CountContentLines(writeContent), "line"), path)
	if insertedSeparator {
		msg += " (inserted separating newline)"
	}
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

type editMatchWarning struct {
	pattern string
	count   int
	hint    string
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

	maxFileSize := requestByteLimit64(req, "max_file_size", defaultEditFileMaxBytes)
	f, err := os.Open(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot open file: %v", err)), nil
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat file: %v", err)), nil
	}
	if info.IsDir() {
		_ = f.Close()
		return mcp.NewToolResultError("path is a directory; edit_file only supports text files"), nil
	}
	if maxFileSize > 0 && info.Size() > maxFileSize {
		_ = f.Close()
		return mcp.NewToolResultError(fmt.Sprintf(
			"file is %s, which exceeds max_file_size=%s; raise max_file_size or set it to 0 for unlimited",
			helper.HumanizeBytes(info.Size()),
			helper.HumanizeBytes(maxFileSize),
		)), nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		_ = f.Close()
		return mcp.NewToolResultError(fmt.Sprintf("cannot read file: %v", err)), nil
	}
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	if helper.IsBinaryContent(sniff, http.DetectContentType(sniff)) {
		_ = f.Close()
		return mcp.NewToolResultError("path is a binary file; edit_file only supports text files"), nil
	}
	if err := f.Close(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot close file: %v", err)), nil
	}
	decoded := helper.DecodeTextBytes(data)
	original := decoded.Text
	current := original
	totalCount := 0
	var missed []string
	var multipleMatchWarnings []editMatchWarning

	for i, spec := range specs {
		if !spec.replaceAll && !spec.useRegex {
			exactCount := strings.Count(current, spec.oldStr)
			normalizedCount := helper.CountNormalizedMatches(current, spec.oldStr)
			matchCount := exactCount
			if normalizedCount > matchCount {
				matchCount = normalizedCount
			}
			if matchCount > 1 {
				multipleMatchWarnings = append(multipleMatchWarnings, editMatchWarning{
					pattern: spec.oldStr,
					count:   matchCount,
					hint:    helper.FindNearbyContext(current, spec.oldStr),
				})
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
		if len(multipleMatchWarnings) > 0 {
			preview += formatEditMatchWarnings(multipleMatchWarnings)
		}
		if diff != "" {
			preview += "\n\n" + diff
		}
		return mcp.NewToolResultText(preview), nil
	}

	writeCurrent := current
	if decoded.HasCRLF {
		writeCurrent = helper.RestoreCRLF(current)
	}
	writePerm := info.Mode().Perm()
	if err := helper.AtomicWriteFile(path, helper.EncodeTextBytes(writeCurrent, decoded.Encoding, decoded.HadBOM), writePerm); err != nil {
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
		msg += formatEditMatchWarnings(multipleMatchWarnings)
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

	stats, err := DeleteEntry(path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Deleted %s: %s (%s)", stats.Kind, path, helper.HumanizeBytes(stats.Size))), nil
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

	stats, err := CopyPath(src, dst, overwrite)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if stats.SourceKind == "directory" {
		return mcp.NewToolResultText(
			fmt.Sprintf("Copied directory %s \u2192 %s (%s, %s)",
				src,
				dst,
				helper.Pluralize(stats.Files, "file"),
				helper.HumanizeBytes(stats.Bytes),
			),
		), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Copied %s \u2192 %s (%s)", src, dst, helper.HumanizeBytes(stats.Bytes))), nil
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

	stats, err := MovePath(src, dst, overwrite)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if stats.SourceKind == "directory" {
		suffix := ""
		if stats.UsedFallback {
			suffix = fmt.Sprintf(" (%s, %s, copied+deleted)", helper.Pluralize(stats.Files, "file"), helper.HumanizeBytes(stats.Bytes))
		}
		return mcp.NewToolResultText(fmt.Sprintf("Moved directory %s \u2192 %s%s", src, dst, suffix)), nil
	}
	suffix := fmt.Sprintf(" (%s)", helper.HumanizeBytes(stats.Bytes))
	if stats.UsedFallback {
		suffix = fmt.Sprintf(" (%s, copied+deleted)", helper.HumanizeBytes(stats.Bytes))
	}
	return mcp.NewToolResultText(fmt.Sprintf("Moved %s \u2192 %s%s", src, dst, suffix)), nil
}

func getFileInfoHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}
	outputMode := req.GetString("output_mode", "compact")
	countLines := req.GetBool("count_lines", false)

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
	if countLines && !linfo.IsDir() && linfo.Mode()&os.ModeSymlink == 0 {
		if n, counted, _, countErr := helper.CountTextFileLinesWithInfo(path, linfo, true); countErr == nil {
			if counted {
				lineInfo = helper.Pluralize(n, "line")
			}
		}
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		return renderSymlinkInfo(path, linfo, outputMode)
	}

	if outputMode != "details" {
		result := fmt.Sprintf("%s: %s %s", path, kind, helper.HumanizeBytes(linfo.Size()))
		if lineInfo != "" {
			result += fmt.Sprintf(", %s", lineInfo)
		}
		result += fmt.Sprintf(", mode %s, modified %s", linfo.Mode().String(), linfo.ModTime().Format("2006-01-02 15:04 MST"))
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
		if meta, metaErr := helper.ReadSymlinkMetadata(path); metaErr == nil {
			return mcp.NewToolResultText(fmt.Sprintf("true — %q is a symlink %s", path, helper.FormatSymlinkCompact(meta))), nil
		}
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
		return helper.DecodeTextBytes(raw).Text, info, nil
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

	type checksumResult struct {
		path string
		hash string
		size int64
		err  error
	}
	results := make([]checksumResult, len(paths))
	helper.RunBoundedParallel(len(paths), func(i int) {
		hash, size, hashErr := helper.HashFile(paths[i], algorithm)
		results[i] = checksumResult{path: paths[i], hash: hash, size: size, err: hashErr}
	})
	for _, result := range results {
		if result.err != nil {
			fmt.Fprintf(&sb, "ERROR  %s \u2014 %v\n", result.path, result.err)
			continue
		}
		fmt.Fprintf(&sb, "%s  %s  (%s)\n", result.hash, result.path, helper.HumanizeBytes(result.size))
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

	linkAbs, _, err := validateCreateSymlinkPaths(link, source)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := helper.MkdirAllClear(filepath.Dir(linkAbs), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create parent directories: %v", err)), nil
	}

	if info, err := os.Lstat(linkAbs); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return mcp.NewToolResultError(fmt.Sprintf("link path %q already exists and is not a symlink", link)), nil
		}
		target, readErr := os.Readlink(linkAbs)
		if readErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to inspect existing symlink: %v", readErr)), nil
		}
		if symlinkTargetsEquivalent(linkAbs, target, source) {
			return mcp.NewToolResultText(fmt.Sprintf("Symlink already exists: %s → %s", link, source)), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("symlink %q already exists and points to %q", link, target)), nil
	} else if !os.IsNotExist(err) {
		return mcp.NewToolResultError(fmt.Sprintf("failed to inspect link path: %v", err)), nil
	}
	if err := os.Symlink(source, linkAbs); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create symlink: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Created symlink: %s → %s", link, source)), nil
}

func renderSymlinkInfo(path string, linfo os.FileInfo, outputMode string) (*mcp.CallToolResult, error) {
	meta, err := helper.ReadSymlinkMetadata(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot inspect symlink %q: %v", path, err)), nil
	}
	if outputMode != "details" {
		return mcp.NewToolResultText(fmt.Sprintf(
			"%s: symlink %s, mode %s, modified %s",
			path,
			helper.FormatSymlinkCompact(meta),
			linfo.Mode().String(),
			linfo.ModTime().Format("2006-01-02 15:04 MST"),
		)), nil
	}

	abs, _ := filepath.Abs(path)
	result := fmt.Sprintf(
		"Path:        %s\n"+
			"Type:        symlink\n"+
			"Mode:        %s\n"+
			"Modified:    %s\n"+
			"Absolute:    %s\n"+
			"Symlink →    %s",
		path,
		linfo.Mode().String(),
		linfo.ModTime().Format("2006-01-02 15:04:05 MST"),
		abs,
		meta.Target,
	)
	if meta.Dangling {
		result += "\nTarget:      dangling"
	} else {
		result += fmt.Sprintf("\nTarget:      %s", meta.TargetKind)
		result += fmt.Sprintf("\nTarget Size: %s", helper.HumanizeBytes(meta.TargetSize))
	}
	return mcp.NewToolResultText(result), nil
}

func formatEditMatchWarnings(warnings []editMatchWarning) string {
	var sb strings.Builder
	for _, warning := range warnings {
		fmt.Fprintf(&sb,
			"\nWarning: pattern %q matched multiple times (%d total); only the first occurrence was replaced. Use replace_all=true or add more context to old_str.",
			warning.pattern,
			warning.count,
		)
		if warning.hint != "" {
			sb.WriteString(warning.hint)
		}
	}
	return sb.String()
}

func detectArchiveFormat(path, explicit string) string {
	if explicit != "" {
		switch strings.ToLower(explicit) {
		case "tgz":
			return "tar.gz"
		default:
			return strings.ToLower(explicit)
		}
	}
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return ""
	}
}

func sameCleanPath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func isSubpath(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	if runtime.GOOS == "windows" {
		base = strings.ToLower(base)
		target = strings.ToLower(target)
	}
	return target == base || strings.HasPrefix(target, base+string(filepath.Separator))
}

func validateCreateSymlinkPaths(linkPath, source string) (string, string, error) {
	linkAbs, err := filepath.Abs(linkPath)
	if err != nil {
		return "", "", fmt.Errorf("invalid link path %q: %w", linkPath, err)
	}
	sourceAbs, err := resolveSymlinkTarget(linkAbs, source)
	if err != nil {
		return "", "", fmt.Errorf("invalid source path %q: %w", source, err)
	}
	linkDir := filepath.Dir(linkAbs)
	if !isSubpath(linkDir, sourceAbs) {
		return "", "", fmt.Errorf("refusing to create symlink %q → %q because the target resolves outside %q", linkPath, source, linkDir)
	}
	return linkAbs, sourceAbs, nil
}

func resolveSymlinkTarget(linkPath, target string) (string, error) {
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Abs(filepath.Join(filepath.Dir(linkPath), target))
}

func symlinkTargetsEquivalent(linkPath, existingTarget, requestedTarget string) bool {
	existingAbs, existingErr := resolveSymlinkTarget(linkPath, existingTarget)
	requestedAbs, requestedErr := resolveSymlinkTarget(linkPath, requestedTarget)
	if existingErr == nil && requestedErr == nil {
		return sameCleanPath(existingAbs, requestedAbs)
	}
	return sameCleanPath(existingTarget, requestedTarget)
}

func validateCompressSources(sources []string, output string) error {
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}
	outputInfo, outputStatErr := os.Stat(output)
	for _, src := range sources {
		srcInfo, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("cannot stat source %q: %w", src, err)
		}
		srcAbs, err := filepath.Abs(src)
		if err != nil {
			return fmt.Errorf("invalid source path %q: %w", src, err)
		}
		if sameCleanPath(srcAbs, outputAbs) {
			return fmt.Errorf("output path %q is also an input source", output)
		}
		if outputStatErr == nil && os.SameFile(srcInfo, outputInfo) {
			return fmt.Errorf("output path %q refers to the same file as input source %q", output, src)
		}
	}
	return nil
}

type archiveStats struct {
	Files int
	Dirs  int
}

type extractStats struct {
	Files           int
	SkippedSymlinks int
	SkippedSpecials int
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
		return mcp.NewToolResultError("cannot detect archive format from output path; specify format as \"zip\", \"tar\", or \"tar.gz\""), nil
	}
	if format != "zip" && format != "tar" && format != "tar.gz" {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported format %q; use \"zip\", \"tar\", or \"tar.gz\"", format)), nil
	}
	if err := validateCompressSources(paths, output); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if dir := filepath.Dir(output); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create output directory: %v", err)), nil
		}
	}

	var stats archiveStats
	var err error
	switch format {
	case "zip":
		stats, err = compressZip(paths, output)
	case "tar":
		stats, err = compressTar(paths, output)
	case "tar.gz":
		stats, err = compressTarGz(paths, output)
	}
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("compression failed: %v", err)), nil
	}

	info, statErr := os.Stat(output)
	sizeStr := "unknown"
	if statErr == nil {
		sizeStr = helper.HumanizeBytes(info.Size())
	}
	summary := fmt.Sprintf("Compressed %s", helper.Pluralize(stats.Files, "file"))
	if stats.Dirs > 0 {
		summary += fmt.Sprintf(" and %s", helper.Pluralize(stats.Dirs, "directory"))
	}
	summary += fmt.Sprintf(" into %s (%s, %s)", output, format, sizeStr)
	return mcp.NewToolResultText(summary), nil
}

func compressZip(sources []string, output string) (archiveStats, error) {
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return archiveStats{}, err
	}
	f, tmpPath, err := createArchiveTempFile(output)
	if err != nil {
		return archiveStats{}, err
	}
	tmpAbs, err := filepath.Abs(tmpPath)
	if err != nil {
		return archiveStats{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = f.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	w := zip.NewWriter(f)

	var stats archiveStats
	for _, src := range sources {
		src = filepath.Clean(src)
		base := filepath.Dir(src)
		err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			pathAbs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if sameCleanPath(pathAbs, outputAbs) || sameCleanPath(pathAbs, tmpAbs) {
				return nil
			}
			relPath, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)
			if info.IsDir() && relPath == "." {
				return nil
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = relPath
			if info.IsDir() {
				if !strings.HasSuffix(header.Name, "/") {
					header.Name += "/"
				}
				header.Method = zip.Store
				if _, err := w.CreateHeader(header); err != nil {
					return err
				}
				stats.Dirs++
				return nil
			}
			header.Method = zip.Deflate

			writer, err := w.CreateHeader(header)
			if err != nil {
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}

			_, copyErr := io.Copy(writer, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			stats.Files++
			return nil
		})
		if err != nil {
			_ = w.Close()
			_ = f.Close()
			return stats, err
		}
	}
	if err := w.Close(); err != nil {
		return stats, err
	}
	if err := f.Close(); err != nil {
		return stats, err
	}
	if err := replaceFileFromTemp(tmpPath, output); err != nil {
		return stats, err
	}
	cleanup = false
	return stats, nil
}

func compressTarGz(sources []string, output string) (archiveStats, error) {
	return compressTarArchive(sources, output, true)
}

func compressTar(sources []string, output string) (archiveStats, error) {
	return compressTarArchive(sources, output, false)
}

func compressTarArchive(sources []string, output string, gzipEnabled bool) (archiveStats, error) {
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return archiveStats{}, err
	}
	f, tmpPath, err := createArchiveTempFile(output)
	if err != nil {
		return archiveStats{}, err
	}
	tmpAbs, err := filepath.Abs(tmpPath)
	if err != nil {
		return archiveStats{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = f.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	var writer io.Writer = f
	var gw *gzip.Writer
	if gzipEnabled {
		gw = gzip.NewWriter(f)
		writer = gw
	}
	tw := tar.NewWriter(writer)

	var stats archiveStats
	for _, src := range sources {
		src = filepath.Clean(src)
		base := filepath.Dir(src)
		err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			pathAbs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if sameCleanPath(pathAbs, outputAbs) || sameCleanPath(pathAbs, tmpAbs) {
				return nil
			}
			relPath, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)
			if info.IsDir() && relPath == "." {
				return nil
			}

			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = relPath
			if info.IsDir() && !strings.HasSuffix(header.Name, "/") {
				header.Name += "/"
			}

			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				stats.Dirs++
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}

			_, copyErr := io.Copy(tw, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			stats.Files++
			return nil
		})
		if err != nil {
			_ = tw.Close()
			if gw != nil {
				_ = gw.Close()
			}
			_ = f.Close()
			return stats, err
		}
	}
	if err := tw.Close(); err != nil {
		if gw != nil {
			_ = gw.Close()
		}
		_ = f.Close()
		return stats, err
	}
	if gw != nil {
		if err := gw.Close(); err != nil {
			return stats, err
		}
	}
	if err := f.Close(); err != nil {
		return stats, err
	}
	if err := replaceFileFromTemp(tmpPath, output); err != nil {
		return stats, err
	}
	cleanup = false
	return stats, nil
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
		return mcp.NewToolResultError("cannot detect archive format from file extension; specify format as \"zip\", \"tar\", or \"tar.gz\""), nil
	}
	if format != "zip" && format != "tar" && format != "tar.gz" {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported format %q; use \"zip\", \"tar\", or \"tar.gz\"", format)), nil
	}

	if err := os.MkdirAll(output, 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create output directory: %v", err)), nil
	}
	resolvedOutput, err := filepath.EvalSymlinks(output)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to resolve output directory: %v", err)), nil
	}

	var stats extractStats
	switch format {
	case "zip":
		stats, err = extractZip(archive, resolvedOutput)
	case "tar":
		stats, err = extractTar(archive, resolvedOutput)
	case "tar.gz":
		stats, err = extractTarGz(archive, resolvedOutput)
	}
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("extraction failed: %v", err)), nil
	}
	summary := fmt.Sprintf("Extracted %d files to %s", stats.Files, output)
	if stats.SkippedSymlinks > 0 || stats.SkippedSpecials > 0 {
		parts := make([]string, 0, 2)
		if stats.SkippedSymlinks > 0 {
			parts = append(parts, helper.Pluralize(stats.SkippedSymlinks, "symlink entry"))
		}
		if stats.SkippedSpecials > 0 {
			parts = append(parts, helper.Pluralize(stats.SkippedSpecials, "special entry"))
		}
		summary += fmt.Sprintf(" (skipped %s)", strings.Join(parts, " and "))
	}
	return mcp.NewToolResultText(summary), nil
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
	if !isSubpath(absDest, absTarget) {
		return "", fmt.Errorf("path %q escapes output directory", name)
	}
	return absTarget, nil
}

func ensureNoSymlinkAncestors(base, target string, includeTarget bool) error {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("invalid extraction base: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("invalid extraction target: %w", err)
	}
	if !isSubpath(absBase, absTarget) {
		return fmt.Errorf("path %q escapes output directory", target)
	}

	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if !includeTarget && len(parts) > 0 {
		parts = parts[:len(parts)-1]
	}
	cur := absBase
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to extract through symlink %q", cur)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q exists and is not a directory", cur)
		}
	}
	return nil
}

func mkdirAllExtractSafe(base, dir string) error {
	if err := ensureNoSymlinkAncestors(base, dir, true); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return ensureNoSymlinkAncestors(base, dir, true)
}

func createExtractFile(base, target string, perm os.FileMode) (*os.File, error) {
	if err := mkdirAllExtractSafe(base, filepath.Dir(target)); err != nil {
		return nil, err
	}
	if err := ensureNoSymlinkAncestors(base, target, false); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to overwrite symlink %q", target)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("refusing to overwrite directory %q with file", target)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if perm == 0 {
		perm = 0o644
	}
	return os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
}

func extractZip(archive, dest string) (extractStats, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return extractStats{}, err
	}
	defer r.Close()

	var stats extractStats
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			target, err := safeJoin(dest, f.Name)
			if err != nil {
				return stats, err
			}
			if err := mkdirAllExtractSafe(dest, target); err != nil {
				return stats, err
			}
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			stats.SkippedSymlinks++
			continue
		}
		if isSpecialExtractMode(f.Mode()) {
			stats.SkippedSpecials++
			continue
		}

		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return stats, err
		}

		rc, err := f.Open()
		if err != nil {
			return stats, err
		}

		out, err := createExtractFile(dest, target, f.FileInfo().Mode().Perm())
		if err != nil {
			rc.Close()
			return stats, err
		}

		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return stats, err
		}
		out.Close()
		rc.Close()
		stats.Files++
	}
	return stats, nil
}

func extractTarGz(archive, dest string) (extractStats, error) {
	f, err := os.Open(archive)
	if err != nil {
		return extractStats{}, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return extractStats{}, err
	}
	defer gr.Close()

	return extractTarStream(gr, dest)
}

func extractTar(archive, dest string) (extractStats, error) {
	f, err := os.Open(archive)
	if err != nil {
		return extractStats{}, err
	}
	defer f.Close()

	return extractTarStream(f, dest)
}

func extractTarStream(r io.Reader, dest string) (extractStats, error) {
	tr := tar.NewReader(r)
	var stats extractStats
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return stats, err
		}

		target, err := safeJoin(dest, header.Name)
		if err != nil {
			return stats, err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := mkdirAllExtractSafe(dest, target); err != nil {
				return stats, err
			}
		case tar.TypeReg, tar.TypeRegA:
			out, err := createExtractFile(dest, target, header.FileInfo().Mode().Perm())
			if err != nil {
				return stats, err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return stats, err
			}
			out.Close()
			stats.Files++
		case tar.TypeSymlink, tar.TypeLink:
			stats.SkippedSymlinks++
		default:
			stats.SkippedSpecials++
		}
	}
	return stats, nil
}

func createArchiveTempFile(output string) (*os.File, string, error) {
	dir := filepath.Dir(output)
	tmp, err := os.CreateTemp(dir, ".bmcptools-archive-*")
	if err != nil {
		return nil, "", err
	}
	return tmp, tmp.Name(), nil
}

func replaceFileFromTemp(tmpPath, output string) error {
	if _, err := os.Stat(output); os.IsNotExist(err) {
		return os.Rename(tmpPath, output)
	} else if err != nil {
		return err
	}

	backup, err := os.CreateTemp(filepath.Dir(output), ".bmcptools-archive-backup-*")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if closeErr := backup.Close(); closeErr != nil {
		_ = os.Remove(backupPath)
		return closeErr
	}
	_ = os.Remove(backupPath)
	if err := os.Rename(output, backupPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, output); err != nil {
		_ = os.Rename(backupPath, output)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func isSpecialExtractMode(mode os.FileMode) bool {
	return mode&(os.ModeNamedPipe|os.ModeDevice|os.ModeCharDevice|os.ModeSocket|os.ModeIrregular) != 0
}
