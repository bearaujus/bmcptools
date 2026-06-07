package dir

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

type detailLevel string

const (
	detailNames detailLevel = "names"
	detailSize  detailLevel = "size"
	detailFull  detailLevel = "full"
)

type dirItemKind int

const (
	dirKindDirectory dirItemKind = iota
	dirKindLinkDir
	dirKindFile
	dirKindLinkFile
)

type dirItem struct {
	helper.EntryWithInfo
	kind     dirItemKind
	linkMeta *helper.SymlinkMetadata
}

type dirRenderStats struct {
	fileCount     int
	dirCount      int
	linkCount     int
	totalBytes    int64
	skippedErrors int
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

func parseDetailLevel(req mcp.CallToolRequest, defaultLevel detailLevel) detailLevel {
	switch req.GetString("detail", string(defaultLevel)) {
	case string(detailFull):
		return detailFull
	case string(detailSize):
		return detailSize
	case string(detailNames):
		return detailNames
	default:
		return defaultLevel
	}
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

func effectiveExcludePatterns(req mcp.CallToolRequest) []string {
	if _, provided := req.GetArguments()["exclude_patterns"]; provided {
		return req.GetStringSlice("exclude_patterns", nil)
	}
	return helper.DefaultTraversalExcludePatterns()
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

	detail := parseDetailLevel(req, detailSize)
	globFilter := req.GetString("glob", "")
	excludePatterns := effectiveExcludePatterns(req)
	limit := newEntryLimit(req)
	matchCache := make(map[string]bool)
	stats := &dirRenderStats{}

	var sb strings.Builder
	abs, _ := filepath.Abs(path)
	fmt.Fprintf(&sb, "Directory: %s\n\n", abs)

	err = listDirRecursive(
		&sb,
		path,
		"",
		showHidden,
		recursive,
		0,
		maxDepth,
		sortBy,
		detail,
		globFilter,
		excludePatterns,
		limit,
		stats,
		matchCache,
	)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing error: %v", err)), nil
	}

	if limit.isTruncated() {
		fmt.Fprintf(&sb, "\nOutput truncated after %d entries. Rerun with a narrower glob/exclude_patterns or raise max_entries (0 = unlimited).\n", limit.max)
	}
	fmt.Fprintf(&sb, "\nShown: %s, %s", helper.Pluralize(stats.fileCount, "file"), helper.Pluralize(stats.dirCount, "directory"))
	if stats.linkCount > 0 {
		fmt.Fprintf(&sb, ", %s", helper.Pluralize(stats.linkCount, "link"))
	}
	fmt.Fprintf(&sb, " (%s)", helper.HumanizeBytes(stats.totalBytes))
	if stats.skippedErrors > 0 {
		fmt.Fprintf(&sb, "\nSkipped %s due to permission/stat errors.", helper.Pluralize(stats.skippedErrors, "entry"))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func listDirRecursive(
	sb *strings.Builder,
	dir, prefix string,
	showHidden, recursive bool,
	depth, maxDepth int,
	sortBy string,
	detail detailLevel,
	globFilter string,
	excludePatterns []string,
	limit *entryLimit,
	stats *dirRenderStats,
	matchCache map[string]bool,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var items []dirItem
	for _, e := range entries {
		fullPath := filepath.Join(dir, e.Name())
		name := e.Name()
		if !showHidden && helper.IsHiddenPath(fullPath, nil) {
			continue
		}
		if matchesAnyPattern(name, excludePatterns) {
			continue
		}
		items = append(items, buildDirItem(fullPath, e, stats))
	}

	sort.Slice(items, func(i, j int) bool {
		ai, bi := items[i], items[j]
		if items[i].kind != items[j].kind {
			return items[i].kind < items[j].kind
		}
		if sortBy == "size" && ai.kind != dirKindDirectory && ai.kind != dirKindLinkDir {
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
		switch item.kind {
		case dirKindDirectory:
			if globFilter != "" {
				if !recursive {
					continue
				}
				subdir := filepath.Join(dir, name)
				hasMatch, matchErr := subtreeHasGlobMatch(subdir, showHidden, depth+1, maxDepth, excludePatterns, globFilter, matchCache)
				if !hasMatch && matchErr == nil {
					continue
				}
			}
			if !limit.allow() {
				return nil
			}
			stats.dirCount++
			fmt.Fprintf(sb, "%s[DIR]  %s/\n", prefix, name)
			if recursive && depth < maxDepth {
				if err := listDirRecursive(
					sb,
					filepath.Join(dir, name),
					prefix+"    ",
					showHidden, recursive,
					depth+1, maxDepth, sortBy, detail, globFilter, excludePatterns, limit,
					stats, matchCache,
				); err != nil {
					stats.skippedErrors++
					if !limit.allow() {
						return nil
					}
					fmt.Fprintf(sb, "%s    [ERROR] cannot read subdirectory: %v\n", prefix, err)
				}
				if limit.isTruncated() {
					return nil
				}
			}
		case dirKindLinkDir, dirKindLinkFile:
			if globFilter != "" && item.kind == dirKindLinkFile && !matchesGlobFilter(name, globFilter) {
				continue
			}
			if !limit.allow() {
				return nil
			}
			stats.linkCount++
			fmt.Fprintf(sb, "%s%s\n", prefix, formatListLinkLine(item, detail))
		case dirKindFile:
			if !matchesGlobFilter(name, globFilter) {
				continue
			}
			if !limit.allow() {
				return nil
			}
			stats.fileCount++
			if item.Info != nil {
				stats.totalBytes += item.Info.Size()
			}
			fmt.Fprintf(sb, "%s%s\n", prefix, formatListFileLine(item, detail))
		}
	}
	return nil
}

func createDirHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	if err := helper.MkdirAllClear(path, 0o755); err != nil {
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
		resolvedPath, err := resolveSafeForceDeletePath(path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := os.RemoveAll(resolvedPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot delete directory: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Deleted directory (recursively): %s", resolvedPath)), nil
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
	excludePatterns := effectiveExcludePatterns(req)
	globFilter := req.GetString("glob", "")
	limit := newEntryLimit(req)
	detail := parseDetailLevel(req, detailNames)

	abs, _ := filepath.Abs(path)
	var sb strings.Builder
	sb.WriteString(abs + "\n")

	stats := &dirRenderStats{}
	nodes, treeErr := buildTreeNodes(path, showHidden, 0, maxDepth, excludePatterns, globFilter, stats)
	if treeErr != nil {
		fmt.Fprintf(&sb, "[ERROR] %v\n", treeErr)
	}
	renderDirTreeNodes(&sb, nodes, "", 0, maxDepth, detail, globFilter, limit, stats)

	if limit.isTruncated() {
		fmt.Fprintf(&sb, "\nOutput truncated after %d entries. Rerun with a narrower glob/exclude_patterns or raise max_entries (0 = unlimited).\n", limit.max)
	}
	fmt.Fprintf(&sb, "\n%s, %s", helper.Pluralize(stats.fileCount, "file"), helper.Pluralize(stats.dirCount, "directory"))
	if stats.linkCount > 0 {
		fmt.Fprintf(&sb, ", %s", helper.Pluralize(stats.linkCount, "link"))
	}
	fmt.Fprintf(&sb, " (%s)", helper.HumanizeBytes(stats.totalBytes))
	if stats.skippedErrors > 0 {
		fmt.Fprintf(&sb, "\nSkipped %s due to permission/stat errors.", helper.Pluralize(stats.skippedErrors, "entry"))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

type treeNode struct {
	entry    dirItem
	children []treeNode
	readErr  error
}

func renderDirTreeNodes(
	sb *strings.Builder,
	nodes []treeNode,
	prefix string,
	depth, maxDepth int,
	detail detailLevel,
	globFilter string,
	limit *entryLimit,
	stats *dirRenderStats,
) bool {
	for i, node := range nodes {
		if !limit.allow() {
			return true
		}

		isLast := i == len(nodes)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		name := node.entry.Entry.Name()
		switch node.entry.kind {
		case dirKindDirectory:
			stats.dirCount++
			fmt.Fprintf(sb, "%s%s%s/\n", prefix, connector, name)
			if node.readErr != nil {
				stats.skippedErrors++
				if !limit.allow() {
					return true
				}
				fmt.Fprintf(sb, "%s[ERROR] %v\n", childPrefix, node.readErr)
				continue
			}
			if depth >= maxDepth {
				if globFilter == "" {
					if !limit.allow() {
						return true
					}
					fmt.Fprintf(sb, "%s...\n", childPrefix)
				}
				continue
			}

			renderDirTreeNodes(
				sb,
				node.children,
				childPrefix,
				depth+1, maxDepth,
				detail, globFilter, limit,
				stats,
			)
			if limit.isTruncated() {
				return true
			}
		case dirKindLinkDir, dirKindLinkFile:
			stats.linkCount++
			fmt.Fprintf(sb, "%s%s%s\n", prefix, connector, formatTreeLinkLabel(node.entry, detail))
		default:
			stats.fileCount++
			if node.entry.Info != nil {
				stats.totalBytes += node.entry.Info.Size()
			}
			fmt.Fprintf(sb, "%s%s%s\n", prefix, connector, formatTreeFileLabel(node.entry, detail))
		}
	}

	return len(nodes) > 0
}

func buildTreeNodes(
	dir string,
	showHidden bool,
	depth, maxDepth int,
	excludePatterns []string,
	globFilter string,
	stats *dirRenderStats,
) ([]treeNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	items := make([]dirItem, 0, len(entries))
	for _, e := range entries {
		fullPath := filepath.Join(dir, e.Name())
		name := e.Name()
		if !showHidden && helper.IsHiddenPath(fullPath, nil) {
			continue
		}
		if matchesAnyPattern(name, excludePatterns) {
			continue
		}
		items = append(items, buildDirItem(fullPath, e, stats))
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].kind != items[j].kind {
			return items[i].kind < items[j].kind
		}
		return items[i].Entry.Name() < items[j].Entry.Name()
	})

	visible := make([]treeNode, 0, len(items))
	for _, item := range items {
		name := item.Entry.Name()
		if item.kind == dirKindDirectory {
			node := treeNode{entry: item}
			if depth < maxDepth {
				children, childErr := buildTreeNodes(filepath.Join(dir, name), showHidden, depth+1, maxDepth, excludePatterns, globFilter, stats)
				node.children = children
				node.readErr = childErr
			}
			if globFilter != "" && len(node.children) == 0 && node.readErr == nil {
				continue
			}
			visible = append(visible, node)
			continue
		}
		if item.kind == dirKindLinkDir || matchesGlobFilter(name, globFilter) {
			visible = append(visible, treeNode{entry: item})
		}
	}
	return visible, nil
}

func resolveSafeForceDeletePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", path, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", path, err)
	}
	if isFilesystemRoot(resolvedPath) {
		return "", fmt.Errorf("refusing to force-delete filesystem root %q", resolvedPath)
	}
	if cwd, err := os.Getwd(); err == nil && sameDirPath(resolvedPath, cwd) {
		return "", fmt.Errorf("refusing to force-delete the current working directory %q", resolvedPath)
	}
	if home, err := os.UserHomeDir(); err == nil && sameDirPath(resolvedPath, home) {
		return "", fmt.Errorf("refusing to force-delete the user home directory %q", resolvedPath)
	}
	if pathComponentDepth(resolvedPath) < 2 {
		return "", fmt.Errorf("refusing to force-delete shallow path %q; choose a more specific target", resolvedPath)
	}
	return resolvedPath, nil
}

func isFilesystemRoot(path string) bool {
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	rest := cleaned[len(volume):]
	return rest == string(filepath.Separator)
}

func sameDirPath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func pathComponentDepth(path string) int {
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	rest := strings.TrimPrefix(cleaned, volume)
	rest = strings.Trim(rest, string(filepath.Separator))
	if rest == "" {
		return 0
	}
	return len(strings.Split(rest, string(filepath.Separator)))
}

func buildDirItem(fullPath string, entry os.DirEntry, stats *dirRenderStats) dirItem {
	item := dirItem{
		EntryWithInfo: helper.EntryWithInfo{
			Path:  fullPath,
			Entry: entry,
		},
		kind: dirKindFile,
	}
	info, err := entry.Info()
	if err != nil {
		stats.skippedErrors++
	} else {
		item.Info = info
	}
	if entry.Type()&os.ModeSymlink != 0 {
		item.kind = dirKindLinkFile
		if meta, metaErr := helper.ReadSymlinkMetadata(fullPath); metaErr == nil {
			item.linkMeta = &meta
			if meta.TargetIsDir {
				item.kind = dirKindLinkDir
			}
		} else {
			stats.skippedErrors++
			if target, readErr := os.Readlink(fullPath); readErr == nil {
				item.linkMeta = &helper.SymlinkMetadata{Target: target}
			}
		}
		return item
	}
	if entry.IsDir() {
		item.kind = dirKindDirectory
	}
	return item
}

func subtreeHasGlobMatch(dir string, showHidden bool, depth, maxDepth int, excludePatterns []string, globFilter string, cache map[string]bool) (bool, error) {
	if globFilter == "" {
		return true, nil
	}
	key := fmt.Sprintf("%s|%d", filepath.Clean(dir), maxDepth-depth)
	if cached, ok := cache[key]; ok {
		return cached, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		if !showHidden && helper.IsHiddenPath(fullPath, nil) {
			continue
		}
		if matchesAnyPattern(entry.Name(), excludePatterns) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if meta, metaErr := helper.ReadSymlinkMetadata(fullPath); metaErr == nil && meta.TargetIsDir {
				cache[key] = true
				return true, nil
			}
		}
		if entry.IsDir() {
			if depth < maxDepth {
				hasMatch, childErr := subtreeHasGlobMatch(fullPath, showHidden, depth+1, maxDepth, excludePatterns, globFilter, cache)
				if hasMatch {
					cache[key] = true
					return true, nil
				}
				if childErr != nil {
					return false, childErr
				}
			}
			continue
		}
		if matchesGlobFilter(entry.Name(), globFilter) {
			cache[key] = true
			return true, nil
		}
	}
	cache[key] = false
	return false, nil
}

func formatListFileLine(item dirItem, detail detailLevel) string {
	name := item.Entry.Name()
	switch detail {
	case detailFull:
		if item.Info == nil {
			return fmt.Sprintf("[FILE] %s  [stat error]", name)
		}
		return fmt.Sprintf("[FILE] %-40s %8s  %s", name, helper.HumanizeBytes(item.Info.Size()), item.Info.ModTime().Format("2006-01-02 15:04"))
	case detailSize:
		if item.Info == nil {
			return fmt.Sprintf("[FILE] %s  [stat error]", name)
		}
		return fmt.Sprintf("[FILE] %s  %s", name, helper.HumanizeBytes(item.Info.Size()))
	default:
		return fmt.Sprintf("[FILE] %s", name)
	}
}

func formatListLinkLine(item dirItem, detail detailLevel) string {
	name := item.Entry.Name()
	suffix := ""
	if item.kind == dirKindLinkDir {
		name += "/"
	}
	if item.linkMeta != nil {
		suffix = " " + helper.FormatSymlinkCompact(*item.linkMeta)
	}
	if detail == detailFull && item.Info != nil {
		return fmt.Sprintf("[LINK] %-40s%s  %s", name, suffix, item.Info.ModTime().Format("2006-01-02 15:04"))
	}
	return fmt.Sprintf("[LINK] %s%s", name, suffix)
}

func formatTreeFileLabel(item dirItem, detail detailLevel) string {
	name := item.Entry.Name()
	if item.Info == nil || detail == detailNames {
		return name
	}
	if detail == detailFull {
		return fmt.Sprintf("%s  %s  %s", name, helper.HumanizeBytes(item.Info.Size()), item.Info.ModTime().Format("2006-01-02 15:04"))
	}
	return fmt.Sprintf("%s  %s", name, helper.HumanizeBytes(item.Info.Size()))
}

func formatTreeLinkLabel(item dirItem, detail detailLevel) string {
	name := item.Entry.Name()
	if item.kind == dirKindLinkDir {
		name += "/"
	}
	if item.linkMeta == nil {
		return name
	}
	return name + " " + helper.FormatSymlinkCompact(*item.linkMeta)
}
