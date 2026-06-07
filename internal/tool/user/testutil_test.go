package user

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestMain suppresses real browser windows and OS notifications for all tests
// in this package. Without this, every call to askUserHandler / notifyUserHandler
// would open a browser tab or fire a system notification on the developer's machine.
func TestMain(m *testing.M) {
	openBrowserFn = func(url string) error { return nil } // no-op: don't open real browsers
	sendNotificationFn = func(_, _, _ string, _ int) {}   // no-op: don't fire OS notifications
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

func mustNewDialogToken(t *testing.T) string {
	t.Helper()
	token, err := newDialogToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func resultToken(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(resultText(r)), &payload); err != nil {
		t.Fatalf("failed to parse token from result: %v", err)
	}
	if payload.Token == "" {
		t.Fatalf("result did not include token: %q", resultText(r))
	}
	return payload.Token
}
