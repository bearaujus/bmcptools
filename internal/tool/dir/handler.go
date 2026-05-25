package dir

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

const defaultMaxEntries = 500

type entryLimit struct {
	max       int
	shown     int
	truncated bool
}

func newEntryLimit(req mcp.CallToolRequest) *entryLimit {
	maxEntries := int(req.GetFloat("max_entries", defaultMaxEntries))
	if maxEntries < 0 {
		maxEntries = defaultMaxEntries
	}
	return &entryLimit{max: maxEntries}
}

func (l *entryLimit) allow() bool {
	if l == nil || l.max == 0 {
		return true
	}
	if l.shown >= l.max {
		l.truncated = true
		return false
	}
	l.shown++
	return true
}

func (l *entryLimit) isTruncated() bool {
	return l != nil && l.truncated
}

func matchesAnyPattern(name string, patterns []string) bool {
	for _, pat := range patterns {
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
	}
	return false
}

func matchesGlobFilter(name, globFilter string) bool {
	if globFilter == "" {
		return true
	}
	for _, alt := range helper.ExpandAlternation(globFilter) {
		if ok, _ := filepath.Match(alt, name); ok {
			return true
		}
	}
	return false
}

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
	excludePatterns := req.GetStringSlice("exclude_patterns", nil)
	limit := newEntryLimit(req)

	var sb strings.Builder
	abs, _ := filepath.Abs(path)
	fmt.Fprintf(&sb, "Directory: %s\n\n", abs)

	var fileCount, dirCount int
	var totalBytes int64
	err = listDirRecursive(
		&sb,
		path,
		"",
		showHidden,
		recursive,
		0,
		maxDepth,
		sortBy,
		globFilter,
		excludePatterns,
		limit,
		&fileCount,
		&dirCount,
		&totalBytes,
	)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing error: %v", err)), nil
	}

	if limit.isTruncated() {
		fmt.Fprintf(&sb, "\nOutput truncated after %d entries. Rerun with a narrower glob/exclude_patterns or raise max_entries (0 = unlimited).\n", limit.max)
	}
	fmt.Fprintf(&sb, "\nShown: %s, %s (%s)",
		helper.Pluralize(fileCount, "file"),
		helper.Pluralize(dirCount, "directory"),
		helper.HumanizeBytes(totalBytes),
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
	excludePatterns []string,
	limit *entryLimit,
	fileCount, dirCount *int,
	totalBytes *int64,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var items []helper.EntryWithInfo
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if matchesAnyPattern(name, excludePatterns) {
			continue
		}
		fi, _ := e.Info()
		items = append(items, helper.EntryWithInfo{Entry: e, Info: fi})
	}

	sort.Slice(items, func(i, j int) bool {
		ai, bi := items[i], items[j]
		if ai.Entry.IsDir() != bi.Entry.IsDir() {
			return ai.Entry.IsDir()
		}
		if sortBy == "size" && !ai.Entry.IsDir() {
			sizeA, sizeB := int64(0), int64(0)
			if ai.Info != nil {
				sizeA = ai.Info.Size()
			}
			if bi.Info != nil {
				sizeB = bi.Info.Size()
			}
			return sizeA > sizeB
		}
		return ai.Entry.Name() < bi.Entry.Name()
	})

	for _, item := range items {
		name := item.Entry.Name()

		if item.Entry.IsDir() {
			if !limit.allow() {
				return nil
			}
			*dirCount++
			fmt.Fprintf(sb, "%s[DIR]  %s/\n", prefix, name)
			if recursive && depth < maxDepth {
				if err := listDirRecursive(
					sb,
					filepath.Join(dir, name),
					prefix+"    ",
					showHidden, recursive,
					depth+1, maxDepth, sortBy, globFilter, excludePatterns, limit,
					fileCount, dirCount, totalBytes,
				); err != nil {
					fmt.Fprintf(sb, "%s    [ERROR] cannot read subdirectory: %v\n", prefix, err)
				}
				if limit.isTruncated() {
					return nil
				}
			}
		} else {
			if !matchesGlobFilter(name, globFilter) {
				continue
			}
			if !limit.allow() {
				return nil
			}
			*fileCount++
			if item.Info != nil {
				*totalBytes += item.Info.Size()
				size := helper.HumanizeBytes(item.Info.Size())
				mod := item.Info.ModTime().Format("2006-01-02 15:04")
				fmt.Fprintf(sb, "%s[FILE] %-40s %8s  %s\n", prefix, name, size, mod)
			} else {
				fmt.Fprintf(sb, "%s[FILE] %s  [stat error]\n", prefix, name)
			}
		}
	}
	return nil
}

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

	if err := os.Remove(path); err != nil {
		return mcp.NewToolResultError(
			fmt.Sprintf("cannot delete %q (it may not be empty — use force=true to delete recursively): %v", path, err),
		), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Deleted directory: %s", path)), nil
}

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
	limit := newEntryLimit(req)

	abs, _ := filepath.Abs(path)
	var sb strings.Builder
	sb.WriteString(abs + "\n")

	var fileCount, dirCount int
	var totalBytes int64
	buildDirTree(&sb, path, "", showHidden, 0, maxDepth, excludePatterns, globFilter, limit, &fileCount, &dirCount, &totalBytes)

	if limit.isTruncated() {
		fmt.Fprintf(&sb, "\nOutput truncated after %d entries. Rerun with a narrower glob/exclude_patterns or raise max_entries (0 = unlimited).\n", limit.max)
	}
	fmt.Fprintf(&sb, "\n%s, %s (%s)", helper.Pluralize(fileCount, "file"), helper.Pluralize(dirCount, "directory"), helper.HumanizeBytes(totalBytes))
	return mcp.NewToolResultText(sb.String()), nil
}

