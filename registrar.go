package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolRegistrar is the minimal interface needed to register MCP tools.
// It decouples registerXTools functions from the concrete *server.MCPServer,
// making them easier to test and future-proof against SDK changes.
type ToolRegistrar interface {
	AddTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}
