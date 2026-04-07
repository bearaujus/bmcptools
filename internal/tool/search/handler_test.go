package search

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bearaujus/bmcptools/internal/helper"
)

// ── expandAlternation ────────────────────────────────────────────────────────

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
		got := helper.ExpandAlternation(tt.pattern)
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

// ── matchGlob ────────────────────────────────────────────────────────────────

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
		wantErr bool
	}{
		{"*.go", "main.go", true, false},
		{"*.go", "main.ts", false, false},
		{"*.{ts,tsx}", "app.ts", true, false},
		{"*.{ts,tsx}", "app.tsx", true, false},
		{"*.{ts,tsx}", "app.js", false, false},
		{"**/*.go", "main.go", true, false},
		{"test_*", "test_file.txt", true, false},
		{"test_*", "nottest.txt", false, false},
		{"[invalid", "file.txt", false, true},
	}
	for _, tt := range tests {
		got, err := matchGlobPath(tt.pattern, tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("matchGlobPath(%q, %q) err=%v, wantErr=%v", tt.pattern, tt.name, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("matchGlobPath(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

// ── grepFile ─────────────────────────────────────────────────────────────────

func TestGrepFileBasic(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(f, []byte("line one\nline two hello\nline three\nhello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matchFn := func(line string) bool { return strings.Contains(line, "hello") }
	matches, err := grepFile(f, matchFn, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].lineNum != 2 {
		t.Errorf("first match lineNum = %d, want 2", matches[0].lineNum)
	}
	if matches[1].lineNum != 4 {
		t.Errorf("second match lineNum = %d, want 4", matches[1].lineNum)
	}
}

func TestGrepFileContext(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ctx.txt")
	content := "aaa\nbbb\nccc MATCH ddd\neee\nfff\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	matchFn := func(line string) bool { return strings.Contains(line, "MATCH") }
	matches, err := grepFile(f, matchFn, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if len(matches[0].context) != 1 {
		t.Errorf("context before: got %d, want 1", len(matches[0].context))
	}
	if len(matches[0].after) != 1 {
		t.Errorf("context after: got %d, want 1", len(matches[0].after))
	}
}

func TestGrepFileBinary(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(f, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := grepFile(f, func(string) bool { return true }, 0, 100)
	if err == nil {
		t.Error("expected error for binary file, got nil")
	}
	if !errors.Is(err, errBinaryFile) {
		t.Errorf("expected errBinaryFile sentinel, got: %v", err)
	}
}

func TestGrepFileLimit(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "many.txt")
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("match line\n")
	}
	if err := os.WriteFile(f, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	matches, err := grepFile(f, func(string) bool { return true }, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 5 {
		t.Errorf("expected 5 matches (limit), got %d", len(matches))
	}
}

// ── handler integration tests ─────────────────────────────────────────────────

func TestSearchFilesHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.ts"), []byte("const x = 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"path": dir, "pattern": "*.go"})
	result, err := searchFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "foo.go") {
		t.Errorf("expected foo.go in results: %q", text)
	}
	if strings.Contains(text, "bar.ts") {
		t.Errorf("did not expect bar.ts in results: %q", text)
	}
}

func TestGrepFilesHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\ngoodbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("no match here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"path": dir, "pattern": "hello"})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "hello world") {
		t.Errorf("expected match in results: %q", text)
	}
}

func TestGrepFilesHandlerRegex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nums.txt"), []byte("abc123\nxyz\nabc456\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   `abc\d+`,
		"use_regex": true,
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "abc123") {
		t.Errorf("expected abc123 in results: %q", text)
	}
}

// ── grep_files output_mode ────────────────────────────────────────────────────

func TestGrepFilesOutputModeFilesWithMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("no match\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "hello",
		"output_mode": "files_with_matches",
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "a.txt") {
		t.Errorf("expected a.txt in files_with_matches output: %q", text)
	}
	if strings.Contains(text, "b.txt") {
		t.Errorf("did not expect b.txt in files_with_matches output: %q", text)
	}
	if strings.Contains(text, ":1:") {
		t.Errorf("did not expect line numbers in files_with_matches mode: %q", text)
	}
}

func TestGrepFilesOutputModeCount(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nhello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "hello",
		"output_mode": "count",
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "2 matches") {
		t.Errorf("expected '2 matches' in count output: %q", text)
	}
}

// ── grep_files glob filter ────────────────────────────────────────────────────

