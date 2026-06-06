package multi

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

	assertToolHint(t, reg.tools[toolname.ReadMultipleFiles].Annotations.ReadOnlyHint, true, "read_multiple_files readOnlyHint")
	assertToolHint(t, reg.tools[toolname.WriteMultipleFiles].Annotations.DestructiveHint, true, "write_multiple_files destructiveHint")
	assertToolHint(t, reg.tools[toolname.WriteMultipleFiles].Annotations.IdempotentHint, true, "write_multiple_files idempotentHint")
	assertToolHint(t, reg.tools[toolname.PathExistsBatch].Annotations.ReadOnlyHint, true, "path_exists_batch readOnlyHint")
	assertToolHint(t, reg.tools[toolname.GetMultipleFileInfo].Annotations.ReadOnlyHint, true, "get_multiple_file_info readOnlyHint")
}

func assertToolHint(t *testing.T, got *bool, want bool, label string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
