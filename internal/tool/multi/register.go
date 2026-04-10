package multi

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all multi-file tools with s.
func Register(s toolreg.ToolRegistrar) {
	s.AddTool(mcp.NewTool(toolname.ReadMultipleFiles,
		mcp.WithDescription(asset.ToolDesc(toolname.ReadMultipleFiles)),
		mcp.WithArray("paths",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.ReadMultipleFiles, "paths")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_bytes_per_file",
			mcp.Description(asset.ParamDesc(toolname.ReadMultipleFiles, "max_bytes_per_file")),
		),
	), readMultipleFilesHandler)

	s.AddTool(mcp.NewTool(toolname.WriteMultipleFiles,
		mcp.WithDescription(asset.ToolDesc(toolname.WriteMultipleFiles)),
		mcp.WithArray("files",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.WriteMultipleFiles, "files")),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "File path to write"},
					"content": map[string]any{"type": "string", "description": "Content to write"},
				},
				"required": []string{"path", "content"},
			}),
		),
		mcp.WithBoolean("create_dirs",
			mcp.Description(asset.ParamDesc(toolname.WriteMultipleFiles, "create_dirs")),
		),
		mcp.WithBoolean("show_diff",
			mcp.Description(asset.ParamDesc(toolname.WriteMultipleFiles, "show_diff")),
		),
	), writeMultipleFilesHandler)

	s.AddTool(mcp.NewTool(toolname.FindReplaceInFiles,
		mcp.WithDescription(asset.ToolDesc(toolname.FindReplaceInFiles)),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "path")),
		),
		mcp.WithString("old_str",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "old_str")),
		),
		mcp.WithString("new_str",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "new_str")),
		),
		mcp.WithBoolean("use_regex",
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "use_regex")),
		),
		mcp.WithBoolean("recursive",
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "recursive")),
		),
		mcp.WithString("glob",
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "glob")),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "dry_run")),
		),
		mcp.WithBoolean("show_diff",
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "show_diff")),
		),
		mcp.WithBoolean("show_hidden",
			mcp.Description(asset.ParamDesc(toolname.FindReplaceInFiles, "show_hidden")),
		),
	), findReplaceInFilesHandler)
}
