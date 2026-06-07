package dir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── list_directory ───────────────────────────────────────────────────────────

func TestListDirHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := listDirHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "file.txt") {
		t.Errorf("expected file.txt in listing: %q", text)
	}
	if !strings.Contains(text, "subdir") {
		t.Errorf("expected subdir in listing: %q", text)
	}
}

func TestListDirHandlerHidden(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without show_hidden — should not see .hidden.
	result, err := listDirHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, ".hidden") {
		t.Errorf("did not expect .hidden in default listing: %q", text)
	}

	// With show_hidden — should see .hidden.
	result, err = listDirHandler(nil, newTestRequest(map[string]any{"path": dir, "show_hidden": true}))
	if err != nil {
		t.Fatal(err)
	}
	text = resultText(result)
	if !strings.Contains(text, ".hidden") {
		t.Errorf("expected .hidden in listing when show_hidden=true: %q", text)
	}
}

func TestListDirHandlerRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "child")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.txt"), []byte("d"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := listDirHandler(nil, newTestRequest(map[string]any{"path": dir, "recursive": true}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "deep.txt") {
		t.Errorf("expected deep.txt in recursive listing: %q", text)
	}
}

func TestListDirHandlerRejectsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "notadir.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := listDirHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when path is a file")
	}
}

// ── create_directory ─────────────────────────────────────────────────────────

func TestCreateDirHandler(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "a", "b", "c")
	result, err := createDirHandler(nil, newTestRequest(map[string]any{"path": newDir}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if info, statErr := os.Stat(newDir); statErr != nil || !info.IsDir() {
		t.Error("directory was not created")
	}
}

func TestCreateDirHandlerIdempotent(t *testing.T) {
	dir := t.TempDir()
	result, err := createDirHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error on existing dir: %s", resultText(result))
	}
}

// ── delete_directory ─────────────────────────────────────────────────────────

func TestDeleteDirHandlerEmpty(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "empty")
	if err := os.Mkdir(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := deleteDirHandler(nil, newTestRequest(map[string]any{"path": newDir}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if _, statErr := os.Stat(newDir); !os.IsNotExist(statErr) {
		t.Error("directory still exists after deletion")
	}
}

func TestDeleteDirHandlerForce(t *testing.T) {
	dir := t.TempDir()
	newDir := filepath.Join(dir, "nonempty")
	if err := os.Mkdir(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without force — should fail (non-empty).
	result, err := deleteDirHandler(nil, newTestRequest(map[string]any{"path": newDir}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when deleting non-empty dir without force")
	}

	// With force — should succeed.
	result, err = deleteDirHandler(nil, newTestRequest(map[string]any{"path": newDir, "force": true}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error with force=true: %s", resultText(result))
	}
	if _, statErr := os.Stat(newDir); !os.IsNotExist(statErr) {
		t.Error("directory still exists after force delete")
	}
}

func TestDeleteDirHandlerRejectsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := deleteDirHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when path is a file")
	}
}

func TestListDirHandlerTotalSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world!"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := listDirHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "B") {
		t.Errorf("expected total size in listing summary: %q", text)
	}
	if !strings.Contains(text, "file") {
		t.Errorf("expected 'file' count in listing summary: %q", text)
	}
}

// ── directory_tree ────────────────────────────────────────────────────────────

func TestDirTreeHandler(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := dirTreeHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "root.go") {
		t.Errorf("expected root.go in tree: %q", text)
	}
	if !strings.Contains(text, "subdir") {
		t.Errorf("expected subdir in tree: %q", text)
	}
	if !strings.Contains(text, "nested.go") {
		t.Errorf("expected nested.go in tree: %q", text)
	}
	if !strings.Contains(text, "──") {
		t.Errorf("expected tree connectors in output: %q", text)
	}
	if !strings.Contains(text, "file") {
		t.Errorf("expected file count in summary: %q", text)
	}
}

func TestDirTreeHandlerExcludePatterns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "exclude.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{
		"path":             dir,
		"exclude_patterns": []any{"*.log"},
	})
	result, err := dirTreeHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "keep.go") {
		t.Errorf("expected keep.go in tree: %q", text)
	}
	if strings.Contains(text, "exclude.log") {
		t.Errorf("excluded.log should not appear in tree: %q", text)
	}
}

func TestDirTreeHandlerRejectsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := dirTreeHandler(nil, newTestRequest(map[string]any{"path": f}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when path is a file")
	}
}

// ── list_directory sort_by ────────────────────────────────────────────────────

func TestListDirHandlerSortBySize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte("hello world this is larger"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"path": dir, "sort_by": "size"})
	result, err := listDirHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	largeIdx := strings.Index(text, "large.txt")
	smallIdx := strings.Index(text, "small.txt")
	if largeIdx == -1 || smallIdx == -1 {
		t.Fatalf("both files must appear in listing: %q", text)
	}
	if largeIdx > smallIdx {
		t.Errorf("sort_by=size: expected large.txt before small.txt in output")
	}
}

func TestListDirHandlerGlobFilter(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"main.go":    "package main",
		"util.go":    "package main",
		"config.yml": "key: value",
		"README.md":  "# readme",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	req := newTestRequest(map[string]any{"path": dir, "glob": "*.go"})
	result, err := listDirHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "main.go") {
		t.Errorf("expected main.go in glob=*.go listing: %q", text)
	}
	if !strings.Contains(text, "util.go") {
		t.Errorf("expected util.go in glob=*.go listing: %q", text)
	}
	if strings.Contains(text, "config.yml") {
		t.Errorf("config.yml should be excluded by glob=*.go: %q", text)
	}
	if strings.Contains(text, "README.md") {
		t.Errorf("README.md should be excluded by glob=*.go: %q", text)
	}
}

