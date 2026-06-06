package file

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

	assertToolHint(t, reg.tools[toolname.ReadFile].Annotations.ReadOnlyHint, true, "read_file readOnlyHint")
	assertToolHint(t, reg.tools[toolname.WriteFile].Annotations.DestructiveHint, true, "write_file destructiveHint")
	assertToolHint(t, reg.tools[toolname.WriteFile].Annotations.IdempotentHint, true, "write_file idempotentHint")
	assertToolHint(t, reg.tools[toolname.GetFileInfo].Annotations.ReadOnlyHint, true, "get_file_info readOnlyHint")
	assertToolHint(t, reg.tools[toolname.CreateSymlink].Annotations.IdempotentHint, true, "create_symlink idempotentHint")
}

func assertToolHint(t *testing.T, got *bool, want bool, label string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
