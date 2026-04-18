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

// Binance USDT-M Futures tools.
const (
	BinanceFuturesPing               = "binance_futures_ping"
	BinanceFuturesExchangeInfo       = "binance_futures_exchange_info"
	BinanceFuturesSymbolSpecs        = "binance_futures_symbol_specs"
	BinanceFuturesKlines             = "binance_futures_klines"
	BinanceFuturesTickerPrice        = "binance_futures_ticker_price"
	BinanceFuturesTicker24hr         = "binance_futures_ticker_24hr"
	BinanceFuturesOrderBook          = "binance_futures_order_book"
	BinanceFuturesMarkPrice          = "binance_futures_mark_price"
	BinanceFuturesOpenInterest       = "binance_futures_open_interest"
	BinanceFuturesLongShortRatio     = "binance_futures_long_short_ratio"
	BinanceFuturesAccountInfo        = "binance_futures_account_info"
	BinanceFuturesPositionRisk       = "binance_futures_position_risk"
	BinanceFuturesOpenOrders         = "binance_futures_open_orders"
	BinanceFuturesOrderHistory       = "binance_futures_order_history"
	BinanceFuturesIncomeHistory      = "binance_futures_income_history"
	BinanceFuturesChangeLeverage     = "binance_futures_change_leverage"
	BinanceFuturesChangeMarginType   = "binance_futures_change_margin_type"
	BinanceFuturesChangePositionMode = "binance_futures_change_position_mode"
	BinanceFuturesPlaceOrder         = "binance_futures_place_order"
	BinanceFuturesPlaceBracketOrder  = "binance_futures_place_bracket_order"
	BinanceFuturesCancelOrder        = "binance_futures_cancel_order"
	BinanceFuturesCancelAllOrders    = "binance_futures_cancel_all_open_orders"
	BinanceFuturesClosePosition      = "binance_futures_close_position"
	BinanceFuturesCommissionRate     = "binance_futures_commission_rate"
	BinanceFuturesModifyOrder        = "binance_futures_modify_order"
	BinanceFuturesFundingRateHistory = "binance_futures_funding_rate_history"
	BinanceFuturesPositionOverview   = "binance_futures_position_overview"
	BinanceFuturesTASnapshot         = "binance_futures_ta_snapshot"
)
