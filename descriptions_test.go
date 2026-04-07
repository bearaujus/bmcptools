package bmcptools

import (
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
