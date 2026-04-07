package toolname

// File tools.
const (
	ReadFile          = "read_file"
	WriteFile         = "write_file"
	AppendToFile      = "append_to_file"
	EditFile          = "edit_file"
	DeleteFile        = "delete_file"
	CopyFile          = "copy_file"
	MoveFile          = "move_file"
	GetFileInfo       = "get_file_info"
	PathExists        = "path_exists"
	DiffFiles         = "diff_files"
	CalculateChecksum = "calculate_checksum"
)

// Multi-file tools.
const (
	ReadMultipleFiles  = "read_multiple_files"
	WriteMultipleFiles = "write_multiple_files"
	FindReplaceInFiles = "find_replace_in_files"
)

// Directory tools.
const (
	ListDirectory   = "list_directory"
	CreateDirectory = "create_directory"
	DeleteDirectory = "delete_directory"
	DirectoryTree   = "directory_tree"
)

// Exec tools.
const (
	GetWorkingDirectory = "get_working_directory"
	RunCommand          = "run_command"
	OpenInApp           = "open_in_app"
)

// Search tools.
const (
	SearchFiles = "search_files"
	GrepFiles   = "grep_files"
)

// System tools.
const (
	ClipboardWrite = "clipboard_write"
	ClipboardRead  = "clipboard_read"
	HTTPRequest    = "http_request"
	ListProcesses  = "list_processes"
	GetSystemInfo  = "get_system_info"
)

// User interaction tools.
const (
	NotifyUser      = "notify_user"
	AskUser         = "ask_user"
	GetUserResponse = "get_user_response"
	UpdateDialog    = "update_dialog"
	OpenChat        = "open_chat"
	SendChatMessage = "send_chat_message"
	GetChatMessages = "get_chat_messages"
	CloseChat       = "close_chat"
	Rest            = "rest"
)
