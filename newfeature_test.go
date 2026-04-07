package main

import (
	"os"
	"strings"
	"testing"
)

func TestAtomicWriteFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.txt"
	if err := atomicWriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func TestMkdirAllClearFileBlocking(t *testing.T) {
	tmp := t.TempDir()
	blocker := tmp + "/blocker"
	os.WriteFile(blocker, []byte("x"), 0o644)
	err := mkdirAllClear(blocker+"/subdir", 0o755)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists as a file") {
		t.Errorf("expected 'already exists as a file' in error, got: %v", err)
	}
}

func TestRunCommandRawOutput(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":    "echo hello",
		"raw_output": true,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v %v", err, resultText(result))
	}
	text := resultText(result)
	if strings.Contains(text, "$ echo hello") {
		t.Errorf("raw_output=true should omit metadata header, but got:\n%s", text)
	}
	if !strings.Contains(text, "hello") {
		t.Errorf("expected 'hello' in output, got:\n%s", text)
	}
}

func TestFindReplaceReportsUnmodified(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(tmp+"/a.txt", []byte("hello world"), 0o644)
	os.WriteFile(tmp+"/b.txt", []byte("no match here"), 0o644)

	req := newTestRequest(map[string]any{
		"path":    tmp,
		"old_str": "hello",
		"new_str": "hi",
	})
	result, err := findReplaceInFilesHandler(nil, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "No match in") {
		t.Errorf("expected 'No match in' section, got:\n%s", text)
	}
	if !strings.Contains(text, "b.txt") {
		t.Errorf("expected b.txt in unmatched list, got:\n%s", text)
	}
}

func TestWriteMultipleFilesShowDiff(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/out.txt"
	os.WriteFile(path, []byte("line 1\nline 2\n"), 0o644)

	req := newTestRequest(map[string]any{
		"files": []any{
			map[string]any{"path": path, "content": "line 1\nline 2 changed\n"},
		},
		"show_diff": true,
	})
	result, err := writeMultipleFilesHandler(nil, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected: %v", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "line 2 changed") {
		t.Errorf("expected diff with changed line, got:\n%s", text)
	}
}
