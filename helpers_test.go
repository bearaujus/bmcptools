package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{int64(1.5 * 1024 * 1024), "1.5 MB"},
	}
	for _, tt := range tests {
		if got := humanizeBytes(tt.input); got != tt.want {
			t.Errorf("humanizeBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripBOM(t *testing.T) {
	utf8BOM := []byte{0xEF, 0xBB, 0xBF}
	utf16BE := []byte{0xFE, 0xFF}
	utf16LE := []byte{0xFF, 0xFE}
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{"no bom", []byte("hello"), []byte("hello")},
		{"utf-8 bom", append(utf8BOM, []byte("hello")...), []byte("hello")},
		{"utf-16 be bom", append(utf16BE, []byte("hello")...), []byte("hello")},
		{"utf-16 le bom", append(utf16LE, []byte("hello")...), []byte("hello")},
		{"empty", []byte{}, []byte{}},
		{"only utf-8 bom", utf8BOM, []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripBOM(tt.input); !bytes.Equal(got, tt.want) {
				t.Errorf("stripBOM(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name        string
		header      []byte
		contentType string
		want        bool
	}{
		{"null byte", []byte{0x00, 0x01}, "application/octet-stream", true},
		{"text/plain", []byte("hello world"), "text/plain; charset=utf-8", false},
		{"application/json", []byte(`{"k":"v"}`), "application/json", false},
		{"octet-stream no null", []byte("hello"), "application/octet-stream", true},
		{"application/javascript", []byte("function(){}"), "application/javascript", false},
		{"application/x-yaml", []byte("key: value"), "application/x-yaml", false},
		{"application/xml", []byte("<root/>"), "application/xml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinaryContent(tt.header, tt.contentType); got != tt.want {
				t.Errorf("isBinaryContent(%v, %q) = %v, want %v", tt.header, tt.contentType, got, tt.want)
			}
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		n    int
		word string
		want string
	}{
		// regular
		{1, "file", "1 file"},
		{0, "file", "0 files"},
		{2, "file", "2 files"},
		{5, "result", "5 results"},
		{3, "occurrence", "3 occurrences"},
		// -ch / -sh endings
		{2, "match", "2 matches"},
		{3, "branch", "3 branches"},
		// -y with consonant preceding
		{0, "directory", "0 directories"},
		{2, "directory", "2 directories"},
		{2, "library", "2 libraries"},
		// -y with vowel preceding (keep s)
		{2, "day", "2 days"},
		{2, "boy", "2 boys"},
		// -x ending
		{2, "box", "2 boxes"},
		// singular unchanged
		{1, "match", "1 match"},
		{1, "directory", "1 directory"},
	}
	for _, tt := range tests {
		if got := pluralize(tt.n, tt.word); got != tt.want {
			t.Errorf("pluralize(%d, %q) = %q, want %q", tt.n, tt.word, got, tt.want)
		}
	}
}

// ── generateDiff ──────────────────────────────────────────────────────────────

func TestGenerateDiffIdentical(t *testing.T) {
	result := generateDiff("hello\nworld\n", "hello\nworld\n", 2)
	if result != "" {
		t.Errorf("expected empty diff for identical strings, got %q", result)
	}
}

func TestGenerateDiffSimpleReplace(t *testing.T) {
	orig := "line1\nOldName\nline3\n"
	mod := "line1\nNewName\nline3\n"
	diff := generateDiff(orig, mod, 1)
	if !strings.Contains(diff, "-OldName") {
		t.Errorf("expected -OldName in diff: %q", diff)
	}
	if !strings.Contains(diff, "+NewName") {
		t.Errorf("expected +NewName in diff: %q", diff)
	}
}

func TestGenerateDiffLargeFile(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 600; i++ {
		sb.WriteString("line\n")
	}
	orig := sb.String()
	mod := strings.Replace(orig, "line\n", "changed\n", 1)
	result := generateDiff(orig, mod, 2)
	// New Myers-based diff has no line-count limit — it should produce an actual diff.
	if result == "" {
		t.Errorf("expected non-empty diff for 600-line file: got empty string")
	}
	if !strings.Contains(result, "-line") {
		t.Errorf("expected deleted line in diff: %q", result)
	}
	if !strings.Contains(result, "+changed") {
		t.Errorf("expected added line in diff: %q", result)
	}
}

// ── readOneFileAsText binary detection ────────────────────────────────────────

func TestReadOneFileAsTextBinaryFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	// Write a file with null bytes (binary indicator).
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := readOneFileAsText(f, defaultMaxReadBytes)
	if err != nil {
		t.Fatalf("unexpected error reading binary file: %v", err)
	}
	// Should contain the [BINARY FILE] marker, not garbage bytes.
	if !strings.Contains(text, "[BINARY FILE]") {
		t.Errorf("expected [BINARY FILE] marker for binary file, got: %q", text)
	}
}

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
