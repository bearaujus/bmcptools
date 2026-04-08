package multi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── read_multiple_files ───────────────────────────────────────────────────────

func TestReadMultipleFilesHandler(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fa, []byte("content of a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fb, []byte("content of b"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa, fb}})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "content of a") {
		t.Errorf("expected content of a: %q", text)
	}
	if !strings.Contains(text, "content of b") {
		t.Errorf("expected content of b: %q", text)
	}
	if !strings.Contains(text, "File 1:") || !strings.Contains(text, "File 2:") {
		t.Errorf("expected file headers: %q", text)
	}
}

func TestReadMultipleFilesHandlerPartialError(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(fa, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa, "/nonexistent/file.txt"}})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "hello") {
		t.Errorf("expected good file content: %q", text)
	}
	if !strings.Contains(text, "[ERROR]") {
		t.Errorf("expected error marker for missing file: %q", text)
	}
}

func TestReadMultipleFilesHandlerEmptyPaths(t *testing.T) {
	req := newTestRequest(map[string]any{"paths": []any{}})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty paths array")
	}
}

// ── find_replace_in_files ─────────────────────────────────────────────────────

func TestFindReplaceInFilesBasic(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.go")
	fb := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fa, []byte("func OldName() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fb, []byte("// calls OldName here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "OldName",
		"new_str": "NewName",
	})
	result, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	dataA, _ := os.ReadFile(fa)
	if strings.Contains(string(dataA), "OldName") {
		t.Errorf("a.go still contains OldName: %q", string(dataA))
	}
	if !strings.Contains(string(dataA), "NewName") {
		t.Errorf("a.go missing NewName: %q", string(dataA))
	}

	dataB, _ := os.ReadFile(fb)
	if strings.Contains(string(dataB), "OldName") {
		t.Errorf("b.go still contains OldName: %q", string(dataB))
	}
}

func TestFindReplaceInFilesDryRun(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "target.txt")
	original := "hello world"
	if err := os.WriteFile(fa, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "hello",
		"new_str": "goodbye",
		"dry_run": true,
	})
	result, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "DRY RUN") {
		t.Errorf("expected DRY RUN in output: %q", text)
	}

	data, _ := os.ReadFile(fa)
	if string(data) != original {
		t.Errorf("dry_run modified file: %q", string(data))
	}
}

func TestFindReplaceInFilesGlob(t *testing.T) {
	dir := t.TempDir()
	fgo := filepath.Join(dir, "code.go")
	ftxt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(fgo, []byte("old code"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ftxt, []byte("old notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "old",
		"new_str": "new",
		"glob":    "*.go",
	})
	_, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}

	dataGo, _ := os.ReadFile(fgo)
	if strings.Contains(string(dataGo), "old") {
		t.Errorf("*.go file not updated: %q", string(dataGo))
	}

	dataTxt, _ := os.ReadFile(ftxt)
	if !strings.Contains(string(dataTxt), "old") {
		t.Errorf("notes.txt should not have been updated: %q", string(dataTxt))
	}
}

