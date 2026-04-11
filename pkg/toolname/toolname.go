// Package toolname provides string constants for every bmcptools MCP tool name.
// Import this package to reference tool names without hardcoding strings.
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
	CreateSymlink     = "create_symlink"
	CompressFiles     = "compress_files"
	ExtractArchive    = "extract_archive"
)

// Multi-file tools.
const (
	ReadMultipleFiles    = "read_multiple_files"
	WriteMultipleFiles   = "write_multiple_files"
	FindReplaceInFiles   = "find_replace_in_files"
	PathExistsBatch      = "path_exists_batch"
	GetMultipleFileInfo  = "get_multiple_file_info"
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
	GetEnv              = "get_env"
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
	DownloadFile   = "download_file"
)

// User interaction tools.
const (
	NotifyUser      = "notify_user"
	AskUser         = "ask_user"
	GetUserResponse = "get_user_response"
	UpdateDialog    = "update_dialog"
	CancelAskUser   = "cancel_ask_user"
	Rest            = "rest"
)
