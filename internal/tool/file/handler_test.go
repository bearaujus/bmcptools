package file

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bearaujus/bmcptools/internal/helper"
)

// ── applyEdit ────────────────────────────────────────────────────────────────

func TestApplyEditPlain(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		oldStr     string
		newStr     string
		replaceAll bool
		wantResult string
		wantCount  int
	}{
		{"first only", "hello world hello", "hello", "hi", false, "hi world hello", 1},
		{"replace all", "hello world hello", "hello", "hi", true, "hi world hi", 2},
		{"not found", "hello world", "xyz", "abc", false, "hello world", 0},
		{"empty replacement", "aXb", "X", "", false, "ab", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, err := helper.ApplyEdit(tt.original, tt.oldStr, tt.newStr, false, tt.replaceAll)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantResult {
				t.Errorf("result = %q, want %q", got, tt.wantResult)
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestApplyEditRegex(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		pattern    string
		newStr     string
		replaceAll bool
		wantResult string
		wantCount  int
		wantErr    bool
	}{
		{"regex first", "foo123bar foo456bar", `foo\d+bar`, "X", false, "X foo456bar", 1, false},
		{"regex all", "foo123bar foo456bar", `foo\d+bar`, "X", true, "X X", 2, false},
		{"backreference", "2024-01-15", `(\d{4})-(\d{2})-(\d{2})`, "$3/$2/$1", true, "15/01/2024", 1, false},
		{"not found", "hello", `xyz\d+`, "x", false, "hello", 0, false},
		{"invalid regex", "hello", "[invalid", "x", false, "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, err := helper.ApplyEdit(tt.original, tt.pattern, tt.newStr, true, tt.replaceAll)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.wantResult {
				t.Errorf("result = %q, want %q", got, tt.wantResult)
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

// ── read_file ────────────────────────────────────────────────────────────────

func TestReadFileHandler(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "read.txt")
	if err := os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := readFileHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "line1") || !strings.Contains(text, "line3") {
		t.Errorf("unexpected content: %q", text)
	}
}

func TestReadFileHandlerLineRange(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "range.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "start_line": float64(2), "end_line": float64(3)})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "L2") || !strings.Contains(text, "L3") {
		t.Errorf("expected L2 and L3 in result: %q", text)
	}
	if strings.Contains(text, "L1") || strings.Contains(text, "L5") {
		t.Errorf("did not expect L1/L5 in result: %q", text)
	}
}

func TestReadFileMissingPath(t *testing.T) {
	result, err := readFileHandler(nil, newTestRequest(map[string]any{"path": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error result for empty path")
	}
}

func TestReadFileNotExist(t *testing.T) {
	result, err := readFileHandler(nil, newTestRequest(map[string]any{"path": "/nonexistent/file.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error result for nonexistent file")
	}
}

func TestReadFileBinarySummaryByDefault(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := readFileHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "[BINARY FILE]") {
		t.Errorf("expected binary summary: %q", text)
	}
	if strings.Contains(text, "Base64:\n") {
		t.Errorf("binary read should not include base64 by default: %q", text)
	}
}

func TestReadFileBinaryIncludeBase64(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := readFileHandler(nil, newTestRequest(map[string]any{
		"path":           f,
		"include_base64": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Base64:\n") {
		t.Errorf("expected base64 when include_base64=true: %q", text)
	}
}

// ── write_file ───────────────────────────────────────────────────────────────

func TestWriteFileHandler(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "write.txt")
	req := newTestRequest(map[string]any{"path": f, "content": "hello world"})
	result, err := writeFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want %q", string(data), "hello world")
	}
}

func TestWriteFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sub", "deep", "file.txt")
	result, err := writeFileHandler(nil, newTestRequest(map[string]any{"path": f, "content": "data"}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if _, err := os.Stat(f); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// ── append_to_file ───────────────────────────────────────────────────────────

func TestAppendFileHandler(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "append.txt")
	if err := os.WriteFile(f, []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := appendFileHandler(nil, newTestRequest(map[string]any{"path": f, "content": "line2\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	if string(data) != "line1\nline2\n" {
		t.Errorf("file content = %q", string(data))
	}
}

// ── edit_file ────────────────────────────────────────────────────────────────

func TestEditFileHandler(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(f, []byte("foo bar foo"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "old_str": "foo", "new_str": "baz"})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	if string(data) != "baz bar foo" {
		t.Errorf("file content = %q, want %q", string(data), "baz bar foo")
	}
}

func TestEditFileHandlerReplaceAll(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "edit_all.txt")
	if err := os.WriteFile(f, []byte("foo bar foo"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "old_str": "foo", "new_str": "baz", "replace_all": true})
	if _, err := editFileHandler(nil, req); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f)
	if string(data) != "baz bar baz" {
		t.Errorf("file content = %q, want %q", string(data), "baz bar baz")
	}
}

func TestEditFileHandlerRegexBackreferenceFirstOnly(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "version.txt")
	if err := os.WriteFile(f, []byte("version = 1.2.3\nversion = 4.5.6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{
		"path":      f,
		"old_str":   `version = (\d+)\.(\d+)\.(\d+)`,
		"new_str":   "version = $3.$2.$1",
		"use_regex": true,
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	got := string(data)
	want := "version = 3.2.1\nversion = 4.5.6\n"
	if got != want {
		t.Errorf("regex first replacement = %q, want %q", got, want)
	}
}

// ── delete_file ──────────────────────────────────────────────────────────────

func TestDeleteFileHandler(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "del.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := deleteFileHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if _, statErr := os.Stat(f); !os.IsNotExist(statErr) {
		t.Error("file still exists after deletion")
	}
}

func TestDeleteFileRejectsDir(t *testing.T) {
	dir := t.TempDir()
	result, err := deleteFileHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when deleting a directory with delete_file")
	}
}

// ── copy_file ────────────────────────────────────────────────────────────────

func TestCopyFileHandler(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("copy me"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := copyFileHandler(nil, newTestRequest(map[string]any{"source": src, "destination": dst}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "copy me" {
		t.Errorf("destination content = %q, want %q", string(data), "copy me")
	}
}

func TestCopyFileHandlerNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := copyFileHandler(nil, newTestRequest(map[string]any{"source": src, "destination": dst}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(res2) {
		t.Error("expected error when overwrite=false and destination exists")
	}
}

// ── move_file ────────────────────────────────────────────────────────────────

func TestMoveFileHandler(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "moved.txt")
	if err := os.WriteFile(src, []byte("move me"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := moveFileHandler(nil, newTestRequest(map[string]any{"source": src, "destination": dst}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
		t.Error("source still exists after move")
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "move me" {
		t.Errorf("destination content = %q, want %q", string(data), "move me")
	}
}

// ── get_file_info ────────────────────────────────────────────────────────────

func TestGetFileInfoHandler(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "info.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{"path": f, "count_lines": true}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "file") {
		t.Errorf("expected 'file' type in result: %q", text)
	}
	if !strings.Contains(text, "info.txt") {
		t.Errorf("expected filename in result: %q", text)
	}
	if strings.Contains(text, "Path:") {
		t.Errorf("default get_file_info output should be compact: %q", text)
	}
}

func TestGetFileInfoHandlerDetailsMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "info.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{
		"path":        f,
		"output_mode": "details",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Path:") || !strings.Contains(text, "Type:        file") || !strings.Contains(text, "Modified:") {
		t.Errorf("expected expanded labeled metadata in details mode: %q", text)
	}
}

func TestGetFileInfoHandlerLineCount(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{"path": f, "count_lines": true}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "3 lines") {
		t.Errorf("expected '3 lines' in get_file_info output for 3-line file: %q", text)
	}
}

func TestGetFileInfoHandlerBinaryNoLineCount(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, "Lines:") {
		t.Errorf("binary file should not include Lines: in get_file_info output: %q", text)
	}
}

func TestGetFileInfoHandlerDirectory(t *testing.T) {
	dir := t.TempDir()
	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "directory") {
		t.Errorf("expected 'directory' type in result: %q", text)
	}
}

func TestGetFileInfoHandlerSkipsLargeLineCountByDefault(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "large.txt")
	lineCount := int(helper.AutoLineCountMaxBytes/5) + 5000
	content := strings.Repeat("line\n", lineCount)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, fmt.Sprintf("%d lines", lineCount)) {
		t.Fatalf("default large-file metadata should skip exact line counts: %q", text)
	}
}

func TestGetFileInfoHandlerForceCountLinesForLargeFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "large_counted.txt")
	lineCount := int(helper.AutoLineCountMaxBytes/5) + 5000
	content := strings.Repeat("line\n", lineCount)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{
		"path":        f,
		"count_lines": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, fmt.Sprintf("%d lines", lineCount)) {
		t.Fatalf("expected explicit large-file line count, got: %q", text)
	}
}

// ── edit_file diff output ─────────────────────────────────────────────────────

func TestEditFileHandlerShowsDiff(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "diff_test.txt")
	if err := os.WriteFile(f, []byte("func OldName() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "old_str": "OldName", "new_str": "NewName"})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "-") || !strings.Contains(text, "+") {
		t.Errorf("expected diff markers in edit_file output: %q", text)
	}
	if !strings.Contains(text, "OldName") {
		t.Errorf("expected removed line in diff: %q", text)
	}
	if !strings.Contains(text, "NewName") {
		t.Errorf("expected added line in diff: %q", text)
	}
}

// ── read_file head/tail ───────────────────────────────────────────────────────

func TestReadFileHandlerHead(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "head": float64(2)})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "L1") || !strings.Contains(text, "L2") {
		t.Errorf("expected L1 and L2 in head result: %q", text)
	}
	if strings.Contains(text, "L3") || strings.Contains(text, "L5") {
		t.Errorf("did not expect L3/L5 in head=2 result: %q", text)
	}
}