func TestFindReplaceInFilesRegex(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "ver.go")
	if err := os.WriteFile(fa, []byte(`version = "1.2.3"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":      dir,
		"old_str":   `"(\d+\.\d+\.\d+)"`,
		"new_str":   `"9.9.9"`,
		"use_regex": true,
	})
	result, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	data, _ := os.ReadFile(fa)
	if !strings.Contains(string(data), "9.9.9") {
		t.Errorf("regex replace failed: %q", string(data))
	}
}

func TestFindReplaceInFilesNoMatch(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(fa, []byte("nothing here"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "NONEXISTENT",
		"new_str": "X",
	})
	result, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "No matches") {
		t.Errorf("expected no-match message: %q", text)
	}
}

// ── write_multiple_files ──────────────────────────────────────────────────────

func TestWriteMultipleFilesHandler(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "sub", "b.txt")

	req := newTestRequest(map[string]any{
		"files": []any{
			map[string]any{"path": fa, "content": "content of a"},
			map[string]any{"path": fb, "content": "content of b"},
		},
	})
	result, err := writeMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	dataA, _ := os.ReadFile(fa)
	if string(dataA) != "content of a" {
		t.Errorf("a.txt content = %q", string(dataA))
	}
	dataB, _ := os.ReadFile(fb)
	if string(dataB) != "content of b" {
		t.Errorf("b.txt content = %q", string(dataB))
	}

	text := resultText(result)
	if !strings.Contains(text, "Wrote 2") {
		t.Errorf("expected 'Wrote 2 files' in result: %q", text)
	}
}

func TestWriteMultipleFilesHandlerPartialError(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "good.txt")

	req := newTestRequest(map[string]any{
		"files": []any{
			map[string]any{"path": fa, "content": "ok"},
			map[string]any{"path": "", "content": "bad"},
		},
	})
	result, err := writeMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error result for partial failure")
	}
	text := resultText(result)
	if !strings.Contains(text, "✓") || !strings.Contains(text, "✗") {
		t.Errorf("expected success and failure markers: %q", text)
	}
}

func TestWriteMultipleFilesHandlerEmpty(t *testing.T) {
	req := newTestRequest(map[string]any{"files": []any{}})
	result, err := writeMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty files array")
	}
}

// ── find_replace_in_files show_hidden ─────────────────────────────────────────

func TestFindReplaceInFilesSkipsHidden(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".env")
	visible := filepath.Join(dir, "code.go")
	if err := os.WriteFile(hidden, []byte("SECRET=old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(visible, []byte("old value"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "old",
		"new_str": "new",
	})
	_, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}

	hiddenData, _ := os.ReadFile(hidden)
	if !strings.Contains(string(hiddenData), "old") {
		t.Errorf("hidden .env should NOT have been modified with show_hidden=false: %q", string(hiddenData))
	}

	visibleData, _ := os.ReadFile(visible)
	if !strings.Contains(string(visibleData), "new") {
		t.Errorf("visible code.go SHOULD have been modified: %q", string(visibleData))
	}
}

func TestFindReplaceInFilesShowHidden(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".env")
	if err := os.WriteFile(hidden, []byte("SECRET=old"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":        dir,
		"old_str":     "old",
		"new_str":     "new",
		"show_hidden": true,
	})
	_, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(hidden)
	if !strings.Contains(string(data), "new") {
		t.Errorf("hidden .env SHOULD have been modified with show_hidden=true: %q", string(data))
	}
}

// ── read_multiple_files line count ────────────────────────────────────────────

func TestReadMultipleFilesLineCountCorrect(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "twolines.txt")
	if err := os.WriteFile(fa, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa}})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "2 lines") {
		t.Errorf("expected '2 lines' in header for 2-line file, got: %q", text)
	}
	if strings.Contains(text, "3 lines") {
		t.Errorf("overcounted lines ('3 lines') in header: %q", text)
	}
}

// ── read_multiple_files summary line ─────────────────────────────────────────

func TestReadMultipleFilesSummarySuccess(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fa, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fb, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa, fb}})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Summary") {
		t.Errorf("expected Summary line at end of read_multiple_files: %q", text)
	}
	if !strings.Contains(text, "2 files") {
		t.Errorf("expected '2 files' in summary: %q", text)
	}
}

func TestReadMultipleFilesSummaryPartialError(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "ok.txt")
	if err := os.WriteFile(fa, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa, "/nonexistent/ghost.txt"}})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Summary") {
		t.Errorf("expected Summary line: %q", text)
	}
	if !strings.Contains(text, "failed") {
		t.Errorf("expected 'failed' count in summary: %q", text)
	}
}

// ── find_replace_in_files binary skip (512-byte sniff) ───────────────────────

func TestFindReplaceInFilesBinarySniff(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "data.bin")
	payload := make([]byte, 100)
	payload[10] = 0x00
	copy(payload[:4], []byte("old "))
	if err := os.WriteFile(bin, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(dir, "code.txt")
	if err := os.WriteFile(txt, []byte("old value"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "old",
		"new_str": "new",
	})
	result, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "data.bin") || !strings.Contains(text, "binary") {
		t.Errorf("expected data.bin in skipped binary list: %q", text)
	}
	data, _ := os.ReadFile(txt)
	if !strings.Contains(string(data), "new") {
		t.Errorf("text file not modified: %q", string(data))
	}
}

// ── find_replace_in_files CRLF ────────────────────────────────────────────────

func TestFindReplaceInFilesCRLF(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "windows.txt")
	crlfContent := "hello world\r\nfoo bar\r\nhello again\r\n"
	if err := os.WriteFile(f, []byte(crlfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "hello",
		"new_str": "hi",
	})
	result, err := findReplaceInFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	data, _ := os.ReadFile(f)
	got := string(data)
	want := "hi world\r\nfoo bar\r\nhi again\r\n"
	if got != want {
		t.Errorf("CRLF find-replace = %q, want %q", got, want)
	}
}

// ── TestReadMultipleFilesHandlerBinaryFile (ported from helpers_test.go) ──────

func TestReadMultipleFilesHandlerBinaryFile(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "text.txt")
	fb := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(fa, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fb, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa, fb}})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "hello") {
		t.Errorf("expected text file content: %q", text)
	}
	if !strings.Contains(text, "[BINARY FILE]") {
		t.Errorf("expected [BINARY FILE] marker for binary file: %q", text)
	}
}

// ── write_multiple_files (additional edge cases) ──────────────────────────────

// Reason: If the "files" array contains a non-object entry (e.g., a string),
// the handler should report an error for that entry without crashing. This
// protects against malformed LLM output.
func TestWriteMultipleFilesInvalidEntryType(t *testing.T) {
	dir := t.TempDir()
	result, err := writeMultipleFilesHandler(nil, newTestRequest(map[string]any{
		"files": []any{
			"not-an-object",
			map[string]any{"path": filepath.Join(dir, "valid.txt"), "content": "ok"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	// The invalid entry causes a partial failure → handler returns error result
	if !isResultError(result) {
		t.Error("expected error result when files array contains non-object entry")
	}
	text := resultText(result)
	if !strings.Contains(text, "[entry 1]") {
		t.Errorf("expected entry error reference in output: %q", text)
	}
}

// Reason: An entry with an empty path should produce a clear per-entry error
// rather than writing to the current directory or panicking.
func TestWriteMultipleFilesEmptyPath(t *testing.T) {
	result, err := writeMultipleFilesHandler(nil, newTestRequest(map[string]any{
		"files": []any{
			map[string]any{"path": "   ", "content": "data"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error result for empty path entry")
	}
}

// Reason: show_diff=true should include a diff section when an existing file
// is overwritten. The diff path was never exercised by existing tests.
func TestWriteMultipleFilesShowDiff(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(f, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := writeMultipleFilesHandler(nil, newTestRequest(map[string]any{
		"files": []any{
			map[string]any{"path": f, "content": "new content"},
		},
		"show_diff": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "@@") {
		t.Errorf("expected unified diff in show_diff=true output: %q", text)
	}
}

// Reason: When all entries succeed the result should be a text result (not
// an error). Verifies the success/failure classification logic.
func TestWriteMultipleFilesAllSucceedReturnsText(t *testing.T) {
	dir := t.TempDir()
	result, err := writeMultipleFilesHandler(nil, newTestRequest(map[string]any{
		"files": []any{
			map[string]any{"path": filepath.Join(dir, "a.txt"), "content": "aaa"},
			map[string]any{"path": filepath.Join(dir, "b.txt"), "content": "bbb"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "2 files of 2 files") && !strings.Contains(text, "Wrote 2") {
		t.Errorf("expected success summary in output: %q", text)
	}
}

// ── find_replace_in_files (additional edge cases) ─────────────────────────────

// Reason: dry_run=true should report what WOULD be replaced without actually
// modifying the file. This path was never covered.
func TestFindReplaceInFilesDryRunNoModify(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "code.txt")
	original := "hello world"
	if err := os.WriteFile(f, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "hello",
		"new_str": "hi",
		"dry_run": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(strings.ToLower(text), "dry run") && !strings.Contains(strings.ToLower(text), "would replace") {
		t.Errorf("expected dry run indicator in output: %q", text)
	}
	// File must NOT have been modified
	data, _ := os.ReadFile(f)
	if string(data) != original {
		t.Errorf("dry_run should not modify files; file content changed to: %q", string(data))
	}
}

// Reason: use_regex=true enables Go regex patterns. This path was never tested.
// A broken regex-replace pipeline would silently replace nothing or crash.
func TestFindReplaceInFilesRegexMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "src.go")
	if err := os.WriteFile(f, []byte("var foo123 = 1\nvar bar456 = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"old_str":   `var \w+ = `,
		"new_str":   "const x = ",
		"use_regex": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), "const x = ") {
		t.Errorf("expected regex replacement in file, got: %q", string(data))
	}
}

// Reason: An invalid regex should be rejected early with a clear error.
func TestFindReplaceInFilesInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"old_str":   "[bad regex",
		"new_str":   "x",
		"use_regex": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for invalid regex in find_replace_in_files")
	}
}

// Reason: When no files match the pattern, the handler should return a
// descriptive text result (not an error). The LLM must be told nothing changed.
func TestFindReplaceInFilesNoMatchFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("no match here"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "XYZZY_NOT_IN_FILE",
		"new_str": "replacement",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("expected text result (not error) when no match: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "No matches") {
		t.Errorf("expected 'No matches' message: %q", text)
	}
}

// Reason: show_diff=false should suppress the diff section in the output,
// reducing noise when bulk-replacing many files.
func TestFindReplaceInFilesShowDiffFalse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"old_str":   "hello",
		"new_str":   "hi",
		"show_diff": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if strings.Contains(text, "@@") {
		t.Errorf("show_diff=false should not include diff hunks: %q", text)
	}
}

// ── read_multiple_files (additional edge cases) ───────────────────────────────

// Reason: max_bytes_per_file limits how much of each file is read. This
// truncation path was never tested; a regression could return unbounded data.
func TestReadMultipleFilesMaxBytesPerFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.txt")
	// Write 100 bytes
	if err := os.WriteFile(f, []byte(strings.Repeat("A", 100)), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := readMultipleFilesHandler(nil, newTestRequest(map[string]any{
		"paths":              []any{f},
		"max_bytes_per_file": float64(10),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	// With a 10-byte limit, we should see a truncation message or at most 10 A's
	fullContent := strings.Repeat("A", 100)
	if strings.Contains(text, fullContent) {
		t.Errorf("expected content to be limited to max_bytes_per_file=10: %q", text)
	}
}

// Reason: Mixing valid and missing files should report per-file errors while
// still returning the content of successful reads — not MCP error.
func TestReadMultipleFilesPartialFailure(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(good, []byte("good content"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "does_not_exist.txt")

	result, err := readMultipleFilesHandler(nil, newTestRequest(map[string]any{
		"paths": []any{good, missing},
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Partial failures still return text (not MCP error)
	text := resultText(result)
	if !strings.Contains(text, "good content") {
		t.Errorf("expected successful file content in output: %q", text)
	}
	if !strings.Contains(text, "ERROR") && !strings.Contains(text, "failed") {
		t.Errorf("expected error indication for missing file: %q", text)
	}
}
