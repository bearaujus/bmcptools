package main

import "github.com/mark3labs/mcp-go/mcp"

func registerUserTools(s ToolRegistrar) {
	s.AddTool(
		mcp.NewTool("notify_user",
			mcp.WithDescription(td("notify_user")),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(pd("notify_user", "message")),
			),
			mcp.WithString("title",
				mcp.Description(pd("notify_user", "title")),
			),
			mcp.WithString("level",
				mcp.Description(pd("notify_user", "level")),
			),
			mcp.WithNumber("duration_seconds",
				mcp.Description(pd("notify_user", "duration_seconds")),
			),
		),
		notifyUserHandler,
	)

	s.AddTool(
		mcp.NewTool("ask_user",
			mcp.WithDescription(td("ask_user")),
			mcp.WithString("question",
				mcp.Required(),
				mcp.Description(pd("ask_user", "question")),
			),
			mcp.WithString("title",
				mcp.Description(pd("ask_user", "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(pd("ask_user", "subtitle")),
			),
			mcp.WithArray("choices",
				mcp.Description(pd("ask_user", "choices")),
				mcp.Items(map[string]any{"type": "string"}),
			),
			mcp.WithNumber("timeout_seconds",
				mcp.Description(pd("ask_user", "timeout_seconds")),
			),
			mcp.WithBoolean("allow_freeform",
				mcp.Description(pd("ask_user", "allow_freeform")),
			),
			mcp.WithBoolean("notify",
				mcp.Description(pd("ask_user", "notify")),
			),
			mcp.WithBoolean("non_blocking",
				mcp.Description(pd("ask_user", "non_blocking")),
			),
		),
		askUserHandler,
	)

	s.AddTool(
		mcp.NewTool("get_user_response",
			mcp.WithDescription(td("get_user_response")),
			mcp.WithString("token",
				mcp.Required(),
				mcp.Description(pd("get_user_response", "token")),
			),
			mcp.WithNumber("wait_seconds",
				mcp.Description(pd("get_user_response", "wait_seconds")),
			),
		),
		getUserResponseHandler,
	)

	s.AddTool(
		mcp.NewTool("update_dialog",
			mcp.WithDescription(td("update_dialog")),
			mcp.WithString("token",
				mcp.Required(),
				mcp.Description(pd("update_dialog", "token")),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(pd("update_dialog", "message")),
			),
		),
		updateDialogHandler,
	)

	s.AddTool(
		mcp.NewTool("open_chat",
			mcp.WithDescription(td("open_chat")),
			mcp.WithString("title",
				mcp.Description(pd("open_chat", "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(pd("open_chat", "subtitle")),
			),
		),
		openChatHandler,
	)

	s.AddTool(
		mcp.NewTool("send_chat_message",
			mcp.WithDescription(td("send_chat_message")),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(pd("send_chat_message", "chat_id")),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description(pd("send_chat_message", "message")),
			),
			mcp.WithArray("suggestions",
				mcp.Description(pd("send_chat_message", "suggestions")),
				mcp.Items(map[string]any{"type": "string"}),
			),
		),
		sendChatMessageHandler,
	)

	s.AddTool(
		mcp.NewTool("get_chat_messages",
			mcp.WithDescription(td("get_chat_messages")),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(pd("get_chat_messages", "chat_id")),
			),
			mcp.WithNumber("wait_seconds",
				mcp.Description(pd("get_chat_messages", "wait_seconds")),
			),
		),
		getChatMessagesHandler,
	)

	s.AddTool(
		mcp.NewTool("close_chat",
			mcp.WithDescription(td("close_chat")),
			mcp.WithString("chat_id",
				mcp.Required(),
				mcp.Description(pd("close_chat", "chat_id")),
			),
		),
		closeChatHandler,
	)

	s.AddTool(
		mcp.NewTool("rest",
			mcp.WithDescription(td("rest")),
			mcp.WithString("notes",
				mcp.Description(pd("rest", "notes")),
			),
			mcp.WithString("title",
				mcp.Description(pd("rest", "title")),
			),
			mcp.WithString("subtitle",
				mcp.Description(pd("rest", "subtitle")),
			),
			mcp.WithNumber("timeout_seconds",
				mcp.Description(pd("rest", "timeout_seconds")),
			),
		),
		restHandler,
	)
}
