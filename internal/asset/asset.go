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

//go:embed html/dialog.html html/chat.html html/rest.html html/md.css html/md.js
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

// ServerInstructions returns the server instructions text.
func ServerInstructions() string { return serverInstructionsTxt }

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
