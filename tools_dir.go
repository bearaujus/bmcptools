package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDirTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("list_directory",
		mcp.WithDescription(td("list_directory")),
		mcp.WithString("path", mcp.Description(pd("list_directory", "path"))),
		mcp.WithBoolean("show_hidden", mcp.Description(pd("list_directory", "show_hidden"))),
		mcp.WithBoolean("recursive", mcp.Description(pd("list_directory", "recursive"))),
		mcp.WithNumber("max_depth", mcp.Description(pd("list_directory", "max_depth"))),
		mcp.WithString("sort_by", mcp.Description(pd("list_directory", "sort_by"))),
		mcp.WithString("glob", mcp.Description(pd("list_directory", "glob"))),
	), listDirHandler)

	s.AddTool(mcp.NewTool("create_directory",
		mcp.WithDescription(td("create_directory")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("create_directory", "path"))),
	), createDirHandler)

	s.AddTool(mcp.NewTool("delete_directory",
		mcp.WithDescription(td("delete_directory")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("delete_directory", "path"))),
		mcp.WithBoolean("force", mcp.Description(pd("delete_directory", "force"))),
	), deleteDirHandler)

	s.AddTool(mcp.NewTool("directory_tree",
		mcp.WithDescription(td("directory_tree")),
		mcp.WithString("path", mcp.Required(), mcp.Description(pd("directory_tree", "path"))),
		mcp.WithNumber("max_depth", mcp.Description(pd("directory_tree", "max_depth"))),
		mcp.WithBoolean("show_hidden", mcp.Description(pd("directory_tree", "show_hidden"))),
		mcp.WithArray("exclude_patterns",
			mcp.Description(pd("directory_tree", "exclude_patterns")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("glob",
			mcp.Description(pd("directory_tree", "glob")),
		),
	), dirTreeHandler)
}

// ── list_directory ───────────────────────────────────────────────────────────

func listDirHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot get working directory: %v", err)), nil
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", path, err)), nil
	}
	if !info.IsDir() {
		return mcp.NewToolResultError("path is a file; use read_file to read it"), nil
	}

	showHidden := req.GetBool("show_hidden", false)
	recursive := req.GetBool("recursive", false)

	maxDepth := 3
	if md := req.GetFloat("max_depth", -1); md >= 0 {
		maxDepth = int(md)
	}

	sortBy := req.GetString("sort_by", "name")
	if sortBy != "size" {
		sortBy = "name"
	}

	globFilter := req.GetString("glob", "")

	var sb strings.Builder
	abs, _ := filepath.Abs(path)
	fmt.Fprintf(&sb, "Directory: %s\n\n", abs)

	var fileCount, dirCount int
	var totalBytes int64
	err = listDirRecursive(&sb, path, "", showHidden, recursive, 0, maxDepth, sortBy, globFilter, &fileCount, &dirCount, &totalBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing error: %v", err)), nil
	}

	fmt.Fprintf(&sb, "\nTotal: %s, %s (%s)",
		pluralize(fileCount, "file"),
		pluralize(dirCount, "directory"),
		humanizeBytes(totalBytes),
	)
	return mcp.NewToolResultText(sb.String()), nil
}

