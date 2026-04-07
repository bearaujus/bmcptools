package main

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func registerSystemTools(s ToolRegistrar) {
	// ── clipboard_write ───────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("clipboard_write",
		mcp.WithDescription(td("clipboard_write")),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description(pd("clipboard_write", "text")),
		),
	), clipboardWriteHandler)

	// ── clipboard_read ────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("clipboard_read",
		mcp.WithDescription(td("clipboard_read")),
	), clipboardReadHandler)

	// ── http_request ──────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("http_request",
		mcp.WithDescription(td("http_request")),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description(pd("http_request", "url")),
		),
		mcp.WithString("method",
			mcp.Description(pd("http_request", "method")),
		),
		mcp.WithObject("headers",
			mcp.Description(pd("http_request", "headers")),
		),
		mcp.WithString("body",
			mcp.Description(pd("http_request", "body")),
		),
		mcp.WithString("basic_auth",
			mcp.Description(pd("http_request", "basic_auth")),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description(pd("http_request", "timeout_seconds")),
		),
		mcp.WithBoolean("follow_redirects",
			mcp.Description(pd("http_request", "follow_redirects")),
		),
		mcp.WithBoolean("include_response_headers",
			mcp.Description(pd("http_request", "include_response_headers")),
		),
	), httpRequestHandler)

	// ── list_processes ────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("list_processes",
		mcp.WithDescription(td("list_processes")),
		mcp.WithString("filter",
			mcp.Description(pd("list_processes", "filter")),
		),
		mcp.WithString("sort_by",
			mcp.Description(pd("list_processes", "sort_by")),
		),
		mcp.WithNumber("limit",
			mcp.Description(pd("list_processes", "limit")),
		),
	), listProcessesHandler)

	s.AddTool(mcp.NewTool("get_system_info",
		mcp.WithDescription(td("get_system_info")),
	), getSystemInfoHandler)
}
