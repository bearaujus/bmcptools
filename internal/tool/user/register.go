package user

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all user interaction tools with s.
// opts can override the default HTML templates for ask_user and rest dialogs.
func Register(s toolreg.ToolRegistrar, opts ...Option) {
	cfg := userConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	dialogHTML := cfg.dialogHTMLSource()
	restHTML := cfg.restHTMLSource()
	s.AddTool(
		mcp.NewTool(toolname.NotifyUser,
			mcp.WithDescription(asset.ToolDesc(toolname.NotifyUser)),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.NotifyUser, "message")),
			),
			mcp.WithString("title",
				mcp.Description(asset.ParamDesc(toolname.NotifyUser, "title")),
			),
			mcp.WithString("level",
				mcp.Description(asset.ParamDesc(toolname.NotifyUser, "level")),
			),
			mcp.WithNumber("duration_seconds",
				mcp.Description(asset.ParamDesc(toolname.NotifyUser, "duration_seconds")),
			),
		),
		notifyUserHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.AskUser,
			mcp.WithDescription(asset.ToolDesc(toolname.AskUser)),
			mcp.WithString("question",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.AskUser, "question")),
			),
			mcp.WithString("details",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "details")),
			),
			mcp.WithString("title",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "subtitle")),
			),
			mcp.WithArray("choices",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "choices")),
				mcp.Items(map[string]any{"type": "string"}),
			),
			mcp.WithNumber("timeout_seconds",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "timeout_seconds")),
			),
			mcp.WithBoolean("notify",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "notify")),
			),
		),
		makeAskUserHandler(dialogHTML),
	)

	s.AddTool(
		mcp.NewTool(toolname.GetUserResponse,
			mcp.WithDescription(asset.ToolDesc(toolname.GetUserResponse)),
			mcp.WithString("token",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.GetUserResponse, "token")),
			),
			mcp.WithNumber("wait_seconds",
				mcp.Description(asset.ParamDesc(toolname.GetUserResponse, "wait_seconds")),
			),
			mcp.WithNumber("max_response_bytes",
				mcp.Description(asset.ParamDesc(toolname.GetUserResponse, "max_response_bytes")),
			),
		),
		getUserResponseHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.UpdateDialog,
			mcp.WithDescription(asset.ToolDesc(toolname.UpdateDialog)),
			mcp.WithString("token",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.UpdateDialog, "token")),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.UpdateDialog, "message")),
			),
			mcp.WithBoolean("replace_last",
				mcp.Description(asset.ParamDesc(toolname.UpdateDialog, "replace_last")),
			),
		),
		updateDialogHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.CancelAskUser,
			mcp.WithDescription(asset.ToolDesc(toolname.CancelAskUser)),
			mcp.WithString("token",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.CancelAskUser, "token")),
			),
		),
		cancelAskUserHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.Rest,
			mcp.WithDescription(asset.ToolDesc(toolname.Rest)),
			mcp.WithString("notes",
				mcp.Description(asset.ParamDesc(toolname.Rest, "notes")),
			),
			mcp.WithString("title",
				mcp.Description(asset.ParamDesc(toolname.Rest, "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(asset.ParamDesc(toolname.Rest, "subtitle")),
			),
			mcp.WithNumber("timeout_seconds",
				mcp.Description(asset.ParamDesc(toolname.Rest, "timeout_seconds")),
			),
		),
		makeRestHandler(restHTML),
	)
}
