package helper

import "github.com/mark3labs/mcp-go/mcp"

func newTestRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

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

func isResultError(r *mcp.CallToolResult) bool {
	return r != nil && r.IsError
}
