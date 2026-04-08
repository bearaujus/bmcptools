package toolreg

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolRegistrar is the minimal interface needed to register MCP tools.
type ToolRegistrar interface {
	AddTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}
