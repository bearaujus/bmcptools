package file

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all file tools with s.
func Register(s toolreg.ToolRegistrar) {
	s.AddTool(mcp.NewTool(toolname.ReadFile,
		mcp.WithDescription(asset.ToolDesc(toolname.ReadFile)),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.ReadFile, "path"))),
		mcp.WithNumber("start_line", mcp.Description(asset.ParamDesc(toolname.ReadFile, "start_line"))),
		mcp.WithNumber("end_line", mcp.Description(asset.ParamDesc(toolname.ReadFile, "end_line"))),
		mcp.WithNumber("max_bytes", mcp.Description(asset.ParamDesc(toolname.ReadFile, "max_bytes"))),
		mcp.WithNumber("head", mcp.Description(asset.ParamDesc(toolname.ReadFile, "head"))),
		mcp.WithNumber("tail", mcp.Description(asset.ParamDesc(toolname.ReadFile, "tail"))),
		mcp.WithBoolean("show_line_numbers", mcp.Description(asset.ParamDesc(toolname.ReadFile, "show_line_numbers"))),
		mcp.WithArray("ranges",
			mcp.Description(asset.ParamDesc(toolname.ReadFile, "ranges")),
			mcp.Items(map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			}),
		),
	), readFileHandler)

	s.AddTool(mcp.NewTool(toolname.WriteFile,
		mcp.WithDescription(asset.ToolDesc(toolname.WriteFile)),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.WriteFile, "path"))),
		mcp.WithString("content", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.WriteFile, "content"))),
		mcp.WithBoolean("create_dirs", mcp.Description(asset.ParamDesc(toolname.WriteFile, "create_dirs"))),
		mcp.WithBoolean("show_diff", mcp.Description(asset.ParamDesc(toolname.WriteFile, "show_diff"))),
		mcp.WithNumber("max_diff_bytes", mcp.Description(asset.ParamDesc(toolname.WriteFile, "max_diff_bytes"))),
	), writeFileHandler)

	s.AddTool(mcp.NewTool(toolname.AppendToFile,
		mcp.WithDescription(asset.ToolDesc(toolname.AppendToFile)),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.AppendToFile, "path"))),
		mcp.WithString("content", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.AppendToFile, "content"))),
	), appendFileHandler)

	s.AddTool(mcp.NewTool(toolname.EditFile,
		mcp.WithDescription(asset.ToolDesc(toolname.EditFile)),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.EditFile, "path"))),
		mcp.WithString("old_str", mcp.Description(asset.ParamDesc(toolname.EditFile, "old_str"))),
		mcp.WithString("new_str", mcp.Description(asset.ParamDesc(toolname.EditFile, "new_str"))),
		mcp.WithBoolean("use_regex", mcp.Description(asset.ParamDesc(toolname.EditFile, "use_regex"))),
		mcp.WithBoolean("replace_all", mcp.Description(asset.ParamDesc(toolname.EditFile, "replace_all"))),
		mcp.WithBoolean("dry_run", mcp.Description(asset.ParamDesc(toolname.EditFile, "dry_run"))),
		mcp.WithNumber("context_lines", mcp.Description(asset.ParamDesc(toolname.EditFile, "context_lines"))),
		mcp.WithNumber("max_diff_bytes", mcp.Description(asset.ParamDesc(toolname.EditFile, "max_diff_bytes"))),
		mcp.WithArray("edits",
			mcp.Description(asset.ParamDesc(toolname.EditFile, "edits")),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"old_str":     map[string]any{"type": "string", "description": "Text (or regex) to find"},
					"new_str":     map[string]any{"type": "string", "description": "Replacement text"},
					"use_regex":   map[string]any{"type": "boolean", "description": "Treat old_str as Go regex. Default: false"},
					"replace_all": map[string]any{"type": "boolean", "description": "Replace all occurrences. Default: false"},
				},
				"required": []string{"old_str", "new_str"},
			}),
		),
	), editFileHandler)

	s.AddTool(mcp.NewTool(toolname.DeleteFile,
		mcp.WithDescription(asset.ToolDesc(toolname.DeleteFile)),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.DeleteFile, "path"))),
	), deleteFileHandler)

	s.AddTool(mcp.NewTool(toolname.CopyFile,
		mcp.WithDescription(asset.ToolDesc(toolname.CopyFile)),
		mcp.WithString("source", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.CopyFile, "source"))),
		mcp.WithString("destination", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.CopyFile, "destination"))),
		mcp.WithBoolean("overwrite", mcp.Description(asset.ParamDesc(toolname.CopyFile, "overwrite"))),
	), copyFileHandler)

	s.AddTool(mcp.NewTool(toolname.MoveFile,
		mcp.WithDescription(asset.ToolDesc(toolname.MoveFile)),
		mcp.WithString("source", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.MoveFile, "source"))),
		mcp.WithString("destination", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.MoveFile, "destination"))),
		mcp.WithBoolean("overwrite", mcp.Description(asset.ParamDesc(toolname.MoveFile, "overwrite"))),
	), moveFileHandler)

	s.AddTool(mcp.NewTool(toolname.GetFileInfo,
		mcp.WithDescription(asset.ToolDesc(toolname.GetFileInfo)),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.GetFileInfo, "path"))),
	), getFileInfoHandler)

	s.AddTool(mcp.NewTool(toolname.PathExists,
		mcp.WithDescription(asset.ToolDesc(toolname.PathExists)),
		mcp.WithString("path", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.PathExists, "path"))),
	), pathExistsHandler)

	s.AddTool(mcp.NewTool(toolname.DiffFiles,
		mcp.WithDescription(asset.ToolDesc(toolname.DiffFiles)),
		mcp.WithString("path_a", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.DiffFiles, "path_a"))),
		mcp.WithString("path_b", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.DiffFiles, "path_b"))),
		mcp.WithNumber("context_lines", mcp.Description(asset.ParamDesc(toolname.DiffFiles, "context_lines"))),
		mcp.WithNumber("max_file_bytes", mcp.Description(asset.ParamDesc(toolname.DiffFiles, "max_file_bytes"))),
		mcp.WithNumber("max_diff_bytes", mcp.Description(asset.ParamDesc(toolname.DiffFiles, "max_diff_bytes"))),
	), diffFilesHandler)

	s.AddTool(mcp.NewTool(toolname.CalculateChecksum,
		mcp.WithDescription(asset.ToolDesc(toolname.CalculateChecksum)),
		mcp.WithArray("paths",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.CalculateChecksum, "paths")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("algorithm",
			mcp.Description(asset.ParamDesc(toolname.CalculateChecksum, "algorithm")),
		),
	), calculateChecksumHandler)

	s.AddTool(mcp.NewTool(toolname.CreateSymlink,
		mcp.WithDescription(asset.ToolDesc(toolname.CreateSymlink)),
		mcp.WithString("source", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.CreateSymlink, "source"))),
		mcp.WithString("link", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.CreateSymlink, "link"))),
	), createSymlinkHandler)

	s.AddTool(mcp.NewTool(toolname.CompressFiles,
		mcp.WithDescription(asset.ToolDesc(toolname.CompressFiles)),
		mcp.WithArray("paths",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.CompressFiles, "paths")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("output", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.CompressFiles, "output"))),
		mcp.WithString("format", mcp.Description(asset.ParamDesc(toolname.CompressFiles, "format"))),
	), compressFilesHandler)

	s.AddTool(mcp.NewTool(toolname.ExtractArchive,
		mcp.WithDescription(asset.ToolDesc(toolname.ExtractArchive)),
		mcp.WithString("archive", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.ExtractArchive, "archive"))),
		mcp.WithString("output", mcp.Description(asset.ParamDesc(toolname.ExtractArchive, "output"))),
		mcp.WithString("format", mcp.Description(asset.ParamDesc(toolname.ExtractArchive, "format"))),
	), extractArchiveHandler)
}
