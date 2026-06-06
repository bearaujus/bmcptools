package dir

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all directory tools with s.
func Register(s toolreg.ToolRegistrar) {
	s.AddTool(mcp.NewTool(toolname.ListDirectory,
		mcp.WithDescription(asset.ToolDesc(toolname.ListDirectory)),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("path", mcp.Description(asset.ParamDesc(toolname.ListDirectory, "path"))),
		mcp.WithBoolean("show_hidden", mcp.Description(asset.ParamDesc(toolname.ListDirectory, "show_hidden"))),
		mcp.WithBoolean("recursive", mcp.Description(asset.ParamDesc(toolname.ListDirectory, "recursive"))),
		mcp.WithNumber("max_depth", mcp.Description(asset.ParamDesc(toolname.ListDirectory, "max_depth"))),
		mcp.WithString("sort_by", mcp.Description(asset.ParamDesc(toolname.ListDirectory, "sort_by"))),
		mcp.WithString("glob", mcp.Description(asset.ParamDesc(toolname.ListDirectory, "glob"))),
		mcp.WithArray("exclude_patterns",
			mcp.Description(asset.ParamDesc(toolname.ListDirectory, "exclude_patterns")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_entries", mcp.Description(asset.ParamDesc(toolname.ListDirectory, "max_entries"))),
	), listDirHandler)

	s.AddTool(mcp.NewTool(toolname.CreateDirectory,
		mcp.WithDescription(asset.ToolDesc(toolname.CreateDirectory)),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.CreateDirectory, "path"))),
	), createDirHandler)

	s.AddTool(mcp.NewTool(toolname.DeleteDirectory,
		mcp.WithDescription(asset.ToolDesc(toolname.DeleteDirectory)),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.DeleteDirectory, "path"))),
		mcp.WithBoolean("force", mcp.Description(asset.ParamDesc(toolname.DeleteDirectory, "force"))),
	), deleteDirHandler)

	s.AddTool(mcp.NewTool(toolname.DirectoryTree,
		mcp.WithDescription(asset.ToolDesc(toolname.DirectoryTree)),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.DirectoryTree, "path"))),
		mcp.WithNumber("max_depth", mcp.Description(asset.ParamDesc(toolname.DirectoryTree, "max_depth"))),
		mcp.WithBoolean("show_hidden", mcp.Description(asset.ParamDesc(toolname.DirectoryTree, "show_hidden"))),
		mcp.WithArray("exclude_patterns",
			mcp.Description(asset.ParamDesc(toolname.DirectoryTree, "exclude_patterns")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("glob",
			mcp.Description(asset.ParamDesc(toolname.DirectoryTree, "glob")),
		),
		mcp.WithNumber("max_entries", mcp.Description(asset.ParamDesc(toolname.DirectoryTree, "max_entries"))),
	), dirTreeHandler)
}
