package helper

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
	text, err := ReadOneFileAsText(f, DefaultMaxReadBytes, false)
	if err != nil {
		t.Fatalf("unexpected error reading binary file: %v", err)
	}
	if !strings.Contains(text, "[BINARY FILE]") {
		t.Errorf("expected [BINARY FILE] marker for binary file, got: %q", text)
	}
	if strings.Contains(text, "Base64:\n") {
		t.Errorf("binary reads should omit base64 by default: %q", text)
	}
}

func TestReadOneFileAsTextBinaryFileIncludeBase64(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ReadOneFileAsText(f, DefaultMaxReadBytes, true)
	if err != nil {
		t.Fatalf("unexpected error reading binary file: %v", err)
	}
	if !strings.Contains(text, "Base64:\n") {
		t.Errorf("expected base64 when includeBase64=true, got: %q", text)
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
		{"backreference first only", "name: alpha", `name: (\w+)`, "name=$1", false, "name=alpha", 1, false},
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

// ── ApplyEdit normalized ──────────────────────────────────────────────────────

func TestApplyEditNormalized(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		oldStr     string
		newStr     string
		replaceAll bool
		wantResult string
		wantCount  int
	}{
		{
			name:       "trailing_space_in_file",
			original:   "line1  \nline2\nline3",
			oldStr:     "line1\nline2",
			newStr:     "replaced",
			wantResult: "replaced\nline3",
			wantCount:  1,
		},
		{
			name:       "trailing_space_in_pattern",
			original:   "line1\nline2\nline3",
			oldStr:     "line1  \nline2",
			newStr:     "replaced",
			wantResult: "replaced\nline3",
			wantCount:  1,
		},
		{
			name:       "both_have_trailing_whitespace",
			original:   "line1   \nline2\t\nline3",
			oldStr:     "line1\t\nline2 ",
			newStr:     "replaced",
			wantResult: "replaced\nline3",
			wantCount:  1,
		},
		{
			name:       "leading_whitespace_differs_no_match",
			original:   "  line1\n  line2",
			oldStr:     "line1\nline2",
			newStr:     "replaced",
			wantResult: "  line1\n  line2",
			wantCount:  0,
		},
		{
			name:       "exact_match_still_works",
			original:   "hello world",
			oldStr:     "hello",
			newStr:     "hi",
			wantResult: "hi world",
			wantCount:  1,
		},
		{
			name:       "no_match",
			original:   "line1\nline2",
			oldStr:     "line3\nline4",
			newStr:     "replaced",
			wantResult: "line1\nline2",
			wantCount:  0,
		},
		{
			name:       "replaceAll_normalized",
			original:   "line1  \nline2\nline1  \nline2",
			oldStr:     "line1\nline2",
			newStr:     "X",
			replaceAll: true,
			wantResult: "X\nX",
			wantCount:  2,
		},
		{
			name:       "single_line_trailing_space_in_pattern",
			original:   "hello",
			oldStr:     "hello   ",
			newStr:     "world",
			wantResult: "world",
			wantCount:  1,
		},
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

func TestCountNormalizedMatches(t *testing.T) {
	tests := []struct {
		name    string
		content string
		oldStr  string
		want    int
	}{
		{
			name:    "exact_match_fast_path",
			content: "foo\nbar\nfoo\nbar",
			oldStr:  "foo\nbar",
			want:    2,
		},
		{
			name:    "normalized_single",
			content: "line1  \nline2\nline3",
			oldStr:  "line1\nline2",
			want:    1,
		},
		{
			name:    "normalized_multiple",
			content: "a  \nb\na  \nb",
			oldStr:  "a\nb",
			want:    2,
		},
		{
			name:    "no_match",
			content: "hello\nworld",
			oldStr:  "foo\nbar",
			want:    0,
		},
		{
			name:    "leading_ws_no_match",
			content: "  a\n  b",
			oldStr:  "a\nb",
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountNormalizedMatches(tt.content, tt.oldStr)
			if got != tt.want {
				t.Errorf("CountNormalizedMatches(%q, %q) = %d, want %d", tt.content, tt.oldStr, got, tt.want)
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

// ── HashFile ──────────────────────────────────────────────────────────────────
// Reason: HashFile (checksum.go) had zero test coverage despite being the
// sole implementation behind the calculate_checksum MCP tool. A silent
// regression in any algorithm would be invisible without these tests.

func TestHashFileSHA256(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, size, err := HashFile(f, "sha256")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// sha256("hello") is well-known
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if hash != want {
		t.Errorf("SHA256 = %q, want %q", hash, want)
	}
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
}

func TestHashFileMD5(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, _, err := HashFile(f, "md5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = "5d41402abc4b2a76b9719d911017c592"
	if hash != want {
		t.Errorf("MD5 = %q, want %q", hash, want)
	}
}

func TestHashFileSHA1(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, _, err := HashFile(f, "sha1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	if hash != want {
		t.Errorf("SHA1 = %q, want %q", hash, want)
	}
}

func TestHashFileDeterministic(t *testing.T) {
	// Repeated calls on the same content must produce the same digest.
	dir := t.TempDir()
	f := filepath.Join(dir, "same.txt")
	if err := os.WriteFile(f, []byte("deterministic content"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, _, err1 := HashFile(f, "sha256")
	h2, _, err2 := HashFile(f, "sha256")
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v / %v", err1, err2)
	}
	if h1 != h2 {
		t.Errorf("non-deterministic: first=%q second=%q", h1, h2)
	}
}

func TestHashFileDirectory(t *testing.T) {
	dir := t.TempDir()
	// HashFile must return an error when given a directory path, not silently
	// hash 0 bytes; otherwise calculate_checksum would silently succeed on dirs.
	_, _, err := HashFile(dir, "sha256")
	if err == nil {
		t.Error("expected error when hashing a directory, got nil")
	}
}

func TestHashFileNotExist(t *testing.T) {
	_, _, err := HashFile(filepath.Join(t.TempDir(), "missing.txt"), "sha256")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

// ── CountContentLines ─────────────────────────────────────────────────────────
// Reason: CountContentLines is used by write_file, read_multiple_files, and
// other handlers to report line counts. Incorrect counts would silently mislead
// LLM clients about file sizes.

func TestCountContentLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single no newline", "hello", 1},
		{"single with newline", "hello\n", 1},
		{"two lines", "a\nb\n", 2},
		{"two lines no trailing", "a\nb", 2},
		{"three lines", "a\nb\nc\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountContentLines(tt.input); got != tt.want {
				t.Errorf("CountContentLines(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ── AtomicWriteFile ───────────────────────────────────────────────────────────
// Reason: AtomicWriteFile is the sole code path for all file writes in the
// project; if it regresses (e.g. temp-file not renamed), every write-related
// tool breaks. The test documents the expected idempotent contract.

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "out.txt")

	// Create new file.
	if err := AtomicWriteFile(f, []byte("initial"), 0o644); err != nil {
		t.Fatalf("create: unexpected error: %v", err)
	}
	data, _ := os.ReadFile(f)
	if string(data) != "initial" {
		t.Errorf("after create: got %q, want %q", string(data), "initial")
	}

	// Overwrite existing file.
	if err := AtomicWriteFile(f, []byte("updated"), 0o644); err != nil {
		t.Fatalf("overwrite: unexpected error: %v", err)
	}
	data, _ = os.ReadFile(f)
	if string(data) != "updated" {
		t.Errorf("after overwrite: got %q, want %q", string(data), "updated")
	}
}

// ── CollectFiles ──────────────────────────────────────────────────────────────
// Reason: CollectFiles drives both grep_files and find_replace_in_files; bugs
// (e.g. wrong recursion depth or hidden-file leakage) are expensive to catch
// through integration tests alone.

func TestCollectFilesBasic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := CollectFiles(dir, false, "*.go", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if !strings.HasSuffix(files[0], "a.go") {
		t.Errorf("expected a.go, got %q", files[0])
	}
}

func TestCollectFilesHiddenExcluded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := CollectFiles(dir, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".env") {
			t.Errorf("hidden .env should be excluded without show_hidden: %v", files)
		}
	}
}

func TestCollectFilesHiddenIncluded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := CollectFiles(dir, false, "", true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if strings.HasSuffix(f, ".env") {
			found = true
		}
	}
	if !found {
		t.Error("expected .env with show_hidden=true")
	}
}

func TestCollectFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Non-recursive should not see deep.go.
	filesFlat, err := CollectFiles(dir, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range filesFlat {
		if strings.HasSuffix(f, "deep.go") {
			t.Errorf("non-recursive should not include deep.go: %v", filesFlat)
		}
	}

	// Recursive should see it.
	filesDeep, err := CollectFiles(dir, true, "", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range filesDeep {
		if strings.HasSuffix(f, "deep.go") {
			found = true
		}
	}
	if !found {
		t.Error("recursive collect should include deep.go")
	}
}

func TestCollectFilesExcludePatterns(t *testing.T) {
	dir := t.TempDir()
	skip := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(skip, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skip, "dep.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := CollectFiles(dir, true, "*.go", false, []string{"node_modules"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0], "main.go") {
		t.Fatalf("expected only main.go after pruning node_modules, got %v", files)
	}
}

// ── ApplyReplaceToFile ────────────────────────────────────────────────────────

// Reason: ApplyReplaceToFile with dryRun=true must count replacements and
// return a diff without touching the file. This code path was untested, meaning
// the find_replace dry-run feature had no safety net.
func TestApplyReplaceToFileDryRun(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "target.txt")
	original := "hello world\nhello again\n"
	if err := os.WriteFile(f, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	count, diff, skipped, err := ApplyReplaceToFile(f, "hello", "hi", false, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipped {
		t.Fatal("text file should not be skipped as binary")
	}
	if count != 2 {
		t.Errorf("expected 2 replacements, got %d", count)
	}
	if diff == "" {
		t.Error("expected non-empty diff with produceDiff=true")
	}
	// File must be unchanged
	data, _ := os.ReadFile(f)
	if string(data) != original {
		t.Errorf("dryRun=true must not modify file; content changed to: %q", string(data))
	}
}

// Reason: useRegex=true path of ApplyReplaceToFile was only exercised through
// the handler layer. A direct helper test ensures the regex engine is wired
// correctly and that groups/anchors work.
func TestApplyReplaceToFileRegex(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "code.go")
	if err := os.WriteFile(f, []byte("var x = 1\nvar y = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	count, _, _, err := ApplyReplaceToFile(f, `var \w+ = `, "const z = ", true, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 regex replacements, got %d", count)
	}
	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), "const z = ") {
		t.Errorf("expected regex replacement in file: %q", string(data))
	}
}

func TestApplyReplaceToFilePreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix permission bits")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(f, []byte("echo old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f, 0o755); err != nil {
		t.Fatal(err)
	}

	count, _, skipped, err := ApplyReplaceToFile(f, "old", "new", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipped {
		t.Fatal("text file should not be skipped")
	}
	if count != 1 {
		t.Fatalf("expected 1 replacement, got %d", count)
	}
	info, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("mode after replace = %o, want 755", got)
	}
}

// Reason: When oldStr is not found in the file, count should be 0 and the
// file should be unchanged. This tests the early-exit "no change" path.
func TestApplyReplaceToFileNoMatches(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "nochange.txt")
	content := "no match here"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	count, diff, skipped, err := ApplyReplaceToFile(f, "MISSING_STRING", "replacement", false, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 matches, got %d", count)
	}
	if diff != "" {
		t.Errorf("expected empty diff when no matches: %q", diff)
	}
	if skipped {
		t.Error("file should not be skipped as binary")
	}
}

// Reason: Binary file detection in ApplyReplaceToFile must skip the file
// silently (returned skipped=true) without writing or erroring.
func TestApplyReplaceToFileBinarySkipped(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	// Write null bytes to trigger binary detection
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, skipped, err := ApplyReplaceToFile(f, "x", "y", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skipped {
		t.Error("expected binary file to be skipped")
	}
}

// ── GenerateDiff edge cases ────────────────────────────────────────────────────

// Reason: GenerateDiff(x, x, n) should return "" — identical inputs have no
// diff. This is the most common fast-path and was not directly unit-tested.
func TestGenerateDiffIdenticalReturnsEmpty(t *testing.T) {
	diff := GenerateDiff("same content\n", "same content\n", 3)
	if diff != "" {
		t.Errorf("expected empty diff for identical content, got: %q", diff)
	}
}

// Reason: A pure deletion (no insertions) should produce "-" lines and a
// valid @@ header. This edge case can expose off-by-one errors in hunk building.
func TestGenerateDiffPureDeletion(t *testing.T) {
	original := "keep\nremove\nkeep\n"
	modified := "keep\nkeep\n"
	diff := GenerateDiff(original, modified, 1)
	if diff == "" {
		t.Fatal("expected non-empty diff for deletion")
	}
	if !strings.Contains(diff, "-remove") {
		t.Errorf("expected '-remove' in diff: %q", diff)
	}
	if !strings.Contains(diff, "@@") {
		t.Errorf("expected @@ hunk header in diff: %q", diff)
	}
}

// Reason: A pure insertion at the start of the file tests hunk line number
// calculation (oStart=0 / nStart=1 edge case in the hunk-building loop).
func TestGenerateDiffInsertionAtStart(t *testing.T) {
	original := "existing line\n"
	modified := "new first line\nexisting line\n"
	diff := GenerateDiff(original, modified, 1)
	if diff == "" {
		t.Fatal("expected non-empty diff for insertion at start")
	}
	if !strings.Contains(diff, "+new first line") {
		t.Errorf("expected '+new first line' in diff: %q", diff)
	}
}

// ── MkdirAllClear edge cases ──────────────────────────────────────────────────

// Reason: MkdirAllClear on an already-existing directory should be a no-op.
// Without a test, a refactor that calls os.Remove first would silently break
// directory preservation.
func TestMkdirAllClearAlreadyExisting(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MkdirAllClear(nested, 0o755); err != nil {
		t.Fatalf("MkdirAllClear on existing dir should succeed: %v", err)
	}
	// Existing file must be preserved
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel file missing after MkdirAllClear: %v", err)
	}
	if string(data) != "keep me" {
		t.Errorf("sentinel file content changed: %q", string(data))
	}
}

// Reason: When a path component is already an existing file, MkdirAllClear
// should return a descriptive error rather than letting os.MkdirAll produce
// a cryptic OS error.
func TestMkdirAllClearPathConflictsWithFile(t *testing.T) {
	dir := t.TempDir()
	conflict := filepath.Join(dir, "conflict")
	if err := os.WriteFile(conflict, []byte("I am a file"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := MkdirAllClear(filepath.Join(conflict, "sub"), 0o755)
	if err == nil {
		t.Fatal("expected error when path component is a file")
	}
	// The Windows-specific path-walking bug (cur="/" → "\C:") was fixed by using
	// filepath.Dir traversal; the descriptive message should now fire on all platforms.
	if !strings.Contains(err.Error(), "already exists as a file") {
		t.Errorf("expected 'already exists as a file' in error: %v", err)
	}
}

// ── CopyFileDataN ──────────────────────────────────────────────────────────────

// Reason: CopyFileDataN was never directly tested. It is the backbone of
// copy_file and move_file; verifying byte count and content integrity is critical.
func TestCopyFileDataNCopiesContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	content := "copy me\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := CopyFileDataN(src, dst, 0o644)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != int64(len(content)) {
		t.Errorf("expected %d bytes copied, got %d", len(content), n)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != content {
		t.Errorf("destination content mismatch: %q", string(data))
	}
}

// Reason: Copying from a nonexistent source must return an error — not silently
// create an empty file at the destination.
func TestCopyFileDataNMissingSource(t *testing.T) {
	dir := t.TempDir()
	_, err := CopyFileDataN(filepath.Join(dir, "nope.txt"), filepath.Join(dir, "dst.txt"), 0o644)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}
