package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			got, count, err := applyEdit(tt.original, tt.oldStr, tt.newStr, false, tt.replaceAll)
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
			got, count, err := applyEdit(tt.original, tt.pattern, tt.newStr, true, tt.replaceAll)
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
	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{"path": f}))
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
}

func TestGetFileInfoHandlerLineCount(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := getFileInfoHandler(nil, newTestRequest(map[string]any{"path": f}))
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
	// Binary files should not include a Lines: field.
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
	// Empty file: 0 lines.
	if !strings.Contains(text, "0 lines") {
		t.Errorf("expected '0 lines' for empty file write, got: %q", text)
	}
}

func TestWriteFileLineCount(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "count.txt")
	// "hello\nworld\n" = 2 lines (both end with \n)
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
	// Diff should be included.
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
	// No match — should return success with "not found" message.
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

// ── read_file range headers include total line count ─────────────────────────

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
	// Header must say "lines 1..2 of 5 lines"
	if !strings.Contains(text, "5") {
		t.Errorf("expected total line count (5) in head= header: %q", text)
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
	// Header must contain the total (5)
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
	// Header must say "lines 2..3 of 5 lines"
	if !strings.Contains(text, "5") {
		t.Errorf("expected total line count (5) in range header: %q", text)
	}
	if !strings.Contains(text, "L2") || !strings.Contains(text, "L3") {
		t.Errorf("expected L2 and L3 in range result: %q", text)
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
	// File must NOT be modified.
	data, _ := os.ReadFile(f)
	if string(data) != original {
		t.Errorf("dry_run modified the file: %q", string(data))
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
	// No match → pattern not found message (not an error result).
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
	// File doesn't exist yet — show_diff=true should still succeed (full add).
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
		// overwrite defaults to false
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

// TestWriteReadJSON verifies that JSON content (with quotes, braces, colons) survives
// a write → read round-trip byte-for-byte.
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

// TestWriteReadPowerShell verifies that PowerShell scripts containing heredocs,
// backslashes, dollar signs, and single/double quotes survive write → read intact.
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

// TestWriteReadEmoji verifies that emoji and other multi-byte Unicode codepoints
// survive a write → read round-trip with correct byte counts.
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

// TestAppendReadEmoji verifies that emoji content survives append → read intact.
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
