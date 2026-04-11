package system

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all system tools with s.
func Register(s toolreg.ToolRegistrar) {
	s.AddTool(mcp.NewTool(toolname.ClipboardWrite,
		mcp.WithDescription(asset.ToolDesc(toolname.ClipboardWrite)),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.ClipboardWrite, "text")),
		),
	), clipboardWriteHandler)

	s.AddTool(mcp.NewTool(toolname.ClipboardRead,
		mcp.WithDescription(asset.ToolDesc(toolname.ClipboardRead)),
	), clipboardReadHandler)

	s.AddTool(mcp.NewTool(toolname.HTTPRequest,
		mcp.WithDescription(asset.ToolDesc(toolname.HTTPRequest)),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "url")),
		),
		mcp.WithString("method",
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "method")),
		),
		mcp.WithObject("headers",
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "headers")),
		),
		mcp.WithString("body",
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "body")),
		),
		mcp.WithString("basic_auth",
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "basic_auth")),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "timeout_seconds")),
		),
		mcp.WithBoolean("follow_redirects",
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "follow_redirects")),
		),
		mcp.WithBoolean("include_response_headers",
			mcp.Description(asset.ParamDesc(toolname.HTTPRequest, "include_response_headers")),
		),
	), httpRequestHandler)

	s.AddTool(mcp.NewTool(toolname.ListProcesses,
		mcp.WithDescription(asset.ToolDesc(toolname.ListProcesses)),
		mcp.WithString("filter",
			mcp.Description(asset.ParamDesc(toolname.ListProcesses, "filter")),
		),
		mcp.WithString("sort_by",
			mcp.Description(asset.ParamDesc(toolname.ListProcesses, "sort_by")),
		),
		mcp.WithNumber("limit",
			mcp.Description(asset.ParamDesc(toolname.ListProcesses, "limit")),
		),
	), listProcessesHandler)

	s.AddTool(mcp.NewTool(toolname.GetSystemInfo,
		mcp.WithDescription(asset.ToolDesc(toolname.GetSystemInfo)),
	), getSystemInfoHandler)

	s.AddTool(mcp.NewTool(toolname.DownloadFile,
		mcp.WithDescription(asset.ToolDesc(toolname.DownloadFile)),
		mcp.WithString("url", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.DownloadFile, "url"))),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.DownloadFile, "path"))),
		mcp.WithObject("headers", mcp.Description(asset.ParamDesc(toolname.DownloadFile, "headers"))),
		mcp.WithNumber("timeout_seconds", mcp.Description(asset.ParamDesc(toolname.DownloadFile, "timeout_seconds"))),
		mcp.WithBoolean("overwrite", mcp.Description(asset.ParamDesc(toolname.DownloadFile, "overwrite"))),
	), downloadFileHandler)
}
