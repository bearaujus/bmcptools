// Package toolreg exposes the ToolRegistrar interface used to register MCP tools.
// It is a public mirror of the internal interface so that external connectors
// can accept and pass the interface without importing internal packages.
package toolreg

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolRegistrar is the minimal interface needed to register MCP tools.
// It decouples Register functions from the concrete *server.MCPServer,
// making them easier to test and future-proof against SDK changes.
type ToolRegistrar interface {
	AddTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}
