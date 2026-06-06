package dir

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/bearaujus/bmcptools/pkg/toolname"
)

type captureRegistrar struct {
	tools map[string]mcp.Tool
}

func (c *captureRegistrar) AddTool(tool mcp.Tool, _ server.ToolHandlerFunc) {
	if c.tools == nil {
		c.tools = make(map[string]mcp.Tool)
	}
	c.tools[tool.Name] = tool
}

func TestRegisterAddsAnnotations(t *testing.T) {
	reg := &captureRegistrar{}
	Register(reg)

	assertToolHint(t, reg.tools[toolname.ListDirectory].Annotations.ReadOnlyHint, true, "list_directory readOnlyHint")
	assertToolHint(t, reg.tools[toolname.CreateDirectory].Annotations.IdempotentHint, true, "create_directory idempotentHint")
	assertToolHint(t, reg.tools[toolname.DeleteDirectory].Annotations.DestructiveHint, true, "delete_directory destructiveHint")
	assertToolHint(t, reg.tools[toolname.DirectoryTree].Annotations.ReadOnlyHint, true, "directory_tree readOnlyHint")
}

func assertToolHint(t *testing.T, got *bool, want bool, label string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
