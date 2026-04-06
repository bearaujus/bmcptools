package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		got := expandAlternation(tt.pattern)
		if len(got) != len(tt.want) {
			t.Errorf("expandAlternation(%q) len=%d, want %d: %v", tt.pattern, len(got), len(tt.want), got)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("expandAlternation(%q)[%d] = %q, want %q", tt.pattern, i, got[i], tt.want[i])
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
	// Verify the typed sentinel is returned so callers can use errors.Is().
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
	// Should NOT show line numbers in files_with_matches mode.
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

	// src/**/*.go should match src/main.go but not root.go
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
		{"*.go", "dir/file.go", true}, // no separator → basename match → file.go matches *.go
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
	// Two separate alternation groups should both be fully expanded.
	got := expandAlternation("{src,lib}/**/*.{ts,tsx}")
	want := []string{
		"src/**/*.ts",
		"src/**/*.tsx",
		"lib/**/*.ts",
		"lib/**/*.tsx",
	}
	if len(got) != len(want) {
		t.Fatalf("expandAlternation multi-group: got %v, want %v", got, want)
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

	// Default: show_hidden=false — .env should NOT appear.
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

	// show_hidden=true — .env SHOULD appear.
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

	// Default (show_hidden=false) — should not find SECRET in .env.
	req := newTestRequest(map[string]any{"path": dir, "pattern": "SECRET"})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	// With show_hidden=false the .env file is skipped entirely — "No matches" is expected.
	if !isResultError(result) && strings.Contains(resultText(result), ".env") {
		t.Errorf("did not expect .env in results without show_hidden: %q", resultText(result))
	}

	// show_hidden=true — should find SECRET.
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

// TestGrepFileContextAdjacentMatches ensures that two matches close together
// each get independent (possibly overlapping) context — the streaming ring-buffer
// implementation must not conflate them.
func TestGrepFileContextAdjacentMatches(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "adj.txt")
	// Lines: 1:a, 2:MATCH, 3:b, 4:MATCH, 5:c
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
	// First match (line 2): before=[line1="a"], after=[line3="b"]
	if matches[0].lineNum != 2 {
		t.Errorf("first match lineNum = %d, want 2", matches[0].lineNum)
	}
	if len(matches[0].context) != 1 || matches[0].context[0] != "a" {
		t.Errorf("first match before-context = %v, want [a]", matches[0].context)
	}
	if len(matches[0].after) != 1 || matches[0].after[0] != "b" {
		t.Errorf("first match after-context = %v, want [b]", matches[0].after)
	}
	// Second match (line 4): before=[line3="b"], after=[line5="c"]
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

// TestGrepFileContextAtFileStart verifies that the before-context is empty
// when a match occurs on the very first line.
func TestGrepFileContextAtFileStart(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "start.txt")
	if err := os.WriteFile(f, []byte("MATCH\nnext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matchFn := func(line string) bool { return strings.Contains(line, "MATCH") }
	matches, err := grepFile(f, matchFn, 2, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if len(matches[0].context) != 0 {
		t.Errorf("before-context for first-line match should be empty, got %v", matches[0].context)
	}
	if len(matches[0].after) != 1 || matches[0].after[0] != "next" {
		t.Errorf("after-context = %v, want [next]", matches[0].after)
	}
}

// TestGrepFileContextAtFileEnd verifies that after-context is empty when the
// match is the last line.
func TestGrepFileContextAtFileEnd(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "end.txt")
	if err := os.WriteFile(f, []byte("prev\nMATCH\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matchFn := func(line string) bool { return strings.Contains(line, "MATCH") }
	matches, err := grepFile(f, matchFn, 2, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if len(matches[0].context) != 1 || matches[0].context[0] != "prev" {
		t.Errorf("before-context = %v, want [prev]", matches[0].context)
	}
	if len(matches[0].after) != 0 {
		t.Errorf("after-context for last-line match should be empty, got %v", matches[0].after)
	}
}

// ── grep_files multiline ──────────────────────────────────────────────────────

func TestGrepFilesMultiline(t *testing.T) {
	dir := t.TempDir()
	// Two-line pattern: function signature split across lines.
	content := "package main\n\nfunc Hello(\n\tname string,\n) string {\n\treturn name\n}\n"
	f := filepath.Join(dir, "multi.go")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   `func Hello\(\n\tname`,
		"use_regex": true,
		"multiline": true,
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "multi.go") {
		t.Errorf("expected multi.go in multiline result: %q", text)
	}
	// Match should be present (with ↵ replacing the newline).
	if !strings.Contains(text, "Hello") {
		t.Errorf("expected matched function in output: %q", text)
	}
}

func TestGrepFilesMultilineLiteralString(t *testing.T) {
	dir := t.TempDir()
	// Literal string that spans a line boundary.
	content := "hello\nworld\n"
	f := filepath.Join(dir, "lit.txt")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// use_regex=false with multiline=true: treats pattern as literal, quotes it.
	req := newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   "hello\nworld",
		"use_regex": false,
		"multiline": true,
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "lit.txt") {
		t.Errorf("expected lit.txt in multiline literal result: %q", text)
	}
}

func TestGrepFilesMultilineNoMatch(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "nomatch.txt")
	if err := os.WriteFile(f, []byte("foo\nbar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   `foo\nXXX`,
		"use_regex": true,
		"multiline": true,
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "No matches") {
		t.Errorf("expected no-match message: %q", text)
	}
}

// ── generateDiff standard unified-diff format ─────────────────────────────────

func TestGenerateDiffHunkHeader(t *testing.T) {
	orig := "A\nB\nC\n"
	mod := "A\nC\n"
	diff := generateDiff(orig, mod, 1)
	// Should contain full @@ -a,b +c,d @@ header.
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
	diff := generateDiff(orig, mod, 1)
	if !strings.Contains(diff, "+X") {
		t.Errorf("expected +X in diff: %q", diff)
	}
	if !strings.Contains(diff, "@@") {
		t.Errorf("expected @@ header: %q", diff)
	}
}

func TestGrepFilesOffset(t *testing.T) {
	dir := t.TempDir()
	// Write a file with 10 lines each containing "hit"
	var content strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&content, "hit line %d\n", i)
	}
	f := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(f, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	// First page: offset=0 max_results=3 → lines 1-3
	req := newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "hit",
		"max_results": float64(3),
		"offset":      float64(0),
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	page1 := resultText(result)
	if !strings.Contains(page1, "hit line 1") {
		t.Errorf("page1 should contain 'hit line 1': %q", page1)
	}
	if strings.Contains(page1, "hit line 4") {
		t.Errorf("page1 should not contain 'hit line 4': %q", page1)
	}

	// Second page: offset=3 max_results=3 → lines 4-6
	req2 := newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "hit",
		"max_results": float64(3),
		"offset":      float64(3),
	})
	result2, err := grepFilesHandler(nil, req2)
	if err != nil {
		t.Fatal(err)
	}
	page2 := resultText(result2)
	if !strings.Contains(page2, "hit line 4") {
		t.Errorf("page2 should contain 'hit line 4': %q", page2)
	}
	if strings.Contains(page2, "hit line 1") {
		t.Errorf("page2 should not contain 'hit line 1': %q", page2)
	}
}

// ── grep_files max_file_size ───────────────────────────────────────────────────

func TestGrepFilesMaxFileSize(t *testing.T) {
	dir := t.TempDir()
	// small file (10 B) — should be searched
	small := filepath.Join(dir, "small.go")
	if err := os.WriteFile(small, []byte("func match() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// large file (>100 B) — should be skipped
	large := filepath.Join(dir, "large.go")
	if err := os.WriteFile(large, []byte("func match() {}\n"+strings.Repeat("// padding\n", 20)), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":          dir,
		"pattern":       "match",
		"max_file_size": float64(30), // only small.go is under 30 bytes
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)

	// small.go should have a match
	if !strings.Contains(text, "small.go") {
		t.Errorf("expected small.go match: %q", text)
	}
	// large.go should be skipped (oversized)
	if strings.Contains(text, "large.go") {
		t.Errorf("large.go should be skipped by max_file_size: %q", text)
	}
	// header should mention oversized skip
	if !strings.Contains(text, "oversized") {
		t.Errorf("expected 'oversized' in header: %q", text)
	}
}
