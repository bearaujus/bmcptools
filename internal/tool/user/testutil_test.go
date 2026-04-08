package user

import (
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestMain suppresses real browser windows and OS notifications for all tests
// in this package. Without this, every call to askUserHandler / notifyUserHandler
// would open a browser tab or fire a system notification on the developer's machine.
func TestMain(m *testing.M) {
	openBrowserFn = func(url string) {}             // no-op: don't open real browsers
	sendNotificationFn = func(_, _, _ string, _ int) {} // no-op: don't fire OS notifications
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
