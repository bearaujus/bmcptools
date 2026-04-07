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
			if globFilter != "" {
				matched := false
				for _, alt := range helper.ExpandAlternation(globFilter) {
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

	abs, _ := filepath.Abs(path)
	var sb strings.Builder
	sb.WriteString(abs + "\n")

	var fileCount, dirCount int
	var totalBytes int64
	buildDirTree(&sb, path, "", showHidden, 0, maxDepth, excludePatterns, globFilter, &fileCount, &dirCount, &totalBytes)

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
	fileCount, dirCount *int,
	totalBytes *int64,
) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(sb, "%s[ERROR] %v\n", prefix, err)
		return false
	}

	var rawItems []helper.EntryWithInfo
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
		rawItems = append(rawItems, helper.EntryWithInfo{Entry: e, Info: fi})
	}

	sort.Slice(rawItems, func(i, j int) bool {
		if rawItems[i].Entry.IsDir() != rawItems[j].Entry.IsDir() {
			return rawItems[i].Entry.IsDir()
		}
		return rawItems[i].Entry.Name() < rawItems[j].Entry.Name()
	})

	type preItem struct {
		name    string
		isDir   bool
		sizeStr string
		subOut  string
		subFC   int
		subDC   int
		subTB   int64
		fileSz  int64
		visible bool
	}

	pre := make([]preItem, 0, len(rawItems))

	for _, item := range rawItems {
		name := item.Entry.Name()
		if item.Entry.IsDir() {
			if depth >= maxDepth {
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
				prefix+"│   ",
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
				for _, alt := range helper.ExpandAlternation(globFilter) {
					if ok, _ := filepath.Match(alt, name); ok {
						show = true
						break
					}
				}
			}
			sz := int64(0)
			sizeStr := ""
			if item.Info != nil {
				sz = item.Info.Size()
				sizeStr = "  " + helper.HumanizeBytes(sz)
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

	var visible []preItem
	for _, p := range pre {
		if p.visible {
			visible = append(visible, p)
		}
	}

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
