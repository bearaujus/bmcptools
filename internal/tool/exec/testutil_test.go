package exec

import (
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestMain suppresses real OS open-in-app commands during tests.
// Without this, TestOpenInAppHandlerValidTarget would call
// `cmd /C start "" <path>` on Windows, producing error dialogs for temp paths.
func TestMain(m *testing.M) {
	openInAppFn = func(target, app string) error { return nil } // no-op
	os.Exit(m.Run())
}

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