func TestListDirHandlerExcludePatterns(t *testing.T) {
	dir := t.TempDir()
	skip := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(skip, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skip, "dep.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := listDirHandler(nil, newTestRequest(map[string]any{
		"path":             dir,
		"recursive":        true,
		"exclude_patterns": []any{"node_modules"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "app.js") {
		t.Errorf("expected app.js in listing: %q", text)
	}
	if strings.Contains(text, "node_modules") || strings.Contains(text, "dep.js") {
		t.Errorf("excluded directory should not appear in listing: %q", text)
	}
}

func TestListDirHandlerMaxEntriesTruncates(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := listDirHandler(nil, newTestRequest(map[string]any{
		"path":        dir,
		"max_entries": float64(2),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Output truncated after 2 entries") {
		t.Errorf("expected truncation notice: %q", text)
	}
	if !strings.Contains(text, "a.txt") || !strings.Contains(text, "b.txt") {
		t.Errorf("expected first two files before truncation: %q", text)
	}
	if strings.Contains(text, "c.txt") {
		t.Errorf("c.txt should be omitted by max_entries=2: %q", text)
	}
}

// ── directory_tree glob filter ────────────────────────────────────────────────

func TestDirTreeHandlerGlobFilter(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"main.go":   "package main",
		"util.go":   "package main",
		"README.md": "# readme",
		"data.json": "{}",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := dirTreeHandler(nil, newTestRequest(map[string]any{
		"path": dir,
		"glob": "*.go",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "main.go") {
		t.Errorf("expected main.go in glob=*.go tree: %q", text)
	}
	if !strings.Contains(text, "util.go") {
		t.Errorf("expected util.go in glob=*.go tree: %q", text)
	}
	if strings.Contains(text, "README.md") {
		t.Errorf("README.md should be excluded by glob=*.go: %q", text)
	}
	if strings.Contains(text, "data.json") {
		t.Errorf("data.json should be excluded by glob=*.go: %q", text)
	}
}

// ── directory_tree glob prunes empty dirs ─────────────────────────────────────

func TestDirTreeHandlerGlobPrunesEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	sub1 := filepath.Join(dir, "sub1")
	sub2 := filepath.Join(dir, "sub2")
	if err := os.MkdirAll(sub1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub1, "data.txt"), []byte("txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, "helper.go"), []byte("pkg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("pkg"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := dirTreeHandler(nil, newTestRequest(map[string]any{
		"path": dir,
		"glob": "*.go",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)

	// sub1 has no .go files — it should be pruned.
	if strings.Contains(text, "sub1") {
		t.Errorf("sub1 (no .go files) should be pruned from glob=*.go tree: %q", text)
	}
	// sub2 has helper.go — it must appear.
	if !strings.Contains(text, "sub2") {
		t.Errorf("sub2 should appear (contains helper.go): %q", text)
	}
	if !strings.Contains(text, "helper.go") {
		t.Errorf("expected helper.go in tree: %q", text)
	}
	if !strings.Contains(text, "main.go") {
		t.Errorf("expected main.go in tree: %q", text)
	}
}

func TestDirTreeHandlerMaxEntriesTruncates(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := dirTreeHandler(nil, newTestRequest(map[string]any{
		"path":        dir,
		"max_entries": float64(2),
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Output truncated after 2 entries") {
		t.Errorf("expected truncation notice: %q", text)
	}
	if !strings.Contains(text, "a.txt") || !strings.Contains(text, "b.txt") {
		t.Errorf("expected first two files before truncation: %q", text)
	}
	if strings.Contains(text, "c.txt") {
		t.Errorf("c.txt should be omitted by max_entries=2: %q", text)
	}
}

// ── list_directory max_depth ──────────────────────────────────────────────────
// Reason: max_depth is a documented parameter of list_directory that was never
// exercised. A regression in depth-limiting would make the tool ignore the
// parameter silently, exposing unexpected deep results.

func TestListDirHandlerMaxDepth(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "level1")
	deep := filepath.Join(sub, "level2")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "shallow.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "deep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := newTestRequest(map[string]any{"path": dir, "recursive": true, "max_depth": float64(1)})
	result, err := listDirHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "shallow.txt") {
		t.Errorf("expected shallow.txt at depth 1: %q", text)
	}
	if strings.Contains(text, "deep.txt") {
		t.Errorf("deep.txt at depth 2 should be excluded with max_depth=1: %q", text)
	}
}

// ── delete_directory nonexistent ─────────────────────────────────────────────
// Reason: Attempting to delete a directory that does not exist should return
// a clear error. This common LLM mistake was untested.

func TestDeleteDirHandlerNonExistent(t *testing.T) {
	result, err := deleteDirHandler(nil, newTestRequest(map[string]any{
		"path": filepath.Join(t.TempDir(), "ghost_dir"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error when deleting a nonexistent directory")
	}
}

// ── create_directory empty path ───────────────────────────────────────────────
// Reason: An empty path argument should produce a clear error, not a panic or
// a directory created at the process's working directory root.

func TestCreateDirHandlerEmptyPath(t *testing.T) {
	result, err := createDirHandler(nil, newTestRequest(map[string]any{"path": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty path argument")
	}
}

func TestListDirHandlerGlobPrunesEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	sub1 := filepath.Join(dir, "sub1")
	sub2 := filepath.Join(dir, "sub2")
	if err := os.MkdirAll(sub1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub1, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, "match.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := listDirHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"recursive": true,
		"glob":      "*.go",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if strings.Contains(text, "sub1") {
		t.Fatalf("sub1 should be pruned when its subtree has no glob match: %q", text)
	}
	if !strings.Contains(text, "sub2") || !strings.Contains(text, "match.go") {
		t.Fatalf("expected matching subtree in output: %q", text)
	}
}

func TestDirHandlersLabelSymlinkedDirectories(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "child.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on this platform/user: %v", err)
	}

	listResult, err := listDirHandler(nil, newTestRequest(map[string]any{
		"path":      dir,
		"recursive": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	listText := resultText(listResult)
	if !strings.Contains(listText, "[LINK] linked/") {
		t.Fatalf("expected symlinked directory label in list_directory: %q", listText)
	}
	if strings.Contains(listText, "child.txt") {
		t.Fatalf("symlinked directory should not be traversed by default: %q", listText)
	}

	treeResult, err := dirTreeHandler(nil, newTestRequest(map[string]any{"path": dir}))
	if err != nil {
		t.Fatal(err)
	}
	treeText := resultText(treeResult)
	if !strings.Contains(treeText, "linked/ ->") {
		t.Fatalf("expected symlinked directory label in directory_tree: %q", treeText)
	}
}
