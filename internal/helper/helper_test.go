package helper

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
		if got := HumanizeBytes(tt.input); got != tt.want {
			t.Errorf("HumanizeBytes(%d) = %q, want %q", tt.input, got, tt.want)
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
			if got := StripBOM(tt.input); !bytes.Equal(got, tt.want) {
				t.Errorf("StripBOM(%v) = %v, want %v", tt.input, got, tt.want)
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
			if got := IsBinaryContent(tt.header, tt.contentType); got != tt.want {
				t.Errorf("IsBinaryContent(%v, %q) = %v, want %v", tt.header, tt.contentType, got, tt.want)
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
		if got := Pluralize(tt.n, tt.word); got != tt.want {
			t.Errorf("Pluralize(%d, %q) = %q, want %q", tt.n, tt.word, got, tt.want)
		}
	}
}

// ── GenerateDiff ──────────────────────────────────────────────────────────────

func TestGenerateDiffIdentical(t *testing.T) {
	result := GenerateDiff("hello\nworld\n", "hello\nworld\n", 2)
	if result != "" {
		t.Errorf("expected empty diff for identical strings, got %q", result)
	}
}

func TestGenerateDiffSimpleReplace(t *testing.T) {
	orig := "line1\nOldName\nline3\n"
	mod := "line1\nNewName\nline3\n"
	diff := GenerateDiff(orig, mod, 1)
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
	result := GenerateDiff(orig, mod, 2)
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

// ── ReadOneFileAsText binary detection ────────────────────────────────────────

func TestReadOneFileAsTextBinaryFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ReadOneFileAsText(f, DefaultMaxReadBytes)
	if err != nil {
		t.Fatalf("unexpected error reading binary file: %v", err)
	}
	if !strings.Contains(text, "[BINARY FILE]") {
		t.Errorf("expected [BINARY FILE] marker for binary file, got: %q", text)
	}
}

// ── NormalizeCRLF / RestoreCRLF ───────────────────────────────────────────────

func TestNormalizeCRLF(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantOut     string
		wantHasCRLF bool
	}{
		{"no CRLF", "line one\nline two\n", "line one\nline two\n", false},
		{"all CRLF", "line one\r\nline two\r\n", "line one\nline two\n", true},
		{"mixed", "line one\r\nline two\n", "line one\nline two\n", true},
		{"empty", "", "", false},
		{"lone CR", "foo\rbar", "foo\rbar", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hasCRLF := NormalizeCRLF(tt.input)
			if got != tt.wantOut {
				t.Errorf("normalized = %q, want %q", got, tt.wantOut)
			}
			if hasCRLF != tt.wantHasCRLF {
				t.Errorf("hasCRLF = %v, want %v", hasCRLF, tt.wantHasCRLF)
			}
		})
	}
}

func TestRestoreCRLF(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"basic", "line one\nline two\n", "line one\r\nline two\r\n"},
		{"empty", "", ""},
		{"already has CRLF", "foo\r\nbar\n", "foo\r\r\nbar\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RestoreCRLF(tt.input); got != tt.want {
				t.Errorf("RestoreCRLF(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── ApplyEdit ─────────────────────────────────────────────────────────────────

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
			got, count, err := ApplyEdit(tt.original, tt.oldStr, tt.newStr, false, tt.replaceAll)
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
			got, count, err := ApplyEdit(tt.original, tt.pattern, tt.newStr, true, tt.replaceAll)
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

// ── ExpandAlternation ─────────────────────────────────────────────────────────

func TestExpandAlternation(t *testing.T) {
	tests := []struct {
		pattern string
		want    []string
	}{
		{"*.go", []string{"*.go"}},
		{"*.{ts,tsx}", []string{"*.ts", "*.tsx"}},
		{"{a,b,c}.txt", []string{"a.txt", "b.txt", "c.txt"}},
		{"no-braces", []string{"no-braces"}},
		{"src/**/*.{ js , ts }", []string{"src/**/*.js", "src/**/*.ts"}},
	}
	for _, tt := range tests {
		got := ExpandAlternation(tt.pattern)
		if len(got) != len(tt.want) {
			t.Errorf("ExpandAlternation(%q) len=%d, want %d: %v", tt.pattern, len(got), len(tt.want), got)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ExpandAlternation(%q)[%d] = %q, want %q", tt.pattern, i, got[i], tt.want[i])
			}
		}
	}
}

func TestExpandAlternationMultiGroup(t *testing.T) {
	got := ExpandAlternation("{src,lib}/**/*.{ts,tsx}")
	want := []string{
		"src/**/*.ts",
		"src/**/*.tsx",
		"lib/**/*.ts",
		"lib/**/*.tsx",
	}
	if len(got) != len(want) {
		t.Fatalf("ExpandAlternation multi-group: got %v, want %v", got, want)
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("[%d] got %q, want %q", i, g, want[i])
		}
	}
}

// ── GenerateDiff hunk headers ─────────────────────────────────────────────────

func TestGenerateDiffHunkHeader(t *testing.T) {
	orig := "A\nB\nC\n"
	mod := "A\nC\n"
	diff := GenerateDiff(orig, mod, 1)
	if !strings.Contains(diff, "@@") {
		t.Errorf("expected @@ hunk header in diff: %q", diff)
	}
	if !strings.Contains(diff, ",") {
		t.Errorf("expected comma in hunk header (standard -a,b +c,d format): %q", diff)
	}
	if !strings.Contains(diff, "-B") {
		t.Errorf("expected deleted line -B: %q", diff)
	}
}

func TestGenerateDiffInsertionHeader(t *testing.T) {
	orig := "A\nB\n"
	mod := "A\nX\nB\n"
	diff := GenerateDiff(orig, mod, 1)
	if !strings.Contains(diff, "+X") {
		t.Errorf("expected +X in diff: %q", diff)
	}
	if !strings.Contains(diff, "@@") {
		t.Errorf("expected @@ header: %q", diff)
	}
}
