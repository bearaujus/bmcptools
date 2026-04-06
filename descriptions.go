package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
)

//go:embed assets/descriptions/*.json
var descFS embed.FS

// toolEntry holds the description and parameter descriptions for one tool.
type toolEntry struct {
	Desc   string            `json:"desc"`
	Params map[string]string `json:"params"`
}

// descriptions is a global registry mapping tool name → toolEntry, loaded at startup.
var descriptions = func() map[string]toolEntry {
	m := make(map[string]toolEntry)
	_ = fs.WalkDir(descFS, "assets/descriptions", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := ""
		if len(path) > 5 {
			ext = path[len(path)-5:]
		}
		if ext != ".json" {
			return nil
		}
		data, err := descFS.ReadFile(path)
		if err != nil {
			log.Fatalf("read %s: %v", path, err)
		}
		var entries map[string]toolEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			log.Fatalf("parse %s: %v", path, err)
		}
		for k, v := range entries {
			m[k] = v
		}
		return nil
	})
	return m
}()

// td returns the tool description for the given tool name.
func td(name string) string {
	return descriptions[name].Desc
}

// pd returns the parameter description for the given tool and parameter name.
func pd(tool, param string) string {
	if e, ok := descriptions[tool]; ok {
		return e.Params[param]
	}
	return ""
}