func TestGrepFilesGlobFilter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "match.go"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"pattern": "hello",
		"glob":    "*.go",
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "match.go") {
		t.Errorf("expected match.go in glob-filtered grep: %q", text)
	}
	if strings.Contains(text, "skip.txt") {
		t.Errorf("skip.txt should be excluded by glob filter: %q", text)
	}
}

// ── search_files path-based glob ─────────────────────────────────────────────

func TestSearchFilesPathGlob(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":    dir,
		"pattern": "src/**/*.go",
	})
	result, err := searchFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "main.go") {
		t.Errorf("expected main.go in src/**/*.go result: %q", text)
	}
	if strings.Contains(text, "root.go") {
		t.Errorf("root.go should not match src/**/*.go: %q", text)
	}
}

// ── matchGlobPath additional cases ───────────────────────────────────────────

func TestMatchGlobPathDeep(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"src/**/*.go", "src/main.go", true},
		{"src/**/*.go", "src/pkg/foo.go", true},
		{"src/**/*.go", "src/a/b/c/deep.go", true},
		{"src/**/*.go", "other/main.go", false},
		{"src/**/*.go", "main.go", false},
		{"**/*.go", "any/path/file.go", true},
		{"**/*.go", "file.go", true},
		{"*.go", "file.go", true},
		{"*.go", "dir/file.go", true},
	}
	for _, tt := range tests {
		got, err := matchGlobPath(tt.pattern, tt.path)
		if err != nil {
			t.Errorf("matchGlobPath(%q, %q) unexpected error: %v", tt.pattern, tt.path, err)
			continue
		}
		if got != tt.want {
			t.Errorf("matchGlobPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

// ── expandAlternation multi-group ─────────────────────────────────────────────

func TestExpandAlternationMultiGroup(t *testing.T) {
	got := helper.ExpandAlternation("{src,lib}/**/*.{ts,tsx}")
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

// ── search_files show_hidden ──────────────────────────────────────────────────

func TestSearchFilesShowHidden(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"path": dir, "pattern": "*"})
	result, err := searchFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, ".env") {
		t.Errorf("did not expect .env with show_hidden=false: %q", text)
	}
	if !strings.Contains(text, "visible.txt") {
		t.Errorf("expected visible.txt in result: %q", text)
	}

	req2 := newTestRequest(map[string]any{"path": dir, "pattern": "*", "show_hidden": true})
	result2, err := searchFilesHandler(nil, req2)
	if err != nil {
		t.Fatal(err)
	}
	text2 := resultText(result2)
	if !strings.Contains(text2, ".env") {
		t.Errorf("expected .env with show_hidden=true: %q", text2)
	}
}

// ── grep_files show_hidden ────────────────────────────────────────────────────

func TestGrepFilesShowHidden(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("nothing"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"path": dir, "pattern": "SECRET"})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) && strings.Contains(resultText(result), ".env") {
		t.Errorf("did not expect .env in results without show_hidden: %q", resultText(result))
	}

	req2 := newTestRequest(map[string]any{"path": dir, "pattern": "SECRET", "show_hidden": true})
	result2, err := grepFilesHandler(nil, req2)
	if err != nil {
		t.Fatal(err)
	}
	text2 := resultText(result2)
	if !strings.Contains(text2, "SECRET") {
		t.Errorf("expected match in hidden file with show_hidden=true: %q", text2)
	}
}

// ── grepFile streaming context (ring buffer) ──────────────────────────────────

func TestGrepFileContextAdjacentMatches(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "adj.txt")
	content := "a\nMATCH\nb\nMATCH\nc\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	matchFn := func(line string) bool { return strings.Contains(line, "MATCH") }
	matches, err := grepFile(f, matchFn, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].lineNum != 2 {
		t.Errorf("first match lineNum = %d, want 2", matches[0].lineNum)
	}
	if len(matches[0].context) != 1 || matches[0].context[0] != "a" {
		t.Errorf("first match before-context = %v, want [a]", matches[0].context)
	}
	if len(matches[0].after) != 1 || matches[0].after[0] != "b" {
		t.Errorf("first match after-context = %v, want [b]", matches[0].after)
	}
	if matches[1].lineNum != 4 {
		t.Errorf("second match lineNum = %d, want 4", matches[1].lineNum)
	}
	if len(matches[1].context) != 1 || matches[1].context[0] != "b" {
		t.Errorf("second match before-context = %v, want [b]", matches[1].context)
	}
	if len(matches[1].after) != 1 || matches[1].after[0] != "c" {
		t.Errorf("second match after-context = %v, want [c]", matches[1].after)
	}
}