func TestReadFileHandlerTail(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "tail": float64(2)})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "L4") || !strings.Contains(text, "L5") {
		t.Errorf("expected L4 and L5 in tail result: %q", text)
	}
	if strings.Contains(text, "L1") || strings.Contains(text, "L2") {
		t.Errorf("did not expect L1/L2 in tail=2 result: %q", text)
	}
}

func TestReadFileHandlerHeadThenTail(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"path": f, "head": float64(5), "tail": float64(2)})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "L4") || !strings.Contains(text, "L5") {
		t.Fatalf("expected last 2 lines of the head window: %q", text)
	}
	if strings.Contains(text, "L1") || strings.Contains(text, "L2") || strings.Contains(text, "L3") {
		t.Fatalf("did not expect earlier head-window lines after tail composition: %q", text)
	}
}

func TestReadFileHandlerRejectsConflictingSelectors(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := readFileHandler(nil, newTestRequest(map[string]any{
		"path":       f,
		"head":       float64(1),
		"start_line": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatalf("expected conflicting selector error, got: %q", resultText(result))
	}
	if !strings.Contains(resultText(result), "conflicting read selectors") {
		t.Fatalf("expected conflicting selector guidance, got: %q", resultText(result))
	}
}

func TestReadFileHandlerLargeHeadOmitsTotalLineCount(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "large.txt")
	lineCount := int(helper.AutoLineCountMaxBytes/8) + 5000
	var sb strings.Builder
	for i := 0; i < lineCount; i++ {
		fmt.Fprintf(&sb, "L%06d\n", i)
	}
	if err := os.WriteFile(f, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := readFileHandler(nil, newTestRequest(map[string]any{"path": f, "head": float64(2)}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "lines 1..2") {
		t.Fatalf("expected bounded head header: %q", text)
	}
	if strings.Contains(text, "of ") {
		t.Fatalf("large bounded head should skip the exact total line count: %q", text)
	}
}

// ── writeFile empty content ───────────────────────────────────────────────────

func TestWriteFileEmptyContent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.txt")
	req := newTestRequest(map[string]any{"path": f, "content": ""})
	result, err := writeFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "0 lines") {
		t.Errorf("expected '0 lines' for empty file write, got: %q", text)
	}
}

func TestWriteFileLineCount(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "count.txt")
	req := newTestRequest(map[string]any{"path": f, "content": "hello\nworld\n"})
	result, err := writeFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "2 lines") {
		t.Errorf("expected '2 lines' for hello/world content, got: %q", text)
	}
}

// ── edit_file batched edits ───────────────────────────────────────────────────

func TestEditFileHandlerBatchedEdits(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "batch.txt")
	if err := os.WriteFile(f, []byte("foo bar baz"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path": f,
		"edits": []any{
			map[string]any{"old_str": "foo", "new_str": "FOO"},
			map[string]any{"old_str": "baz", "new_str": "BAZ"},
		},
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	if string(data) != "FOO bar BAZ" {
		t.Errorf("batched edits: got %q, want %q", string(data), "FOO bar BAZ")
	}
	text := resultText(result)
	if !strings.Contains(text, "-foo bar baz") && !strings.Contains(text, "-foo") {
		t.Errorf("expected diff in output: %q", text)
	}
}

func TestEditFileHandlerBatchedEditsAllNotFound(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "noop.txt")
	if err := os.WriteFile(f, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{
		"path": f,
		"edits": []any{
			map[string]any{"old_str": "xyz", "new_str": "abc"},
		},
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("expected 'not found' message: %q", text)
	}
}

// ── read_file show_line_numbers ───────────────────────────────────────────────

func TestReadFileHandlerShowLineNumbers(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "numbered.txt")
	if err := os.WriteFile(f, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "show_line_numbers": true})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "     1|") {
		t.Errorf("expected line number prefix '     1|' in output: %q", text)
	}
	if !strings.Contains(text, "     3|") {
		t.Errorf("expected line number prefix '     3|' in output: %q", text)
	}
}