func listDirRecursive(
	sb *strings.Builder,
	dir, prefix string,
	showHidden, recursive bool,
	depth, maxDepth int,
	sortBy string,
	globFilter string,
	fileCount, dirCount *int,
	totalBytes *int64,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var items []entryWithInfo
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		fi, _ := e.Info() // nil info is handled gracefully below
		items = append(items, entryWithInfo{entry: e, info: fi})
	}

	// Sort: directories always first, then by name or size.
	sort.Slice(items, func(i, j int) bool {
		ai, bi := items[i], items[j]
		if ai.entry.IsDir() != bi.entry.IsDir() {
			return ai.entry.IsDir()
		}
		if sortBy == "size" && !ai.entry.IsDir() {
			sizeA, sizeB := int64(0), int64(0)
			if ai.info != nil {
				sizeA = ai.info.Size()
			}
			if bi.info != nil {
				sizeB = bi.info.Size()
			}
			return sizeA > sizeB // largest first
		}
		return ai.entry.Name() < bi.entry.Name()
	})

	for _, item := range items {
		name := item.entry.Name()

		if item.entry.IsDir() {
			*dirCount++
			fmt.Fprintf(sb, "%s[DIR]  %s/\n", prefix, name)
			if recursive && depth < maxDepth {
				if err := listDirRecursive(
					sb,
					filepath.Join(dir, name),
					prefix+"    ",
					showHidden, recursive,
					depth+1, maxDepth, sortBy, globFilter,
					fileCount, dirCount, totalBytes,
				); err != nil {
					fmt.Fprintf(sb, "%s    [ERROR] cannot read subdirectory: %v\n", prefix, err)
				}
			}
		} else {
			// Apply glob filter to files only; directories are always shown.
			if globFilter != "" {
				matched := false
				for _, alt := range expandAlternation(globFilter) {
					if ok, _ := filepath.Match(alt, name); ok {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			*fileCount++
			if item.info != nil {
				*totalBytes += item.info.Size()
				size := humanizeBytes(item.info.Size())
				mod := item.info.ModTime().Format("2006-01-02 15:04")
				fmt.Fprintf(sb, "%s[FILE] %-40s %8s  %s\n", prefix, name, size, mod)
			} else {
				fmt.Fprintf(sb, "%s[FILE] %s  [stat error]\n", prefix, name)
			}
		}
	}
	return nil
}

// ── create_directory ─────────────────────────────────────────────────────────

func createDirHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot create directory: %v", err)), nil
	}

	abs, _ := filepath.Abs(path)
	return mcp.NewToolResultText(fmt.Sprintf("Created directory: %s", abs)), nil
}

// ── delete_directory ─────────────────────────────────────────────────────────

func deleteDirHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", path, err)), nil
	}
	if !info.IsDir() {
		return mcp.NewToolResultError("path is a file; use delete_file instead"), nil
	}

	force := req.GetBool("force", false)

	if force {
		if err := os.RemoveAll(path); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot delete directory: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Deleted directory (recursively): %s", path)), nil
	}

	// Non-force: only remove if empty.
	if err := os.Remove(path); err != nil {
		return mcp.NewToolResultError(
			fmt.Sprintf("cannot delete %q (it may not be empty — use force=true to delete recursively): %v", path, err),
		), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Deleted directory: %s", path)), nil
}

// ── directory_tree ────────────────────────────────────────────────────────────

func dirTreeHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", path, err)), nil
	}
	if !info.IsDir() {
		return mcp.NewToolResultError("path is a file; use read_file to read it"), nil
	}

	showHidden := req.GetBool("show_hidden", false)
	maxDepth := 3
	if md := req.GetFloat("max_depth", -1); md >= 0 {
		maxDepth = int(md)
	}
	excludePatterns := req.GetStringSlice("exclude_patterns", nil)
	globFilter := req.GetString("glob", "")

	abs, _ := filepath.Abs(path)
	var sb strings.Builder
	sb.WriteString(abs + "\n")

	var fileCount, dirCount int
	var totalBytes int64
	buildDirTree(&sb, path, "", showHidden, 0, maxDepth, excludePatterns, globFilter, &fileCount, &dirCount, &totalBytes)

	fmt.Fprintf(&sb, "\n%s, %s (%s)", pluralize(fileCount, "file"), pluralize(dirCount, "directory"), humanizeBytes(totalBytes))
	return mcp.NewToolResultText(sb.String()), nil
}

