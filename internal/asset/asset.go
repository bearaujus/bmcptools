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
	return stripGroupMarkers(serverInstructionsTxt)
}

// ServerInstructionsForGroups returns server instructions filtered to only
// the specified groups. The "intro" section is always included.
// Valid group names: "intro", "user", "file", "dir", "search", "exec", "system".
// Passing no groups returns the full instructions.
func ServerInstructionsForGroups(groups ...string) string {
	if len(groups) == 0 {
		return stripGroupMarkers(serverInstructionsTxt)
	}
	want := make(map[string]bool, len(groups)+1)
	want["intro"] = true
	for _, g := range groups {
		want[g] = true
	}

	sections := parseGroupSections(serverInstructionsTxt)
	var b strings.Builder
	for _, sec := range sections {
		if want[sec.group] {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(sec.content)
		}
	}
	return b.String()
}

// ServerInstructionsExcludingGroups returns server instructions with the
// specified groups removed. The "intro" section is always kept.
// Valid group names: "user", "file", "dir", "search", "exec", "system".
// Passing no groups returns the full instructions.
func ServerInstructionsExcludingGroups(groups ...string) string {
	if len(groups) == 0 {
		return stripGroupMarkers(serverInstructionsTxt)
	}
	skip := make(map[string]bool, len(groups))
	for _, g := range groups {
		if g == "intro" {
			continue
		}
		skip[g] = true
	}

	sections := parseGroupSections(serverInstructionsTxt)
	var b strings.Builder
	for _, sec := range sections {
		if skip[sec.group] {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sec.content)
	}
	return b.String()
}

type groupSection struct {
	group   string
	content string
}

// parseGroupSections splits the instruction text by [[group:xxx]] markers.
func parseGroupSections(text string) []groupSection {
	const prefix = "[[group:"
	const suffix = "]]"
	lines := strings.Split(text, "\n")

	var sections []groupSection
	cur := groupSection{group: "intro"}
	var buf strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) && strings.HasSuffix(trimmed, suffix) {
			cur.content = buf.String()
			sections = append(sections, cur)
			name := trimmed[len(prefix) : len(trimmed)-len(suffix)]
			cur = groupSection{group: name}
			buf.Reset()
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}
	cur.content = buf.String()
	sections = append(sections, cur)
	return sections
}

// stripGroupMarkers removes [[group:xxx]] lines from the text.
func stripGroupMarkers(text string) string {
	const prefix = "[[group:"
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, prefix) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
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
