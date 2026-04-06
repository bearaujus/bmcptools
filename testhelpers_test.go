package main

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// newTestRequest constructs a CallToolRequest with the given argument map.
func newTestRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// resultText extracts the first text content from a CallToolResult.
func resultText(r *mcp.CallToolResult) string {
	if r == nil {
		return ""
	}
	for _, c := range r.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			return tc.Text
		}
	}
	return ""
}

// isResultError returns true when the result carries the error flag.
func isResultError(r *mcp.CallToolResult) bool {
	return r != nil && r.IsError
}
