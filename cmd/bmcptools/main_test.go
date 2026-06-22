package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestNewMCPServerRecoveryPreservesToolCallID(t *testing.T) {
	s := newMCPServer("test instructions")
	s.AddTools(mcpserver.ServerTool{
		Tool: mcp.Tool{
			Name:        "panic-tool",
			Description: "panics to verify stdio recovery preserves the request id",
		},
		Handler: func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			panic("deliberate test panic")
		},
	})

	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"panic-tool","arguments":{}}}` + "\n",
	)
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mcpserver.NewStdioServer(s).Listen(ctx, input, &output); err != nil {
		t.Fatal(err)
	}
	if !stdioOutputHasErrorID(t, output.String(), "2") {
		t.Fatalf("panic response did not preserve tools/call id 2; output:\n%s", output.String())
	}
}

func stdioOutputHasErrorID(t *testing.T, output, wantID string) bool {
	t.Helper()

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var msg struct {
			ID    json.RawMessage `json:"id"`
			Error json.RawMessage `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatal(err)
		}
		if string(msg.ID) == wantID && len(msg.Error) > 0 {
			return true
		}
	}
	return false
}