// ── read_file range headers avoid extra full-file rescans ───────────────────

func TestReadFileHandlerHeadShowsTotalLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "total.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "head": float64(2)})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, "of 5 lines") {
		t.Errorf("head reads should not rescan just to report total lines: %q", text)
	}
	if !strings.Contains(text, "L1") || !strings.Contains(text, "L2") {
		t.Errorf("expected L1 and L2 in head=2 result: %q", text)
	}
}

func TestReadFileHandlerTailShowsTotalLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "total.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "tail": float64(2)})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "5") {
		t.Errorf("expected total line count (5) in tail= header: %q", text)
	}
	if !strings.Contains(text, "L4") || !strings.Contains(text, "L5") {
		t.Errorf("expected L4 and L5 in tail=2 result: %q", text)
	}
}

func TestReadFileHandlerRangeShowsTotalLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "total.txt")
	if err := os.WriteFile(f, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{"path": f, "start_line": float64(2), "end_line": float64(3)})
	result, err := readFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, "of 5 lines") {
		t.Errorf("range reads should not rescan just to report total lines: %q", text)
	}
	if !strings.Contains(text, "L2") || !strings.Contains(text, "L3") {
		t.Errorf("expected L2 and L3 in range result: %q", text)
	}
}

func TestReadFileLineRangeMaxBytesTruncationNotice(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "large-range.txt")
	var content strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&content, "line %02d\n", i)
	}
	if err := os.WriteFile(f, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := readFileHandler(nil, newTestRequest(map[string]any{
		"path":       f,
		"start_line": float64(1),
		"end_line":   float64(20),
		"max_bytes":  float64(40),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "TRUNCATED") {
		t.Errorf("expected max_bytes truncation notice for line range: %q", text)
	}
}

// ── edit_file dry_run ─────────────────────────────────────────────────────────

func TestEditFileHandlerDryRun(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "dry.txt")
	original := "func OldName() {}\n"
	if err := os.WriteFile(f, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{
		"path":    f,
		"old_str": "OldName",
		"new_str": "NewName",
		"dry_run": true,
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN in output: %q", text)
	}
	if !strings.Contains(text, "-") || !strings.Contains(text, "+") {
		t.Errorf("expected diff markers in dry_run output: %q", text)
	}
	data, _ := os.ReadFile(f)
	if string(data) != original {
		t.Errorf("dry_run modified the file: %q", string(data))
	}
}

func TestEditFileHandlerDiffIsCapped(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "dry_large.txt")
	if err := os.WriteFile(f, []byte(strings.Repeat("OldName", 40)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{
		"path":           f,
		"old_str":        "OldName",
		"new_str":        "NewName",
		"replace_all":    true,
		"dry_run":        true,
		"max_diff_bytes": float64(40),
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Diff truncated") {
		t.Errorf("expected capped diff notice, got: %s", resultText(result))
	}
}

func TestEditFileHandlerDryRunNoMatch(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "dry_nomatch.txt")
	if err := os.WriteFile(f, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{
		"path":    f,
		"old_str": "nothere",
		"new_str": "X",
		"dry_run": true,
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "nothere") {
		t.Errorf("expected pattern name in not-found message: %q", text)
	}
}

func TestWriteFileShowDiff(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "patch.txt")
	if err := os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{
		"path":      f,
		"content":   "line1\nLINE2\nline3\n",
		"show_diff": true,
	})
	result, err := writeFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "-line2") {
		t.Errorf("expected removed line in diff: %q", text)
	}
	if !strings.Contains(text, "+LINE2") {
		t.Errorf("expected added line in diff: %q", text)
	}
}

func TestWriteFileShowDiffNewFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "new.txt")
	req := newTestRequest(map[string]any{
		"path":      f,
		"content":   "hello\n",
		"show_diff": true,
	})
	result, err := writeFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}

// ── path_exists ───────────────────────────────────────────────────────────────

func TestPathExistsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := pathExistsHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.HasPrefix(text, "true") {
		t.Errorf("expected 'true' prefix for existing file, got: %q", text)
	}
	if !strings.Contains(text, "file") {
		t.Errorf("expected 'file' in result, got: %q", text)
	}
}

func TestPathExistsDirectory(t *testing.T) {
	dir := t.TempDir()
	result, err := pathExistsHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.HasPrefix(text, "true") {
		t.Errorf("expected 'true' prefix for existing directory, got: %q", text)
	}
	if !strings.Contains(text, "directory") {
		t.Errorf("expected 'directory' in result, got: %q", text)
	}
}

