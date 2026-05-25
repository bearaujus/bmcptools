package multi

import (
	"os"
	"path/filepath"
	"runtime"
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
	if !strings.Contains(text, fa) || !strings.Contains(text, fb) {
		t.Errorf("expected compact file headers with paths: %q", text)
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

func TestFindReplaceInFilesSkipsOversizedByDefault(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, []byte("old "+strings.Repeat("x", defaultFindReplaceMaxFileSize)), 0o644); err != nil {
		t.Fatal(err)
	}
	small := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(small, []byte("old value"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "old",
		"new_str": "new",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "large.txt") || !strings.Contains(text, "max_file_size") {
		t.Errorf("expected oversized skip notice: %q", text)
	}
	largeData, _ := os.ReadFile(large)
	if !strings.HasPrefix(string(largeData), "old ") {
		t.Errorf("oversized file should not be modified")
	}
	smallData, _ := os.ReadFile(small)
	if !strings.Contains(string(smallData), "new value") {
		t.Errorf("small file should be modified: %q", string(smallData))
	}
}

func TestFindReplaceInFilesMaxFileSizeZeroAllowsLarge(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, []byte("old "+strings.Repeat("x", 1024)), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":          dir,
		"old_str":       "old",
		"new_str":       "new",
		"max_file_size": float64(0),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, _ := os.ReadFile(large)
	if !strings.HasPrefix(string(data), "new ") {
		t.Errorf("max_file_size=0 should allow large file replacement")
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
	if strings.Contains(text, "Base64:\n") {
		t.Errorf("binary file should not include base64 by default: %q", text)
	}
}

func TestReadMultipleFilesHandlerBinaryIncludeBase64(t *testing.T) {
	dir := t.TempDir()
	fb := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(fb, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"paths":          []any{fb},
		"include_base64": true,
	})
	result, err := readMultipleFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Base64:\n") {
		t.Errorf("expected base64 when include_base64=true: %q", text)
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

func TestWriteMultipleFilesPreservesMode(t *testing.T) {
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

	result, err := writeMultipleFilesHandler(nil, newTestRequest(map[string]any{
		"files": []any{
			map[string]any{"path": f, "content": "new\n"},
		},
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
		t.Errorf("mode after write_multiple_files = %o, want 750", got)
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

func TestFindReplaceInFilesExcludePatterns(t *testing.T) {
	dir := t.TempDir()
	skip := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(skip, 0o755); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(skip, "dep.txt")
	if err := os.WriteFile(dep, []byte("old dependency"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "app.txt")
	if err := os.WriteFile(app, []byte("old app"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":             dir,
		"old_str":          "old",
		"new_str":          "new",
		"exclude_patterns": []any{"node_modules"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	depData, _ := os.ReadFile(dep)
	if strings.Contains(string(depData), "new") {
		t.Errorf("excluded dependency file should not be modified: %q", depData)
	}
	appData, _ := os.ReadFile(app)
	if !strings.Contains(string(appData), "new app") {
		t.Errorf("non-excluded app file should be modified: %q", appData)
	}
}

func TestFindReplaceInFilesSuppressesUnmodifiedByDefault(t *testing.T) {
	dir := t.TempDir()
	match := filepath.Join(dir, "match.txt")
	untouched := filepath.Join(dir, "untouched.txt")
	if err := os.WriteFile(match, []byte("replace me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(untouched, []byte("nothing here"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":    dir,
		"old_str": "replace",
		"new_str": "updated",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "No match in 1 file") {
		t.Errorf("expected unmodified count in output: %q", text)
	}
	if strings.Contains(text, "untouched.txt") {
		t.Errorf("unmodified path should be suppressed by default: %q", text)
	}
}

func TestFindReplaceInFilesShowUnmodified(t *testing.T) {
	dir := t.TempDir()
	match := filepath.Join(dir, "match.txt")
	untouched := filepath.Join(dir, "untouched.txt")
	if err := os.WriteFile(match, []byte("replace me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(untouched, []byte("nothing here"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := findReplaceInFilesHandler(nil, newTestRequest(map[string]any{
		"path":            dir,
		"old_str":         "replace",
		"new_str":         "updated",
		"show_unmodified": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "untouched.txt") {
		t.Errorf("expected unmodified path when show_unmodified=true: %q", text)
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

// ── path_exists_batch ─────────────────────────────────────────────────────────

func TestPathExistsBatchHandler(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(fa, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.txt")

	req := newTestRequest(map[string]any{"paths": []any{fa, subDir, missing}})
	result, err := pathExistsBatchHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "file") {
		t.Errorf("expected 'file' for existing file: %q", text)
	}
	if !strings.Contains(text, "directory") {
		t.Errorf("expected 'directory' for subdir: %q", text)
	}
	if !strings.Contains(text, "false") {
		t.Errorf("expected 'false' for missing path: %q", text)
	}
}

func TestPathExistsBatchHandlerEmptyPaths(t *testing.T) {
	req := newTestRequest(map[string]any{"paths": []any{}})
	result, err := pathExistsBatchHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty paths array")
	}
}

func TestPathExistsBatchHandlerLimit(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := pathExistsBatchHandler(nil, newTestRequest(map[string]any{
		"paths": []any{a, b},
		"limit": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "checked 1 of 2") || !strings.Contains(text, "Output truncated") {
		t.Errorf("expected limit summary and truncation notice: %q", text)
	}
}

// ── get_multiple_file_info ────────────────────────────────────────────────────

func TestGetMultipleFileInfoHandler(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "info.txt")
	if err := os.WriteFile(fa, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa}})
	result, err := getMultipleFileInfoHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if strings.Contains(text, "Path:") {
		t.Errorf("default output should be compact, got expanded metadata: %q", text)
	}
	if !strings.Contains(text, "file") {
		t.Errorf("expected 'file' type: %q", text)
	}
	if !strings.Contains(text, "modified") {
		t.Errorf("expected compact modified timestamp: %q", text)
	}
	if !strings.Contains(text, "2 lines") {
		t.Errorf("expected '2 lines' for text file: %q", text)
	}
}

func TestGetMultipleFileInfoHandlerDetailsMode(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "info.txt")
	if err := os.WriteFile(fa, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := getMultipleFileInfoHandler(nil, newTestRequest(map[string]any{
		"paths":       []any{fa},
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

func TestGetMultipleFileInfoHandlerDirectory(t *testing.T) {
	dir := t.TempDir()

	req := newTestRequest(map[string]any{"paths": []any{dir}})
	result, err := getMultipleFileInfoHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "directory") {
		t.Errorf("expected 'Type: directory': %q", text)
	}
}

func TestGetMultipleFileInfoHandlerCountLinesFalse(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "info.txt")
	if err := os.WriteFile(f, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := getMultipleFileInfoHandler(nil, newTestRequest(map[string]any{
		"paths":       []any{f},
		"count_lines": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, "lines") {
		t.Errorf("expected line count to be omitted when count_lines=false: %q", text)
	}
}

func TestGetMultipleFileInfoHandlerMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "ghost.txt")

	req := newTestRequest(map[string]any{"paths": []any{missing}})
	result, err := getMultipleFileInfoHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error result: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "ERROR") {
		t.Errorf("expected inline ERROR for missing path: %q", text)
	}
}

func TestGetMultipleFileInfoHandlerEmptyPaths(t *testing.T) {
	req := newTestRequest(map[string]any{"paths": []any{}})
	result, err := getMultipleFileInfoHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty paths array")
	}
}

func TestGetMultipleFileInfoHandlerMultiple(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fa, []byte("aaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fb, []byte("bbb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"paths": []any{fa, fb}})
	result, err := getMultipleFileInfoHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, fa) || !strings.Contains(text, fb) {
		t.Errorf("expected both compact metadata entries: %q", text)
	}
}

func TestGetMultipleFileInfoHandlerLimit(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := getMultipleFileInfoHandler(nil, newTestRequest(map[string]any{
		"paths": []any{a, b},
		"limit": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, a) || strings.Contains(text, b) {
		t.Errorf("expected only first compact metadata entry after limit, got: %q", text)
	}
	if !strings.Contains(text, "1/2 shown") || !strings.Contains(text, "Output truncated") {
		t.Errorf("expected limit summary and truncation notice: %q", text)
	}
}
