package exec

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all exec tools with s.
func Register(s toolreg.ToolRegistrar) {
	s.AddTool(mcp.NewTool(toolname.GetWorkingDirectory,
		mcp.WithDescription(asset.ToolDesc(toolname.GetWorkingDirectory)),
	), getWorkingDirectoryHandler)

	s.AddTool(mcp.NewTool(toolname.RunCommand,
		mcp.WithDescription(asset.ToolDesc(toolname.RunCommand)),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "command")),
		),
		mcp.WithString("cwd",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "cwd")),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "timeout_seconds")),
		),
		mcp.WithArray("env",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "env")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_output_bytes",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "max_output_bytes")),
		),
		mcp.WithString("stdin",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "stdin")),
		),
		mcp.WithBoolean("allow_nonzero_exit",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "allow_nonzero_exit")),
		),
		mcp.WithBoolean("detach",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "detach")),
		),
		mcp.WithBoolean("raw_output",
			mcp.Description(asset.ParamDesc(toolname.RunCommand, "raw_output")),
		),
	), runCommandHandler)

	s.AddTool(mcp.NewTool(toolname.OpenInApp,
		mcp.WithDescription(asset.ToolDesc(toolname.OpenInApp)),
		mcp.WithString("target",
			mcp.Required(),
			mcp.Description(asset.ParamDesc(toolname.OpenInApp, "target")),
		),
		mcp.WithString("app",
			mcp.Description(asset.ParamDesc(toolname.OpenInApp, "app")),
		),
	), openInAppHandler)

	s.AddTool(mcp.NewTool(toolname.GetEnv,
		mcp.WithDescription(asset.ToolDesc(toolname.GetEnv)),
		mcp.WithString("key", mcp.Description(asset.ParamDesc(toolname.GetEnv, "key"))),
		mcp.WithString("filter", mcp.Description(asset.ParamDesc(toolname.GetEnv, "filter"))),
		mcp.WithBoolean("include_values", mcp.Description(asset.ParamDesc(toolname.GetEnv, "include_values"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.GetEnv, "limit"))),
		mcp.WithNumber("value_max_bytes", mcp.Description(asset.ParamDesc(toolname.GetEnv, "value_max_bytes"))),
		mcp.WithBoolean("redact_secrets", mcp.Description(asset.ParamDesc(toolname.GetEnv, "redact_secrets"))),
	), getEnvHandler)
}
