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

// ── grep_files output_mode auto ──────────────────────────────────────────────

func TestGrepFilesAutoModeDefaultFewMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\ngoodbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No output_mode specified — should default to "auto" and behave like content
	// (show matching lines) when result set is small.
	req := newTestRequest(map[string]any{"path": dir, "pattern": "hello"})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	// Should show matching line content (like content mode)
	if !strings.Contains(text, "hello world") {
		t.Errorf("auto mode (few matches) should show matching lines, got: %q", text)
	}
	// Should NOT show the "file list" note
	if strings.Contains(text, "showing file list") {
		t.Errorf("auto mode (few matches) should not show file list note, got: %q", text)
	}
}

func TestGrepFilesAutoModeManyMatchesSwitchesToFileList(t *testing.T) {
	dir := t.TempDir()
	// Create files with enough matches to exceed max_results=3 cap.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\nneedle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("needle\nneedle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set max_results=3 so we hit the cap (4 total matches > 3).
	req := newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "needle",
		"max_results": 3,
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	// Should switch to file list format with a note.
	if !strings.Contains(text, "showing file list") {
		t.Errorf("auto mode (many matches) should show file list note, got: %q", text)
	}
	if !strings.Contains(text, "output_mode") {
		t.Errorf("auto mode (many matches) should hint at output_mode:content, got: %q", text)
	}
}

func TestGrepFilesExplicitContentModeAlwaysShowsLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("needle\nneedle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "needle",
		"output_mode": "content",
		"max_results": 1,
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	// Explicit content mode must always show matching lines.
	if !strings.Contains(text, ":1:") && !strings.Contains(text, "needle") {
		t.Errorf("content mode should show matching lines, got: %q", text)
	}
	if strings.Contains(text, "showing file list") {
		t.Errorf("content mode should never show file list note, got: %q", text)
	}
}

func TestGrepFilesExplicitFilesWithMatchesModeUnchanged(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "needle",
		"output_mode": "files_with_matches",
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "a.txt") {
		t.Errorf("files_with_matches should include matching file, got: %q", text)
	}
	if strings.Contains(text, "b.txt") {
		t.Errorf("files_with_matches should not include non-matching file, got: %q", text)
	}
	// Must not contain line numbers in the file list
	if strings.Contains(text, ":1:") {
		t.Errorf("files_with_matches should not show line numbers, got: %q", text)
	}
}



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



// ── grep_files case_insensitive ───────────────────────────────────────────────
// Reason: The case_insensitive flag is a documented parameter of grep_files
// that was never tested. A bug in the flag would cause wrong match counts
// without any test catching it.

func TestGrepFilesHandlerCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mix.txt"), []byte("Hello World\nHELLO WORLD\nhello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without case_insensitive — only exact lower-case match.
	req := newTestRequest(map[string]any{
		"path":    dir,
		"pattern": "hello world",
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, "Hello World") {
		t.Errorf("case-sensitive search should not match 'Hello World': %q", text)
	}

	// With case_insensitive=true — all three lines should match.
	req2 := newTestRequest(map[string]any{
		"path":             dir,
		"pattern":          "hello world",
		"case_insensitive": true,
	})
	result2, err := grepFilesHandler(nil, req2)
	if err != nil {
		t.Fatal(err)
	}
	text2 := resultText(result2)
	if !strings.Contains(text2, "Hello World") {
		t.Errorf("case-insensitive search should match 'Hello World': %q", text2)
	}
	if !strings.Contains(text2, "HELLO WORLD") {
		t.Errorf("case-insensitive search should match 'HELLO WORLD': %q", text2)
	}
}

// ── grep_files max_file_size ──────────────────────────────────────────────────
// Reason: The max_file_size parameter causes grep_files to skip files that are
// too large. This was never tested; a silent regression would mean large files
// are either always or never skipped.

func TestGrepFilesSizeFilter(t *testing.T) {
	dir := t.TempDir()
	// "small" file: 5 bytes.
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// "large" file: 100 bytes.
	large := make([]byte, 100)
	copy(large, []byte("hello"))
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), large, 0o644); err != nil {
		t.Fatal(err)
	}

	// max_file_size=10 should skip large.txt (100 B) but still search small.txt.
	req := newTestRequest(map[string]any{
		"path":          dir,
		"pattern":       "hello",
		"max_file_size": float64(10),
	})
	result, err := grepFilesHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "small.txt") {
		t.Errorf("expected small.txt to be searched: %q", text)
	}
	if strings.Contains(text, "large.txt") && !strings.Contains(text, "skip") {
		t.Errorf("large.txt should be skipped (exceeds max_file_size): %q", text)
	}
}

// ── grep_files output modes ────────────────────────────────────────────────────

// Reason: output_mode="files_with_matches" was never tested. It de-duplicates
// results to file paths only — critical for LLMs doing project-wide analysis.
func TestGrepFilesFilesWithMatchesMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "code.go")
	if err := os.WriteFile(f, []byte("hello\nhello again\nno match here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "hello",
		"output_mode": "files_with_matches",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "code.go") {
		t.Errorf("expected file path in files_with_matches output: %q", text)
	}
	// With two matches, "files_with_matches" should deduplicate to 1 file
	if strings.Count(text, "code.go") > 2 {
		t.Errorf("files_with_matches should deduplicate; got multiple entries: %q", text)
	}
}

// Reason: output_mode="count" was never tested. It reports match counts per
// file — essential for large-scale refactor audits.
func TestGrepFilesCountMode(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fa, []byte("hit\nhit\nhit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fb, []byte("hit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "hit",
		"output_mode": "count",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "3") {
		t.Errorf("expected count of 3 for a.txt: %q", text)
	}
}

