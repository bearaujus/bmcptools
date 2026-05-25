package search

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all search tools with s.
func Register(s toolreg.ToolRegistrar) {
	s.AddTool(mcp.NewTool(toolname.SearchFiles,
		mcp.WithDescription(asset.ToolDesc(toolname.SearchFiles)),
		mcp.WithString("path", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "path"))),
		mcp.WithString("pattern", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.SearchFiles, "pattern"))),
		mcp.WithBoolean("recursive", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "recursive"))),
		mcp.WithNumber("max_results", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "max_results"))),
		mcp.WithBoolean("show_hidden", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "show_hidden"))),
		mcp.WithBoolean("use_regex", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "use_regex"))),
		mcp.WithBoolean("case_insensitive", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "case_insensitive"))),
		mcp.WithArray("exclude_patterns",
			mcp.Description(asset.ParamDesc(toolname.SearchFiles, "exclude_patterns")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("output_mode", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "output_mode"))),
		mcp.WithString("path_format", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "path_format"))),
		mcp.WithString("entry_type", mcp.Description(asset.ParamDesc(toolname.SearchFiles, "entry_type"))),
	), searchFilesHandler)

	s.AddTool(mcp.NewTool(toolname.GrepFiles,
		mcp.WithDescription(asset.ToolDesc(toolname.GrepFiles)),
		mcp.WithString("path", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "path"))),
		mcp.WithString("pattern", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.GrepFiles, "pattern"))),
		mcp.WithBoolean("use_regex", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "use_regex"))),
		mcp.WithBoolean("case_insensitive", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "case_insensitive"))),
		mcp.WithBoolean("recursive", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "recursive"))),
		mcp.WithNumber("context_lines", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "context_lines"))),
		mcp.WithNumber("max_results", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "max_results"))),
		mcp.WithNumber("offset", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "offset"))),
		mcp.WithString("glob", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "glob"))),
		mcp.WithString("output_mode", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "output_mode"))),
		mcp.WithBoolean("show_hidden", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "show_hidden"))),
		mcp.WithBoolean("multiline", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "multiline"))),
		mcp.WithNumber("max_file_size", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "max_file_size"))),
		mcp.WithArray("exclude_patterns",
			mcp.Description(asset.ParamDesc(toolname.GrepFiles, "exclude_patterns")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("path_format", mcp.Description(asset.ParamDesc(toolname.GrepFiles, "path_format"))),
	), grepFilesHandler)
}
