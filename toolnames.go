package bmcptools

import "github.com/bearaujus/bmcptools/internal/toolname"

// Tool name constants — single source of truth for MCP tool identifiers.

// File tools.
const (
	ToolReadFile          = toolname.ReadFile
	ToolWriteFile         = toolname.WriteFile
	ToolAppendToFile      = toolname.AppendToFile
	ToolEditFile          = toolname.EditFile
	ToolDeleteFile        = toolname.DeleteFile
	ToolCopyFile          = toolname.CopyFile
	ToolMoveFile          = toolname.MoveFile
	ToolGetFileInfo       = toolname.GetFileInfo
	ToolPathExists        = toolname.PathExists
	ToolDiffFiles         = toolname.DiffFiles
	ToolCalculateChecksum = toolname.CalculateChecksum
)

// Multi-file tools.
const (
	ToolReadMultipleFiles  = toolname.ReadMultipleFiles
	ToolWriteMultipleFiles = toolname.WriteMultipleFiles
	ToolFindReplaceInFiles = toolname.FindReplaceInFiles
)

// Directory tools.
const (
	ToolListDirectory   = toolname.ListDirectory
	ToolCreateDirectory = toolname.CreateDirectory
	ToolDeleteDirectory = toolname.DeleteDirectory
	ToolDirectoryTree   = toolname.DirectoryTree
)

// Exec tools.
const (
	ToolGetWorkingDirectory = toolname.GetWorkingDirectory
	ToolRunCommand          = toolname.RunCommand
	ToolOpenInApp           = toolname.OpenInApp
)

// Search tools.
const (
	ToolSearchFiles = toolname.SearchFiles
	ToolGrepFiles   = toolname.GrepFiles
)

// System tools.
const (
	ToolClipboardWrite = toolname.ClipboardWrite
	ToolClipboardRead  = toolname.ClipboardRead
	ToolHTTPRequest    = toolname.HTTPRequest
	ToolListProcesses  = toolname.ListProcesses
	ToolGetSystemInfo  = toolname.GetSystemInfo
)

// User interaction tools.
const (
	ToolNotifyUser      = toolname.NotifyUser
	ToolAskUser         = toolname.AskUser
	ToolGetUserResponse = toolname.GetUserResponse
	ToolUpdateDialog    = toolname.UpdateDialog
	ToolCancelAskUser   = toolname.CancelAskUser
	ToolOpenChat        = toolname.OpenChat
	ToolSendChatMessage = toolname.SendChatMessage
	ToolGetChatMessages = toolname.GetChatMessages
	ToolCloseChat       = toolname.CloseChat
	ToolRest            = toolname.Rest
)
