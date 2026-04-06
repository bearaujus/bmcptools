package main

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
	// Should NOT be a global error — partial success is still a text result.
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

	// File must be unchanged.
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
	// Partial failure → result is an error.
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

	// Default (show_hidden=false) — .env should NOT be modified.
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
	// Two-line file ending with newline — should show "2 lines", not 3.
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
	// Should have a header with the file name and line count.
	if !strings.Contains(text, "myfile.txt") {
		t.Errorf("expected filename in header: %q", text)
	}
	if !strings.Contains(text, "3 lines") {
		t.Errorf("expected '3 lines' in header: %q", text)
	}
	// Content should still be present.
	if !strings.Contains(text, "alpha") {
		t.Errorf("expected file content 'alpha': %q", text)
	}
}

// ── edit_file batch missed pattern reporting ──────────────────────────────────

func TestEditFileHandlerBatchMissedPatterns(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "partial.txt")
	if err := os.WriteFile(f, []byte("foo bar"), 0o644); err != nil {
		t.Fatal(err)
	}

	// One pattern matches (foo→FOO), one does not (xyz→ABC).
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
	// Should mention the unmatched pattern.
	if !strings.Contains(text, "xyz") {
		t.Errorf("expected unmatched pattern 'xyz' reported in output: %q", text)
	}
	// File should be updated with the matched pattern.
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
	// Binary file — null byte in first 512 bytes.
	bin := filepath.Join(dir, "data.bin")
	payload := make([]byte, 100)
	payload[10] = 0x00 // null byte
	copy(payload[:4], []byte("old "))
	if err := os.WriteFile(bin, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	// Text file that should be modified.
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
	// Binary file must appear in the "Skipped" list.
	if !strings.Contains(text, "data.bin") || !strings.Contains(text, "binary") {
		t.Errorf("expected data.bin in skipped binary list: %q", text)
	}
	// Text file must be modified.
	data, _ := os.ReadFile(txt)
	if !strings.Contains(string(data), "new") {
		t.Errorf("text file not modified: %q", string(data))
	}
}
