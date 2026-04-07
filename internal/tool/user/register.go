package user

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/internal/toolname"
	"github.com/bearaujus/bmcptools/internal/toolreg"
)

// Register registers all user interaction tools with s.
func Register(s toolreg.ToolRegistrar) {
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
			mcp.WithBoolean("allow_freeform",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "allow_freeform")),
			),
			mcp.WithBoolean("notify",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "notify")),
			),
			mcp.WithBoolean("non_blocking",
				mcp.Description(asset.ParamDesc(toolname.AskUser, "non_blocking")),
			),
		),
		askUserHandler,
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
		),
		updateDialogHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.OpenChat,
			mcp.WithDescription(asset.ToolDesc(toolname.OpenChat)),
			mcp.WithString("title",
				mcp.Description(asset.ParamDesc(toolname.OpenChat, "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(asset.ParamDesc(toolname.OpenChat, "subtitle")),
			),
		),
		openChatHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.SendChatMessage,
			mcp.WithDescription(asset.ToolDesc(toolname.SendChatMessage)),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.SendChatMessage, "chat_id")),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.SendChatMessage, "message")),
			),
			mcp.WithArray("suggestions",
				mcp.Description(asset.ParamDesc(toolname.SendChatMessage, "suggestions")),
				mcp.Items(map[string]any{"type": "string"}),
			),
		),
		sendChatMessageHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.GetChatMessages,
			mcp.WithDescription(asset.ToolDesc(toolname.GetChatMessages)),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.GetChatMessages, "chat_id")),
			),
			mcp.WithNumber("wait_seconds",
				mcp.Description(asset.ParamDesc(toolname.GetChatMessages, "wait_seconds")),
			),
		),
		getChatMessagesHandler,
	)

	s.AddTool(
		mcp.NewTool(toolname.CloseChat,
			mcp.WithDescription(asset.ToolDesc(toolname.CloseChat)),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(asset.ParamDesc(toolname.CloseChat, "chat_id")),
			),
		),
		closeChatHandler,
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
		restHandler,
	)
}
