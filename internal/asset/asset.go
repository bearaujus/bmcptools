package asset

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed descriptions/*.json
var descFS embed.FS

//go:embed descriptions/server_instructions.txt
var serverInstructionsTxt string

//go:embed html/dialog.html html/rest.html html/confirm.html html/md.css html/md.js
var htmlFS embed.FS

type toolEntry struct {
	Desc   string            `json:"desc"`
	Params map[string]string `json:"params"`
}

var descriptions = func() map[string]toolEntry {
	m := make(map[string]toolEntry)
	_ = fs.WalkDir(descFS, "descriptions", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		data, err := descFS.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("read embedded description %s: %v", path, err))
		}
		var entries map[string]toolEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			panic(fmt.Sprintf("parse embedded description %s: %v", path, err))
		}
		for k, v := range entries {
			m[k] = v
		}
		return nil
	})
	return m
}()

// ToolDesc returns the description for the named tool.
func ToolDesc(name string) string { return descriptions[name].Desc }

// ParamDesc returns the description of a parameter for the named tool.
func ParamDesc(tool, param string) string {
	if e, ok := descriptions[tool]; ok {
		return e.Params[param]
	}
	return ""
}

// ServerInstructions returns the full server instructions text (all groups).
func ServerInstructions() string {
	return renderInstructions(parseInstructionBlocks(serverInstructionsTxt), nil, nil)
}

// ServerInstructionsForGroups returns server instructions filtered to only
// the specified groups. The "intro" section is always included.
// Valid group names: "intro", "user", "file", "multi", "dir", "search", "exec", "system".
// Passing no groups returns the full instructions.
func ServerInstructionsForGroups(groups ...string) string {
	if len(groups) == 0 {
		return ServerInstructions()
	}
	want := make(map[string]bool, len(groups)+1)
	want["intro"] = true
	for _, g := range groups {
		want[g] = true
	}
	return renderInstructions(parseInstructionBlocks(serverInstructionsTxt), func(group string) bool {
		return want[group]
	}, nil)
}

// ServerInstructionsExcludingGroups returns server instructions with the
// specified groups removed. The "intro" section is always kept.
// Valid group names: "user", "file", "multi", "dir", "search", "exec", "system".
// Passing no groups returns the full instructions.
func ServerInstructionsExcludingGroups(groups ...string) string {
	return ServerInstructionsWithExclusions(groups, nil)
}

// ServerInstructionsExcludingTools returns server instructions with the
// specified tool blocks removed. Passing no tools returns the full instructions.
func ServerInstructionsExcludingTools(names ...string) string {
	return ServerInstructionsWithExclusions(nil, names)
}

// ServerInstructionsWithExclusions returns server instructions with the
// specified groups and tools removed. The "intro" section is always kept.
func ServerInstructionsWithExclusions(groups []string, tools []string) string {
	if len(groups) == 0 && len(tools) == 0 {
		return ServerInstructions()
	}
	skipGroups := make(map[string]bool, len(groups))
	for _, g := range groups {
		if g == "intro" {
			continue
		}
		skipGroups[g] = true
	}
	skipTools := make(map[string]bool, len(tools))
	for _, tool := range tools {
		skipTools[tool] = true
	}
	return renderInstructions(parseInstructionBlocks(serverInstructionsTxt), func(group string) bool {
		return !skipGroups[group]
	}, func(tool string) bool {
		return !skipTools[tool]
	})
}

type instructionBlock struct {
	group   string
	tool    string
	content string
}

// parseInstructionBlocks splits the instruction text by [[group:...]] and
// [[tool:...]] markers. Tool blocks are optional and may appear within groups.
func parseInstructionBlocks(text string) []instructionBlock {
	const (
		groupPrefix = "[[group:"
		toolPrefix  = "[[tool:"
		suffix      = "]]"
		toolEnd     = "[[/tool]]"
	)
	lines := strings.Split(text, "\n")

	var blocks []instructionBlock
	currentGroup := "intro"
	currentTool := ""
	var buf strings.Builder

	flush := func() {
		content := buf.String()
		buf.Reset()
		if strings.TrimSpace(content) == "" {
			return
		}
		blocks = append(blocks, instructionBlock{
			group:   currentGroup,
			tool:    currentTool,
			content: content,
		})
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, groupPrefix) && strings.HasSuffix(trimmed, suffix):
			flush()
			currentGroup = trimmed[len(groupPrefix) : len(trimmed)-len(suffix)]
			currentTool = ""
			continue
		case strings.HasPrefix(trimmed, toolPrefix) && strings.HasSuffix(trimmed, suffix):
			flush()
			currentTool = trimmed[len(toolPrefix) : len(trimmed)-len(suffix)]
			continue
		case trimmed == toolEnd:
			flush()
			currentTool = ""
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}
	flush()
	return blocks
}

func renderInstructions(blocks []instructionBlock, includeGroup func(string) bool, includeTool func(string) bool) string {
	var b strings.Builder
	for _, block := range blocks {
		if includeGroup != nil && !includeGroup(block.group) {
			continue
		}
		if block.tool != "" && includeTool != nil && !includeTool(block.tool) {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(block.content)
	}
	return b.String()
}

// HTML returns the contents of html/<name>.html from the embedded FS.
func HTML(name string) string {
	b, err := htmlFS.ReadFile("html/" + name + ".html")
	if err != nil {
		panic(fmt.Sprintf("embedded HTML %q not found: %v", name, err))
	}
	return string(b)
}

// CSS returns the contents of html/<name>.css from the embedded FS.
func CSS(name string) string {
	b, err := htmlFS.ReadFile("html/" + name + ".css")
	if err != nil {
		panic(fmt.Sprintf("embedded CSS %q not found: %v", name, err))
	}
	return string(b)
}

// JS returns the contents of html/<name>.js from the embedded FS.
func JS(name string) string {
	b, err := htmlFS.ReadFile("html/" + name + ".js")
	if err != nil {
		panic(fmt.Sprintf("embedded JS %q not found: %v", name, err))
	}
	return string(b)
}