func TestPathExistsNotFound(t *testing.T) {
	result, err := pathExistsHandler(nil, newTestRequest(map[string]any{
		"path": filepath.Join(t.TempDir(), "nonexistent.txt"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.HasPrefix(text, "false") {
		t.Errorf("expected 'false' prefix for missing path, got: %q", text)
	}
}

func TestPathExistsMissingArg(t *testing.T) {
	result, err := pathExistsHandler(nil, newTestRequest(map[string]any{"path": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty path")
	}
}

// ── move_file overwrite ───────────────────────────────────────────────────────

func TestMoveFileHandlerNoOverwriteRejectsExistingDst(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("src"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("dst"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := moveFileHandler(nil, newTestRequest(map[string]any{
		"source":      src,
		"destination": dst,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when destination exists and overwrite=false")
	}
}

func TestMoveFileHandlerOverwriteReplacesExistingDst(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("new-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := moveFileHandler(nil, newTestRequest(map[string]any{
		"source":      src,
		"destination": dst,
		"overwrite":   true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-content" {
		t.Errorf("expected dst to contain 'new-content', got %q", data)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("expected source file to be removed after move")
	}
}

// ── edit_file context_lines ───────────────────────────────────────────────────

func TestEditFileHandlerContextLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "code.go")
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	if err := os.WriteFile(f, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":          f,
		"old_str":       "line10",
		"new_str":       "CHANGED",
		"context_lines": float64(0),
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if strings.Contains(text, " line9") || strings.Contains(text, " line11") {
		t.Errorf("context_lines=0 should produce no context lines in diff: %q", text)
	}
	if !strings.Contains(text, "-line10") {
		t.Errorf("expected -line10 in diff: %q", text)
	}
	if !strings.Contains(text, "+CHANGED") {
		t.Errorf("expected +CHANGED in diff: %q", text)
	}
}

// ── write_file / read_file round-trip: special content ───────────────────────

func TestWriteReadJSON(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "data.json")

	content := "{\n  \"name\": \"AI Assistant\",\n  \"version\": 1,\n  \"features\": [\"ask_user\", \"notify_user\"],\n  \"nested\": {\"key\": \"value with \\\"escaped\\\" quotes\"}\n}\n"

	result, err := writeFileHandler(nil, newTestRequest(map[string]any{"path": f, "content": content}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("writeFileHandler error: %s", resultText(result))
	}

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("JSON round-trip mismatch:\ngot:  %q\nwant: %q", string(got), content)
	}

	readResult, err := readFileHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(readResult) {
		t.Fatalf("readFileHandler error: %s", resultText(readResult))
	}
	readText := resultText(readResult)
	if !strings.Contains(readText, `"AI Assistant"`) {
		t.Errorf("expected JSON key in read result: %q", readText)
	}
	if !strings.Contains(readText, `"ask_user"`) {
		t.Errorf("expected JSON array value in read result: %q", readText)
	}
}

func TestWriteReadPowerShell(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "script.ps1")

	content := `Add-Type -AssemblyName PresentationFramework
$title = "Hello, World!"
$msg   = 'It''s a single-quoted string'
$block = @'
This is a here-string with 'single quotes' and "double quotes".
It can contain $variables without expansion.
'@
$dblock = @"
This expands: $title
BackSlash: C:\Users\Name\Documents
"@
Write-Output $block
Write-Output $dblock
`

	result, err := writeFileHandler(nil, newTestRequest(map[string]any{"path": f, "content": content}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("writeFileHandler error: %s", resultText(result))
	}

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("PowerShell round-trip mismatch:\ngot:  %q\nwant: %q", string(got), content)
	}

	readResult, err := readFileHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(readResult) {
		t.Fatalf("readFileHandler error: %s", resultText(readResult))
	}
	readText := resultText(readResult)
	if !strings.Contains(readText, `C:\Users\Name\Documents`) {
		t.Errorf("expected backslash path in read result: %q", readText)
	}
	if !strings.Contains(readText, `$variables without expansion`) {
		t.Errorf("expected literal dollar sign in read result: %q", readText)
	}
}

func TestWriteReadEmoji(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "emoji.txt")

	content := "Hello 🎉 World!\nGo is awesome 🐹\nRocket launch: 🚀\nFlag: 🇯🇵\nMath: π ≈ 3.14\nCJK: 日本語\n"

	result, err := writeFileHandler(nil, newTestRequest(map[string]any{"path": f, "content": content}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("writeFileHandler error: %s", resultText(result))
	}

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("emoji round-trip mismatch:\ngot:  %q\nwant: %q", string(got), content)
	}

	readResult, err := readFileHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(readResult) {
		t.Fatalf("readFileHandler error: %s", resultText(readResult))
	}
	readText := resultText(readResult)
	for _, want := range []string{"🎉", "🐹", "🚀", "π", "日本語"} {
		if !strings.Contains(readText, want) {
			t.Errorf("expected %q in read result: %q", want, readText)
		}
	}
}

func TestAppendReadEmoji(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "append_emoji.txt")

	if err := os.WriteFile(f, []byte("Line 1: 🌍\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := appendFileHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"content": "Line 2: 🌙\nLine 3: ⭐\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("appendFileHandler error: %s", resultText(result))
	}

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	want := "Line 1: 🌍\nLine 2: 🌙\nLine 3: ⭐\n"
	if string(got) != want {
		t.Errorf("append emoji round-trip mismatch:\ngot:  %q\nwant: %q", string(got), want)
	}
}

// ── edit_file CRLF ────────────────────────────────────────────────────────────

func TestEditFileHandlerCRLF(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "crlf.txt")
	crlfContent := "line one\r\nline two\r\nline three\r\n"
	if err := os.WriteFile(f, []byte(crlfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    f,
		"old_str": "line two",
		"new_str": "LINE TWO",
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	data, _ := os.ReadFile(f)
	got := string(data)
	want := "line one\r\nLINE TWO\r\nline three\r\n"
	if got != want {
		t.Errorf("CRLF edit result = %q, want %q", got, want)
	}
}

func TestEditFilePreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix permission bits")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "tool.sh")
	if err := os.WriteFile(f, []byte("echo old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := editFileHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"old_str": "old",
		"new_str": "new",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	info, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("mode after edit = %o, want 755", got)
	}
}

func TestEditFileHandlerCRLFMultiline(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "crlf_multi.txt")
	crlfContent := "alpha\r\nbeta\r\ngamma\r\n"
	if err := os.WriteFile(f, []byte(crlfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    f,
		"old_str": "alpha\nbeta",
		"new_str": "ALPHA\nBETA",
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	data, _ := os.ReadFile(f)
	got := string(data)
	want := "ALPHA\r\nBETA\r\ngamma\r\n"
	if got != want {
		t.Errorf("CRLF multiline edit = %q, want %q", got, want)
	}
}

func TestEditFileHandlerMultiMatchWarning(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "multi.txt")
	if err := os.WriteFile(f, []byte("foo\nfoo\nbar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    f,
		"old_str": "foo",
		"new_str": "baz",
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	txt := resultText(result)
	if !strings.Contains(txt, "Warning") || !strings.Contains(txt, "matched multiple times") {
		t.Errorf("expected multi-match warning, got: %s", txt)
	}
	data, _ := os.ReadFile(f)
	if string(data) != "baz\nfoo\nbar\n" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

func TestEditFileHandlerNotFoundContext(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ctx.txt")
	if err := os.WriteFile(f, []byte("hello world\ngoodbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    f,
		"old_str": "hello world\nXXX_DOES_NOT_EXIST",
		"new_str": "replacement",
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	txt := resultText(result)
	if !strings.Contains(txt, "not found") {
		t.Errorf("expected 'not found' in output, got: %s", txt)
	}
	if !strings.Contains(txt, "nearby") || !strings.Contains(txt, "hello world") {
		t.Errorf("expected nearby context hint with 'hello world', got: %s", txt)
	}
}

// ── edit_file batch missed pattern reporting ──────────────────────────────────

func TestEditFileHandlerBatchMissedPatterns(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "partial.txt")
	if err := os.WriteFile(f, []byte("foo bar"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path": f,
		"edits": []any{
			map[string]any{"old_str": "foo", "new_str": "FOO"},
			map[string]any{"old_str": "xyz", "new_str": "ABC"},
		},
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "xyz") {
		t.Errorf("expected unmatched pattern 'xyz' reported in output: %q", text)
	}
	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), "FOO") {
		t.Errorf("expected FOO in file after partial batch edit: %q", string(data))
	}
}

func TestEditFileHandlerSinglePatternNotFoundMessage(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "nomatch.txt")
	if err := os.WriteFile(f, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := newTestRequest(map[string]any{
		"path":    f,
		"old_str": "nothere",
		"new_str": "X",
	})
	result, err := editFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "nothere") {
		t.Errorf("expected pattern name in not-found message: %q", text)
	}
}

// ── read_file header with line count ─────────────────────────────────────────

func TestReadFileHandlerFullFileHeader(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "myfile.txt")
	if err := os.WriteFile(f, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := readFileHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "myfile.txt") {
		t.Errorf("expected filename in header: %q", text)
	}
	if !strings.Contains(text, "3 lines") {
		t.Errorf("expected '3 lines' in header: %q", text)
	}
	if !strings.Contains(text, "alpha") {
		t.Errorf("expected file content 'alpha': %q", text)
	}
}

// ── write_file overwrite shows diff by default ────────────────────────────────

func TestWriteFileOverwriteShowsDiff(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "overwrite.txt")
	if err := os.WriteFile(f, []byte("original content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":      f,
		"content":   "new content\n",
		"show_diff": true,
	})
	result, err := writeFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	txt := resultText(result)
	if !strings.Contains(txt, "Overwrote") {
		t.Errorf("expected 'Overwrote' in output for existing file, got: %s", txt)
	}
	if !strings.Contains(txt, "original content") {
		t.Errorf("expected diff showing removed content, got: %s", txt)
	}
}

func TestWriteFileOverwriteDiffIsCapped(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "overwrite-large.txt")
	if err := os.WriteFile(f, []byte(strings.Repeat("a", 200)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := writeFileHandler(nil, newTestRequest(map[string]any{
		"path":           f,
		"content":        strings.Repeat("b", 200) + "\n",
		"show_diff":      true,
		"max_diff_bytes": float64(40),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Diff truncated") {
		t.Errorf("expected capped diff notice, got: %s", resultText(result))
	}
}

func TestWriteFileShowDiffSkipsOversizedExistingFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "oversized.txt")
	if err := os.WriteFile(f, []byte(strings.Repeat("a", defaultDiffFileMaxBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := writeFileHandler(nil, newTestRequest(map[string]any{
		"path":      f,
		"content":   "replacement\n",
		"show_diff": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("expected overwrite success with skipped diff note, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Diff skipped") {
		t.Fatalf("expected oversized diff note, got: %q", resultText(result))
	}
}

func TestWriteFileOverwritePreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix permission bits")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(f, []byte("old\n"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f, 0o750); err != nil {
		t.Fatal(err)
	}

	result, err := writeFileHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"content": "new\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	info, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Errorf("mode after overwrite = %o, want 750", got)
	}
}

func TestWriteFileNewFileNoAutoDiff(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "newfile.txt")

	req := newTestRequest(map[string]any{
		"path":    f,
		"content": "brand new content\n",
	})
	result, err := writeFileHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	txt := resultText(result)
	if !strings.Contains(txt, "Created") {
		t.Errorf("expected 'Created' in output for new file, got: %s", txt)
	}
	if strings.Contains(txt, "@@") {
		t.Errorf("expected no diff for new file, got: %s", txt)
	}
}

// ── diff_files ────────────────────────────────────────────────────────────────
// Reason: diffFilesHandler had zero test coverage. It is a dedicated MCP tool
// that LLM clients rely on to compare two files; untested regressions (e.g.
// wrong sign convention, missing header, binary rejection) would be silent.

func TestDiffFilesHandler(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("line1\nOldName\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("line1\nNewName\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{"path_a": a, "path_b": b}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "-OldName") {
		t.Errorf("expected -OldName in diff: %q", text)
	}
	if !strings.Contains(text, "+NewName") {
		t.Errorf("expected +NewName in diff: %q", text)
	}
	if !strings.Contains(text, "@@") {
		t.Errorf("expected @@ hunk header in diff: %q", text)
	}
}

func TestDiffFilesHandlerIdentical(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	content := []byte("same content\n")
	if err := os.WriteFile(a, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, content, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{"path_a": a, "path_b": b}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "identical") {
		t.Errorf("expected 'identical' in result for same-content files: %q", text)
	}
}

func TestDiffFilesHandlerMissingArgs(t *testing.T) {
	// Both paths missing.
	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when both path_a and path_b are absent")
	}

	// Only path_b missing.
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	result2, err := diffFilesHandler(nil, newTestRequest(map[string]any{"path_a": a}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result2) {
		t.Error("expected error when path_b is absent")
	}
}

func TestDiffFilesHandlerBinaryFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.bin")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{"path_a": a, "path_b": b}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when path_a is binary")
	}
}

func TestDiffFilesHandlerContextLines(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	contentA := sb.String()
	contentB := strings.Replace(contentA, "line5\n", "CHANGED\n", 1)
	if err := os.WriteFile(a, []byte(contentA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(contentB), 0o644); err != nil {
		t.Fatal(err)
	}

	// context_lines=0: changed hunk should not include adjacent unchanged lines.
	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{
		"path_a":        a,
		"path_b":        b,
		"context_lines": float64(0),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, " line4") || strings.Contains(text, " line6") {
		t.Errorf("context_lines=0 should not show adjacent context lines: %q", text)
	}
	if !strings.Contains(text, "-line5") || !strings.Contains(text, "+CHANGED") {
		t.Errorf("expected changed lines in diff: %q", text)
	}
}

func TestDiffFilesHandlerMaxDiffBytes(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte(strings.Repeat("a", 200)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(strings.Repeat("b", 200)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{
		"path_a":         a,
		"path_b":         b,
		"max_diff_bytes": float64(40),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "Diff truncated") {
		t.Errorf("expected diff truncation notice: %q", text)
	}
}

func TestDiffFilesHandlerMaxFileBytes(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte(strings.Repeat("a", 20)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{
		"path_a":         a,
		"path_b":         b,
		"max_file_bytes": float64(10),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatalf("expected error for file exceeding max_file_bytes, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "max_file_bytes") {
		t.Errorf("expected max_file_bytes guidance: %q", resultText(result))
	}
}

// ── calculate_checksum ────────────────────────────────────────────────────────
// Reason: calculateChecksumHandler had zero test coverage. It delegates to
// HashFile for each path; we need integration-level tests to verify the handler
// wires algorithm selection correctly, reports per-file errors inline, and
// rejects unsupported algorithm values at the handler boundary.

func TestCalculateChecksumHandlerDefault(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := calculateChecksumHandler(nil, newTestRequest(map[string]any{"paths": []any{f}}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "SHA256") {
		t.Errorf("expected 'SHA256' header in output: %q", text)
	}
	// Known sha256("hello").
	const wantHash = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if !strings.Contains(text, wantHash) {
		t.Errorf("expected sha256 digest in output: %q", text)
	}
}

func TestCalculateChecksumHandlerMD5(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := calculateChecksumHandler(nil, newTestRequest(map[string]any{
		"paths":     []any{f},
		"algorithm": "md5",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "MD5") {
		t.Errorf("expected 'MD5' header in output: %q", text)
	}
	const wantMD5 = "5d41402abc4b2a76b9719d911017c592"
	if !strings.Contains(text, wantMD5) {
		t.Errorf("expected md5 digest in output: %q", text)
	}
}

func TestCalculateChecksumHandlerSHA1(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := calculateChecksumHandler(nil, newTestRequest(map[string]any{
		"paths":     []any{f},
		"algorithm": "sha1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "SHA1") {
		t.Errorf("expected 'SHA1' header in output: %q", text)
	}
	const wantSHA1 = "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	if !strings.Contains(text, wantSHA1) {
		t.Errorf("expected sha1 digest in output: %q", text)
	}
}

func TestCalculateChecksumHandlerMultiplePaths(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fa, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fb, []byte("bbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := calculateChecksumHandler(nil, newTestRequest(map[string]any{
		"paths": []any{fa, fb},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "a.txt") || !strings.Contains(text, "b.txt") {
		t.Errorf("expected both file names in output: %q", text)
	}
}

func TestCalculateChecksumHandlerEmptyPaths(t *testing.T) {
	result, err := calculateChecksumHandler(nil, newTestRequest(map[string]any{
		"paths": []any{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty paths slice")
	}
}

func TestCalculateChecksumHandlerUnsupportedAlgorithm(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := calculateChecksumHandler(nil, newTestRequest(map[string]any{
		"paths":     []any{f},
		"algorithm": "blake2b",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for unsupported algorithm")
	}
}

func TestCalculateChecksumHandlerMissingFile(t *testing.T) {
	// A missing file should NOT make the handler return an MCP error — it should
	// report the per-file ERROR inline and still return a text result, because
	// the handler processes all paths individually.
	result, err := calculateChecksumHandler(nil, newTestRequest(map[string]any{
		"paths": []any{filepath.Join(t.TempDir(), "ghost.txt")},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Error("expected non-error result (per-file ERROR reported inline)")
	}
	text := resultText(result)
	if !strings.Contains(text, "ERROR") {
		t.Errorf("expected per-file ERROR notice in output: %q", text)
	}
}

// ── copy_file overwrite ───────────────────────────────────────────────────────
// Reason: The overwrite=true path was never exercised; only the rejection case
// (overwrite=false with existing dst) was tested, leaving the success branch dark.

func TestCopyFileHandlerOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := copyFileHandler(nil, newTestRequest(map[string]any{
		"source":      src,
		"destination": dst,
		"overwrite":   true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error with overwrite=true: %s", resultText(result))
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "new content" {
		t.Errorf("destination content = %q, want %q", string(data), "new content")
	}
}

func TestCopyFileHandlerRejectsSameFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "same.txt")
	if err := os.WriteFile(f, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := copyFileHandler(nil, newTestRequest(map[string]any{
		"source":      f,
		"destination": f,
		"overwrite":   true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error when copy source and destination are the same file")
	}
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep me" {
		t.Errorf("same-file copy must not alter content, got %q", data)
	}
}

func TestMoveFileHandlerRejectsSameFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "same.txt")
	if err := os.WriteFile(f, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := moveFileHandler(nil, newTestRequest(map[string]any{
		"source":      f,
		"destination": f,
		"overwrite":   true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error when move source and destination are the same file")
	}
	if data, err := os.ReadFile(f); err != nil || string(data) != "keep me" {
		t.Fatalf("same-file move must leave source untouched, data=%q err=%v", data, err)
	}
}

// ── delete_file on nonexistent path ──────────────────────────────────────────
// Reason: Deleting a nonexistent file should return a clear error, not panic.
// This edge-case is trivially reachable in LLM sessions.

func TestDeleteFileNotExist(t *testing.T) {
	result, err := deleteFileHandler(nil, newTestRequest(map[string]any{
		"path": filepath.Join(t.TempDir(), "ghost.txt"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when deleting a nonexistent file")
	}
}

// ── append_to_file creates new file ──────────────────────────────────────────
// Reason: The README states append_to_file "creates [the file] if absent".
// This contract was never verified in tests — a regression here would break a
// core advertised behaviour.

func TestAppendFileCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "new_append.txt")
	result, err := appendFileHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"content": "first line\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	if string(data) != "first line\n" {
		t.Errorf("file content = %q, want %q", string(data), "first line\n")
	}
}

func TestAppendFileEnsureLeadingNewline(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "append_safe.txt")
	if err := os.WriteFile(f, []byte("line1"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := appendFileHandler(nil, newTestRequest(map[string]any{
		"path":                   f,
		"content":                "line2\n",
		"ensure_leading_newline": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	if got, want := string(data), "line1\nline2\n"; got != want {
		t.Fatalf("append with safe newline = %q, want %q", got, want)
	}
}

// ── createSymlinkHandler ─────────────────────────────────────────────────────

func TestCreateSymlinkMissingSource(t *testing.T) {
	result, err := createSymlinkHandler(nil, newTestRequest(map[string]any{
		"link": "some_link",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error for missing source")
	}
}

func TestCreateSymlinkMissingLink(t *testing.T) {
	result, err := createSymlinkHandler(nil, newTestRequest(map[string]any{
		"source": "some_target",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error for missing link")
	}
}

func TestCreateSymlinkSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link.txt")
	result, err := createSymlinkHandler(nil, newTestRequest(map[string]any{
		"source": target,
		"link":   link,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		txt := resultText(result)
		if strings.Contains(txt, "privilege") || strings.Contains(txt, "not permitted") {
			t.Skip("skipping: symlink creation requires elevated privileges on this OS")
		}
		t.Fatalf("unexpected error: %s", txt)
	}

	resolved, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	if resolved != target {
		t.Errorf("symlink target = %q, want %q", resolved, target)
	}
	if !strings.Contains(resultText(result), "Created symlink") {
		t.Errorf("unexpected result text: %s", resultText(result))
	}
}

func TestCreateSymlinkIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link.txt")
	req := newTestRequest(map[string]any{"source": target, "link": link})
	first, err := createSymlinkHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(first) {
		txt := resultText(first)
		if strings.Contains(txt, "privilege") || strings.Contains(txt, "not permitted") {
			t.Skip("skipping: symlink creation requires elevated privileges on this OS")
		}
		t.Fatalf("unexpected error: %s", txt)
	}

	second, err := createSymlinkHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(second) {
		t.Fatalf("second create should be idempotent: %s", resultText(second))
	}
	if !strings.Contains(resultText(second), "already exists") {
		t.Fatalf("expected idempotent status message, got: %q", resultText(second))
	}
}

func TestCreateSymlinkRejectsTargetOutsideLinkDirectory(t *testing.T) {
	dir := t.TempDir()
	linkDir := filepath.Join(dir, "links")
	outsideTarget := filepath.Join(dir, "target.txt")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideTarget, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(linkDir, "link.txt")
	result, err := createSymlinkHandler(nil, newTestRequest(map[string]any{
		"source": "../target.txt",
		"link":   link,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatalf("expected safety error, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "outside") {
		t.Fatalf("expected outside-directory error, got: %s", resultText(result))
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("link should not have been created; stat err=%v", err)
	}
}

// ── compressFilesHandler ─────────────────────────────────────────────────────

func TestCompressFilesMissingPaths(t *testing.T) {
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"output": "out.zip",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error for missing paths")
	}
}

func TestCompressFilesMissingOutput(t *testing.T) {
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths": []any{"a.txt"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error for missing output")
	}
}

func TestCompressFilesZip(t *testing.T) {
	dir := t.TempDir()

	// Create test files.
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte("content of "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	output := filepath.Join(dir, "out.zip")
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": output,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Compressed 2 files and 1 directory") {
		t.Errorf("unexpected result: %s", resultText(result))
	}

	// Verify the archive is a valid zip.
	r, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("invalid zip archive: %v", err)
	}
	r.Close()
}

func TestCompressFilesZipOutputInsideSourceSkipsArchive(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(srcDir, "out.zip")
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": output,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	r, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("invalid zip archive: %v", err)
	}
	defer r.Close()
	for _, entry := range r.File {
		if entry.Name == "src/out.zip" {
			t.Fatalf("archive included itself: entries=%v", r.File)
		}
	}
	var names []string
	for _, entry := range r.File {
		names = append(names, entry.Name)
	}
	if len(names) != 2 || names[0] != "src/" || names[1] != "src/hello.txt" {
		t.Fatalf("unexpected zip entries: %+v", r.File)
	}
}

func TestCompressFilesTarGz(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.tar.gz")
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": output,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Compressed 1 file and 1 directory") {
		t.Errorf("unexpected result: %s", resultText(result))
	}

	// Verify archive exists and is non-empty.
	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("output file missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output archive is empty")
	}
}

func TestCompressFilesTar(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "out.tar")
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": output,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "(tar,") {
		t.Errorf("expected tar format in summary, got: %s", resultText(result))
	}

	f, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tr := tar.NewReader(f)
	var names []string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	if len(names) != 2 || names[0] != "src/" || names[1] != "src/hello.txt" {
		t.Fatalf("unexpected tar entries: %v", names)
	}
}

func TestCompressFilesTarGzOutputInsideSourceSkipsArchive(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(srcDir, "out.tar.gz")
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": output,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	f, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	var names []string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	for _, name := range names {
		if name == "src/out.tar.gz" {
			t.Fatalf("archive included itself: entries=%v", names)
		}
	}
	if len(names) != 2 || names[0] != "src/" || names[1] != "src/hello.txt" {
		t.Fatalf("unexpected tar entries: %v", names)
	}
}

func TestCompressAndExtractZipPreservesEmptyDirectories(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	emptyDir := filepath.Join(srcDir, "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "empty.zip")
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": archivePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected compression error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "2 directories") {
		t.Fatalf("expected directory count in compression summary, got: %q", resultText(result))
	}

	extractDir := filepath.Join(dir, "unzipped")
	result, err = extractArchiveHandler(nil, newTestRequest(map[string]any{
		"archive": archivePath,
		"output":  extractDir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected extract error: %s", resultText(result))
	}
	info, err := os.Stat(filepath.Join(extractDir, "src", "empty"))
	if err != nil {
		t.Fatalf("expected empty directory after extract: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected extracted empty path to be a directory, got mode %s", info.Mode())
	}
}

func TestCompressAndExtractTarGzPreservesEmptyDirectories(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	emptyDir := filepath.Join(srcDir, "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "empty.tar.gz")
	result, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": archivePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected compression error: %s", resultText(result))
	}

	extractDir := filepath.Join(dir, "untarred")
	result, err = extractArchiveHandler(nil, newTestRequest(map[string]any{
		"archive": archivePath,
		"output":  extractDir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected extract error: %s", resultText(result))
	}
	info, err := os.Stat(filepath.Join(extractDir, "src", "empty"))
	if err != nil {
		t.Fatalf("expected empty directory after tar extract: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected extracted empty path to be a directory, got mode %s", info.Mode())
	}
}

// ── extractArchiveHandler ────────────────────────────────────────────────────

func TestExtractArchiveMissingArchive(t *testing.T) {
	result, err := extractArchiveHandler(nil, newTestRequest(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error for missing archive")
	}
}

func TestExtractArchiveZip(t *testing.T) {
	dir := t.TempDir()

	// Create source files and compress them.
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"a.txt": "aaa", "b.txt": "bbb"}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	archivePath := filepath.Join(dir, "test.zip")
	cResult, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": archivePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(cResult) {
		t.Fatalf("compress failed: %s", resultText(cResult))
	}

	// Extract to a fresh directory.
	extractDir := filepath.Join(dir, "extracted")
	result, err := extractArchiveHandler(nil, newTestRequest(map[string]any{
		"archive": archivePath,
		"output":  extractDir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Extracted 2 files") {
		t.Errorf("unexpected result: %s", resultText(result))
	}

	// Verify extracted files exist.
	for name, content := range files {
		// The zip stores paths relative to parent of srcDir, so look under "src/".
		fPath := filepath.Join(extractDir, "src", name)
		data, err := os.ReadFile(fPath)
		if err != nil {
			t.Errorf("missing extracted file %s: %v", name, err)
			continue
		}
		if string(data) != content {
			t.Errorf("file %s content = %q, want %q", name, string(data), content)
		}
	}
}

func TestExtractArchiveRejectsSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "evil.zip")
	zf, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, err := zw.Create("link/pwn.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("owned")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Skipf("symlinks unavailable on this platform/user: %v", err)
	}

	result, err := extractArchiveHandler(nil, newTestRequest(map[string]any{
		"archive": archivePath,
		"output":  dest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatalf("expected extraction error for symlink parent, got: %s", resultText(result))
	}
	if _, err := os.Stat(filepath.Join(outside, "pwn.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped through symlink; outside file stat err=%v", err)
	}
}

func TestExtractArchiveTarGz(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "c.txt"), []byte("ccc"), 0o644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "test.tar.gz")
	cResult, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": archivePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(cResult) {
		t.Fatalf("compress failed: %s", resultText(cResult))
	}

	extractDir := filepath.Join(dir, "extracted")
	result, err := extractArchiveHandler(nil, newTestRequest(map[string]any{
		"archive": archivePath,
		"output":  extractDir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Extracted 1 files") {
		t.Errorf("unexpected result: %s", resultText(result))
	}

	data, err := os.ReadFile(filepath.Join(extractDir, "src", "c.txt"))
	if err != nil {
		t.Fatalf("missing extracted file: %v", err)
	}
	if string(data) != "ccc" {
		t.Errorf("content = %q, want %q", string(data), "ccc")
	}
}

func TestExtractArchiveTar(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "c.txt"), []byte("ccc"), 0o644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "test.tar")
	cResult, err := compressFilesHandler(nil, newTestRequest(map[string]any{
		"paths":  []any{srcDir},
		"output": archivePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(cResult) {
		t.Fatalf("compress failed: %s", resultText(cResult))
	}

	extractDir := filepath.Join(dir, "extracted")
	result, err := extractArchiveHandler(nil, newTestRequest(map[string]any{
		"archive": archivePath,
		"output":  extractDir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "Extracted 1 files") {
		t.Errorf("unexpected result: %s", resultText(result))
	}

	data, err := os.ReadFile(filepath.Join(extractDir, "src", "c.txt"))
	if err != nil {
		t.Fatalf("missing extracted file: %v", err)
	}
	if string(data) != "ccc" {
		t.Errorf("content = %q, want %q", string(data), "ccc")
	}
}

// ── readFile ranges ──────────────────────────────────────────────────────────

func TestReadFileRanges(t *testing.T) {
	// Create a 20-line file.
	dir := t.TempDir()
	p := filepath.Join(dir, "lines.txt")
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := readFileHandler(nil, newTestRequest(map[string]any{
		"path":   p,
		"ranges": []any{[]any{1.0, 3.0}, []any{10.0, 12.0}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(r) {
		t.Fatalf("unexpected error: %s", resultText(r))
	}

	text := resultText(r)
	if !strings.Contains(text, "line 1") {
		t.Errorf("expected 'line 1' in output, got: %s", text)
	}
	if !strings.Contains(text, "line 3") {
		t.Errorf("expected 'line 3' in output, got: %s", text)
	}
	if !strings.Contains(text, "line 10") {
		t.Errorf("expected 'line 10' in output, got: %s", text)
	}
	if !strings.Contains(text, "line 12") {
		t.Errorf("expected 'line 12' in output, got: %s", text)
	}
	// Should NOT contain lines between ranges.
	if strings.Contains(text, "line 5\n") {
		t.Errorf("should not contain line 5, got: %s", text)
	}
}

func TestReadFileRangesWithLineNumbers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lines.txt")
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&sb, "content %d\n", i)
	}
	os.WriteFile(p, []byte(sb.String()), 0644)

	r, _ := readFileHandler(nil, newTestRequest(map[string]any{
		"path":              p,
		"ranges":            []any{[]any{2.0, 4.0}},
		"show_line_numbers": true,
	}))
	text := resultText(r)
	if !strings.Contains(text, "2|") {
		t.Errorf("expected line number prefix, got: %s", text)
	}
}

func TestReadFileRangesMaxBytesTruncationNotice(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lines.txt")
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, "content %02d\n", i)
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := readFileHandler(nil, newTestRequest(map[string]any{
		"path":      p,
		"ranges":    []any{[]any{1.0, 20.0}},
		"max_bytes": float64(50),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(r)
	if !strings.Contains(text, "TRUNCATED") {
		t.Errorf("expected max_bytes truncation notice for multi-range read, got: %s", text)
	}
}

func TestReadFileRangesInvalidPair(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lines.txt")
	os.WriteFile(p, []byte("hello\n"), 0644)

	r, _ := readFileHandler(nil, newTestRequest(map[string]any{
		"path":   p,
		"ranges": []any{[]any{1.0}}, // only one element
	}))
	if !isResultError(r) {
		t.Errorf("expected error for invalid range pair")
	}
}

func TestWriteFileOverwriteOmitsDiffByDefault(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "plain-overwrite.txt")
	if err := os.WriteFile(f, []byte("old value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := writeFileHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"content": "new value\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if strings.Contains(text, "old value") || strings.Contains(text, "@@") {
		t.Fatalf("write_file should omit diffs by default: %q", text)
	}
}

func TestEditFileHandlerRejectsBinary(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := editFileHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"old_str": "a",
		"new_str": "b",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) || !strings.Contains(strings.ToLower(resultText(result)), "binary") {
		t.Fatalf("expected binary-file rejection, got: %q", resultText(result))
	}
}

func TestEditFileHandlerRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(f, []byte(strings.Repeat("x", 64)), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := editFileHandler(nil, newTestRequest(map[string]any{
		"path":          f,
		"old_str":       "x",
		"new_str":       "y",
		"max_file_size": float64(8),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) || !strings.Contains(resultText(result), "max_file_size") {
		t.Fatalf("expected max_file_size rejection, got: %q", resultText(result))
	}
}

func TestCopyFileHandlerDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "srcdir")
	dst := filepath.Join(dir, "dstdir")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := copyFileHandler(nil, newTestRequest(map[string]any{
		"source":      src,
		"destination": dst,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, err := os.ReadFile(filepath.Join(dst, "nested.txt"))
	if err != nil {
		t.Fatalf("copied directory missing file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("copied directory content = %q", data)
	}
}

func TestMoveFileHandlerDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "srcdir")
	dst := filepath.Join(dir, "moveddir")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := moveFileHandler(nil, newTestRequest(map[string]any{
		"source":      src,
		"destination": dst,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source directory should be gone after move, stat err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "nested.txt"))
	if err != nil {
		t.Fatalf("moved directory missing file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("moved directory content = %q", data)
	}
}

func TestEditFileHandlerUTF16LE(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "utf16.txt")
	original := helper.EncodeTextBytes("alpha\r\nbeta\r\n", helper.TextEncodingUTF16LE, true)
	if err := os.WriteFile(f, original, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := editFileHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"old_str": "beta",
		"new_str": "gamma",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	raw, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xFE {
		t.Fatalf("expected UTF-16LE BOM to be preserved, got %v", raw[:min(2, len(raw))])
	}
	if got := helper.DecodeTextBytes(raw).Text; got != "alpha\ngamma\n" {
		t.Fatalf("unexpected edited UTF-16 content: %q", got)
	}
}

func TestDiffFilesHandlerNoNewlineMarker(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("line1\nline2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := diffFilesHandler(nil, newTestRequest(map[string]any{"path_a": a, "path_b": b}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, `\ No newline at end of file`) {
		t.Fatalf("expected EOF newline marker in diff output: %q", text)
	}
}

func TestGetFileInfoAndPathExistsShowSymlinkTargetMetadata(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on this platform/user: %v", err)
	}

	infoResult, err := getFileInfoHandler(nil, newTestRequest(map[string]any{
		"path":        link,
		"output_mode": "details",
	}))
	if err != nil {
		t.Fatal(err)
	}
	infoText := resultText(infoResult)
	if !strings.Contains(infoText, "Target:      file") || !strings.Contains(infoText, "Target Size:") {
		t.Fatalf("expected resolved symlink metadata in get_file_info: %q", infoText)
	}

	existsResult, err := pathExistsHandler(nil, newTestRequest(map[string]any{"path": link}))
	if err != nil {
		t.Fatal(err)
	}
	existsText := resultText(existsResult)
	if !strings.Contains(existsText, "symlink ->") || !strings.Contains(existsText, "target: file") {
		t.Fatalf("expected resolved symlink metadata in path_exists: %q", existsText)
	}
}
