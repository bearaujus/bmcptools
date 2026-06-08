package bmcptools

import (
	"fmt"
	"sort"
	"strings"
)

var groupOrder = []string{
	GroupUser,
	GroupFile,
	GroupMulti,
	GroupDir,
	GroupSearch,
	GroupExec,
	GroupSystem,
}

var groupTools = map[string][]string{
	GroupUser: {
		ToolNotifyUser,
		ToolAskUser,
		ToolGetUserResponse,
		ToolUpdateDialog,
		ToolCancelAskUser,
		ToolRest,
	},
	GroupFile: {
		ToolReadFile,
		ToolWriteFile,
		ToolAppendToFile,
		ToolEditFile,
		ToolDeleteFile,
		ToolCopyFile,
		ToolMoveFile,
		ToolGetFileInfo,
		ToolPathExists,
		ToolDiffFiles,
		ToolCalculateChecksum,
		ToolCreateSymlink,
		ToolCompressFiles,
		ToolExtractArchive,
	},
	GroupMulti: {
		ToolReadMultipleFiles,
		ToolWriteMultipleFiles,
		ToolFindReplaceInFiles,
		ToolPathExistsBatch,
		ToolGetMultipleFileInfo,
		ToolDeleteFiles,
		ToolCopyPaths,
		ToolMovePaths,
	},
	GroupDir: {
		ToolListDirectory,
		ToolCreateDirectory,
		ToolDeleteDirectory,
		ToolDirectoryTree,
	},
	GroupSearch: {
		ToolSearchFiles,
		ToolGrepFiles,
	},
	GroupExec: {
		ToolGetWorkingDirectory,
		ToolRunCommand,
		ToolOpenInApp,
		ToolGetEnv,
	},
	GroupSystem: {
		ToolClipboardWrite,
		ToolClipboardRead,
		ToolHTTPRequest,
		ToolListProcesses,
		ToolGetSystemInfo,
		ToolDownloadFile,
	},
}

var toolToGroup = func() map[string]string {
	m := make(map[string]string, len(AllTools()))
	for group, tools := range groupTools {
		for _, tool := range tools {
			m[tool] = group
		}
	}
	return m
}()

// AllGroups returns the canonical list of registrable tool groups, in the
// order they are normally registered. Useful for CLI output and docs.
func AllGroups() []string {
	return append([]string(nil), groupOrder...)
}

// AllTools returns the canonical list of registrable tool names, in group order.
func AllTools() []string {
	out := make([]string, 0, 44)
	for _, group := range groupOrder {
		out = append(out, groupTools[group]...)
	}
	return out
}

// ToolsForGroup returns the canonical tool names for a group.
// Unknown groups return nil.
func ToolsForGroup(group string) []string {
	tools, ok := groupTools[group]
	if !ok {
		return nil
	}
	return append([]string(nil), tools...)
}

// ToolGroup returns the group name for a tool.
func ToolGroup(name string) (string, bool) {
	group, ok := toolToGroup[name]
	return group, ok
}

// ValidateGroups returns an error if any name is not a known group.
// Empty input is valid (returns nil).
func ValidateGroups(names []string) error {
	known := make(map[string]bool, len(groupOrder))
	for _, g := range groupOrder {
		known[g] = true
	}
	var bad []string
	for _, n := range names {
		if !known[n] {
			bad = append(bad, n)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("unknown tool group(s): %s — valid groups: %s",
			strings.Join(bad, ", "), strings.Join(AllGroups(), ", "))
	}
	return nil
}

// ValidateToolNames returns an error if any name is not a known tool.
// Empty input is valid (returns nil).
func ValidateToolNames(names []string) error {
	var bad []string
	for _, n := range names {
		if _, ok := toolToGroup[n]; !ok {
			bad = append(bad, n)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("unknown tool name(s): %s — valid tools: %s",
			strings.Join(bad, ", "), strings.Join(AllTools(), ", "))
	}
	return nil
}