// buildDirTree recursively writes a visual tree to sb.
// Returns true if at least one entry was written (used by parent for glob-aware pruning).
func buildDirTree(
	sb *strings.Builder,
	dir, prefix string,
	showHidden bool,
	depth, maxDepth int,
	excludePatterns []string,
	globFilter string,
	fileCount, dirCount *int,
	totalBytes *int64,
) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(sb, "%s[ERROR] %v\n", prefix, err)
		return false
	}

	var rawItems []entryWithInfo
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		excluded := false
		for _, pat := range excludePatterns {
			if ok, _ := filepath.Match(pat, name); ok {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		fi, _ := e.Info()
		rawItems = append(rawItems, entryWithInfo{entry: e, info: fi})
	}

	// Sort: directories first, then alphabetical.
	sort.Slice(rawItems, func(i, j int) bool {
		if rawItems[i].entry.IsDir() != rawItems[j].entry.IsDir() {
			return rawItems[i].entry.IsDir()
		}
		return rawItems[i].entry.Name() < rawItems[j].entry.Name()
	})

	// Pre-render each entry using local counters so we can decide visibility before choosing connectors.
	type preItem struct {
		name    string
		isDir   bool
		sizeStr string     // non-empty for files
		subOut  string     // subtree output for directories
		subFC   int        // file count in subtree
		subDC   int        // directory count in subtree
		subTB   int64      // total bytes in subtree
		fileSz  int64      // own size for files
		visible bool
	}

	pre := make([]preItem, 0, len(rawItems))

	for _, item := range rawItems {
		name := item.entry.Name()
		if item.entry.IsDir() {
			// At depth limit: can't know if subtree has matches; show dir only when no glob.
			if depth >= maxDepth {
				// placeholder — emit "..." as a single line
				var buf strings.Builder
				fmt.Fprintf(&buf, "%s│   ...\n", prefix)
				pre = append(pre, preItem{
					name:    name,
					isDir:   true,
					subOut:  buf.String(),
					visible: globFilter == "",
				})
				continue
			}

			var sub strings.Builder
			var subFC, subDC int
			var subTB int64
			childHasMatch := buildDirTree(
				&sub,
				filepath.Join(dir, name),
				prefix+"│   ", // placeholder prefix — fixed up when isLast
				showHidden, depth+1, maxDepth,
				excludePatterns, globFilter,
				&subFC, &subDC, &subTB,
			)

			pre = append(pre, preItem{
				name:    name,
				isDir:   true,
				subOut:  sub.String(),
				subFC:   subFC,
				subDC:   subDC,
				subTB:   subTB,
				visible: globFilter == "" || childHasMatch,
			})
		} else {
			show := true
			if globFilter != "" {
				show = false
				for _, alt := range expandAlternation(globFilter) {
					if ok, _ := filepath.Match(alt, name); ok {
						show = true
						break
					}
				}
			}
			sz := int64(0)
			sizeStr := ""
			if item.info != nil {
				sz = item.info.Size()
				sizeStr = "  " + humanizeBytes(sz)
			}
			pre = append(pre, preItem{
				name:    name,
				isDir:   false,
				sizeStr: sizeStr,
				fileSz:  sz,
				visible: show,
			})
		}
	}

	// Collect only visible entries.
	var visible []preItem
	for _, p := range pre {
		if p.visible {
			visible = append(visible, p)
		}
	}

	// Emit with correct connectors (isLast changes ├── → └── and child prefix │    →     ).
	for i, p := range visible {
		isLast := i == len(visible)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		if p.isDir {
			*dirCount++
			*fileCount += p.subFC
			*dirCount += p.subDC
			*totalBytes += p.subTB
			fmt.Fprintf(sb, "%s%s%s/\n", prefix, connector, p.name)

			// The subtree was built with prefix+"│   ".  When this dir is the last sibling,
			// replace that placeholder prefix with the correct "    " at this level only.
			subOut := p.subOut
			if isLast {
				subOut = strings.ReplaceAll(subOut, prefix+"│   ", childPrefix)
			}
			sb.WriteString(subOut)
		} else {
			*fileCount++
			*totalBytes += p.fileSz
			fmt.Fprintf(sb, "%s%s%s%s\n", prefix, connector, p.name, p.sizeStr)
		}
	}

	return len(visible) > 0
}