func buildDirTree(
	sb *strings.Builder,
	dir, prefix string,
	showHidden bool,
	depth, maxDepth int,
	excludePatterns []string,
	globFilter string,
	limit *entryLimit,
	fileCount, dirCount *int,
	totalBytes *int64,
) bool {
	visible, err := visibleTreeEntries(dir, showHidden, depth, maxDepth, excludePatterns, globFilter)
	if err != nil {
		fmt.Fprintf(sb, "%s[ERROR] %v\n", prefix, err)
		return false
	}

	for i, item := range visible {
		if !limit.allow() {
			return true
		}

		isLast := i == len(visible)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		name := item.Entry.Name()
		if item.Entry.IsDir() {
			*dirCount++
			fmt.Fprintf(sb, "%s%s%s/\n", prefix, connector, name)
			if depth >= maxDepth {
				if globFilter == "" {
					fmt.Fprintf(sb, "%s...\n", childPrefix)
				}
				continue
			}

			buildDirTree(
				sb,
				filepath.Join(dir, name),
				childPrefix,
				showHidden, depth+1, maxDepth,
				excludePatterns, globFilter, limit,
				fileCount, dirCount, totalBytes,
			)
			if limit.isTruncated() {
				return true
			}
		} else {
			*fileCount++
			sizeStr := ""
			if item.Info != nil {
				*totalBytes += item.Info.Size()
				sizeStr = "  " + helper.HumanizeBytes(item.Info.Size())
			}
			fmt.Fprintf(sb, "%s%s%s%s\n", prefix, connector, name, sizeStr)
		}
	}

	return len(visible) > 0
}

func visibleTreeEntries(
	dir string,
	showHidden bool,
	depth, maxDepth int,
	excludePatterns []string,
	globFilter string,
) ([]helper.EntryWithInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	items := make([]helper.EntryWithInfo, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if matchesAnyPattern(name, excludePatterns) {
			continue
		}
		fi, _ := e.Info()
		items = append(items, helper.EntryWithInfo{Entry: e, Info: fi})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Entry.IsDir() != items[j].Entry.IsDir() {
			return items[i].Entry.IsDir()
		}
		return items[i].Entry.Name() < items[j].Entry.Name()
	})

	if globFilter == "" {
		return items, nil
	}

	visible := make([]helper.EntryWithInfo, 0, len(items))
	for _, item := range items {
		name := item.Entry.Name()
		if item.Entry.IsDir() {
			if depth < maxDepth && treeHasGlobMatch(filepath.Join(dir, name), showHidden, depth+1, maxDepth, excludePatterns, globFilter) {
				visible = append(visible, item)
			}
			continue
		}
		if matchesGlobFilter(name, globFilter) {
			visible = append(visible, item)
		}
	}
	return visible, nil
}

func treeHasGlobMatch(
	dir string,
	showHidden bool,
	depth, maxDepth int,
	excludePatterns []string,
	globFilter string,
) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if matchesAnyPattern(name, excludePatterns) {
			continue
		}
		if e.IsDir() {
			if depth < maxDepth && treeHasGlobMatch(filepath.Join(dir, name), showHidden, depth+1, maxDepth, excludePatterns, globFilter) {
				return true
			}
			continue
		}
		if matchesGlobFilter(name, globFilter) {
			return true
		}
	}
	return false
}
