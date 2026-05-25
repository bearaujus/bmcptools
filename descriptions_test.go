package bmcptools

import (
	"strings"
	"testing"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// collectingRegistrar records the names of every tool added to it.
type collectingRegistrar struct {
	names []string
}

func (c *collectingRegistrar) AddTool(tool mcp.Tool, _ server.ToolHandlerFunc) {
	c.names = append(c.names, tool.Name)
}

// TestDescriptionsCoverage ensures every registered tool has a non-empty
// description entry in the embedded JSON assets so that silent mismatches
// between tool names and description keys are caught at test time.
func TestDescriptionsCoverage(t *testing.T) {
	r := &collectingRegistrar{}
	Register(r)

	for _, name := range r.names {
		if asset.ToolDesc(name) == "" {
			t.Errorf("tool %q is registered but has no description entry in internal/asset/descriptions/*.json", name)
		}
	}
}

// TestWithExcludeTools verifies that excluded tools are not registered.
func TestWithExcludeTools(t *testing.T) {
	all := &collectingRegistrar{}
	Register(all)
	totalCount := len(all.names)

	excluded := &collectingRegistrar{}
	Register(excluded, WithExcludeTools("read_file", "write_file"))

	if len(excluded.names) != totalCount-2 {
		t.Errorf("expected %d tools after excluding 2, got %d", totalCount-2, len(excluded.names))
	}
	for _, name := range excluded.names {
		if name == "read_file" || name == "write_file" {
			t.Errorf("excluded tool %q was registered", name)
		}
	}
}

// TestServerInstructionsForGroups verifies scoped instructions contain only requested groups.
func TestServerInstructionsForGroups(t *testing.T) {
	full := asset.ServerInstructions()
	if full == "" {
		t.Fatal("full instructions are empty")
	}
	if strings.Contains(full, "[[group:") || strings.Contains(full, "<!--") {
		t.Fatal("full instructions should not expose internal group markers")
	}

	fileOnly := asset.ServerInstructionsForGroups("file")
	if !strings.Contains(fileOnly, "File operations") {
		t.Error("file-group instructions should contain 'File operations'")
	}
	if strings.Contains(fileOnly, "User interaction") {
		t.Error("file-group instructions should NOT contain 'User interaction'")
	}
	if strings.Contains(fileOnly, "[[group:") || strings.Contains(fileOnly, "<!--") {
		t.Fatal("scoped instructions should not expose internal group markers")
	}

	noGroups := asset.ServerInstructionsForGroups()
	if noGroups == "" {
		t.Error("passing no groups should return full instructions")
	}
}