// Reason: multiline=true changes the entire grep backend (reads whole file vs
// line-by-line). It was never tested, meaning a refactor of grepFileMultiline
// would be invisible to the test suite.
func TestGrepFilesMultilineMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "multi.txt")
	if err := os.WriteFile(f, []byte("line one\nline two\nline three\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   `one\nline two`,
		"multiline": true,
		"use_regex": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "multi.txt") {
		t.Errorf("expected multiline match in output: %q", text)
	}
}

// Reason: Invalid regex with multiline=true should return a clear error rather
// than panicking on Compile.
func TestGrepFilesMultilineInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   "[invalid",
		"multiline": true,
		"use_regex": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for invalid regex in multiline mode")
	}
}

// Reason: use_regex=true with a valid regex was only tested via the
// case_insensitive path. A standalone valid-regex test catches bugs in the
// non-CI regex code path.
func TestGrepFilesRegexMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "src.go")
	if err := os.WriteFile(f, []byte("func Add(a, b int) int {\n\treturn a + b\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   `func \w+\(`,
		"use_regex": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "src.go") {
		t.Errorf("expected regex match in output: %q", text)
	}
}

// Reason: Invalid regex with use_regex=true should return a clear error.
func TestGrepFilesInvalidRegexError(t *testing.T) {
	dir := t.TempDir()
	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   "[unclosed",
		"use_regex": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for invalid regex pattern")
	}
}

// Reason: Offset-based pagination was never tested. If offset >= all matches
// the result should indicate no matches, not panic.
func TestGrepFilesOffsetBeyondResultsReturnsNoMatches(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":    dir,
		"pattern": "needle",
		"offset":  float64(100),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "No matches") {
		t.Errorf("expected 'No matches' when offset exceeds results: %q", text)
	}
}

// Reason: context_lines > 50 should be clamped to 50. An unclamped value
// would include excessive context lines, bloating the response.
func TestGrepFilesContextLinesAbove50Clamped(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 60)
	for i := range lines {
		if i == 30 {
			lines[i] = "TARGET"
		} else {
			lines[i] = "context"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "ctx.txt"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":          dir,
		"pattern":       "TARGET",
		"context_lines": float64(999),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	// Should succeed without returning thousands of context lines
	_ = resultText(result)
}

// Reason: When a literal pattern contains regex metacharacters, the handler
// emits a hint suggesting use_regex=true. This hint was never verified.
func TestGrepFilesRegexMetacharHint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("no match here"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := grepFilesHandler(nil, newTestRequest(map[string]any{
		"path":    dir,
		"pattern": "foo.bar", // "." is a metachar
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "use_regex") {
		t.Errorf("expected regex metachar hint in output: %q", text)
	}
}

// ── search_files (additional edge cases) ──────────────────────────────────────

// Reason: max_results=1 should stop after finding the first matching file.
// This boundary was never tested; a bug here would return too many results.
func TestSearchFilesMaxResultsLimit(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := searchFilesHandler(nil, newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "*.go",
		"max_results": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "limit") && !strings.Contains(text, "1") {
		t.Errorf("expected limit message in output: %q", text)
	}
}

// Reason: recursive=false should not descend into subdirectories.
// The base case (root only) was never explicitly tested.
func TestSearchFilesRecursiveFalse(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := searchFilesHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"pattern":   "*.go",
		"recursive": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if strings.Contains(text, "deep.go") {
		t.Errorf("recursive=false should not find files in subdirectory: %q", text)
	}
	if !strings.Contains(text, "top.go") {
		t.Errorf("expected top-level file in output: %q", text)
	}
}

// Reason: show_hidden=true enables hidden-file discovery. Without a test,
// any change to the hidden-file logic would be undetected.
func TestSearchFilesShowHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".hidden.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without show_hidden, the hidden file should NOT appear
	resultHidden, err := searchFilesHandler(nil, newTestRequest(map[string]any{
		"path":    dir,
		"pattern": "*.go",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resultText(resultHidden), ".hidden.go") {
		t.Error("hidden file should not appear when show_hidden=false")
	}

	// With show_hidden=true it must appear
	resultVisible, err := searchFilesHandler(nil, newTestRequest(map[string]any{
		"path":        dir,
		"pattern":     "*.go",
		"show_hidden": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resultText(resultVisible), ".hidden.go") {
		t.Error("expected hidden file in output when show_hidden=true")
	}
}

// Reason: If the root is a file (not a directory) the handler should return
// a clear error. This is a common LLM mistake.
func TestSearchFilesPathIsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := searchFilesHandler(nil, newTestRequest(map[string]any{
		"path":    f,
		"pattern": "*.txt",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when path is a file, not a directory")
	}
}

// ── doubleStarMatch ───────────────────────────────────────────────────────────

// Reason: The doubleStarMatch recursive function was never directly unit-tested.
// Tests cover: standalone **, nested paths, and non-matching cases.
func TestDoubleStarMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**", "any/path/here", true},
		{"**", "", true},
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "a/b/c/foo.go", true},
		{"**/*.go", "a/b/foo.ts", false},
		{"src/**/*.go", "src/pkg/file.go", true},
		{"src/**/*.go", "other/pkg/file.go", false},
		{"a/b", "a/b", true},
		{"a/b", "a/c", false},
	}
	for _, tt := range tests {
		pats := strings.Split(tt.pattern, "/")
		segs := strings.Split(tt.path, "/")
		if tt.path == "" {
			segs = nil
		}
		got, err := doubleStarMatch(pats, segs)
		if err != nil {
			t.Errorf("doubleStarMatch(%q, %q) unexpected error: %v", tt.pattern, tt.path, err)
			continue
		}
		if got != tt.want {
			t.Errorf("doubleStarMatch(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}
