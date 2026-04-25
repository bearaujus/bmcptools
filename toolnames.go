package bmcptools

import "github.com/bearaujus/bmcptools/pkg/toolname"

// Tool name constants re-exported from pkg/toolname for convenient embedding.
// Importers can use these instead of importing pkg/toolname directly.
// pkg/toolname is the canonical source; this file is a re-export convenience layer.

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
	ToolCreateSymlink     = toolname.CreateSymlink
	ToolCompressFiles     = toolname.CompressFiles
	ToolExtractArchive    = toolname.ExtractArchive
)

// Multi-file tools.
const (
	ToolReadMultipleFiles   = toolname.ReadMultipleFiles
	ToolWriteMultipleFiles  = toolname.WriteMultipleFiles
	ToolFindReplaceInFiles  = toolname.FindReplaceInFiles
	ToolPathExistsBatch     = toolname.PathExistsBatch
	ToolGetMultipleFileInfo = toolname.GetMultipleFileInfo
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
	ToolGetEnv              = toolname.GetEnv
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
	ToolDownloadFile   = toolname.DownloadFile
)

// User interaction tools.
const (
	ToolNotifyUser      = toolname.NotifyUser
	ToolAskUser         = toolname.AskUser
	ToolGetUserResponse = toolname.GetUserResponse
	ToolUpdateDialog    = toolname.UpdateDialog
	ToolCancelAskUser   = toolname.CancelAskUser
	ToolRest            = toolname.Rest
)

// Binance USDT-M Futures tools.
const (
	ToolBinanceFuturesPing               = toolname.BinanceFuturesPing
	ToolBinanceFuturesExchangeInfo       = toolname.BinanceFuturesExchangeInfo
	ToolBinanceFuturesSymbolSpecs        = toolname.BinanceFuturesSymbolSpecs
	ToolBinanceFuturesKlines             = toolname.BinanceFuturesKlines
	ToolBinanceFuturesTickerPrice        = toolname.BinanceFuturesTickerPrice
	ToolBinanceFuturesTicker24hr         = toolname.BinanceFuturesTicker24hr
	ToolBinanceFuturesOrderBook          = toolname.BinanceFuturesOrderBook
	ToolBinanceFuturesMarkPrice          = toolname.BinanceFuturesMarkPrice
	ToolBinanceFuturesOpenInterest       = toolname.BinanceFuturesOpenInterest
	ToolBinanceFuturesLongShortRatio     = toolname.BinanceFuturesLongShortRatio
	ToolBinanceFuturesOpenOrders          = toolname.BinanceFuturesOpenOrders
	ToolBinanceFuturesOrderHistory        = toolname.BinanceFuturesOrderHistory
	ToolBinanceFuturesIncomeHistory       = toolname.BinanceFuturesIncomeHistory
	ToolBinanceFuturesConfigureSymbol     = toolname.BinanceFuturesConfigureSymbol
	ToolBinanceFuturesChangePositionMode  = toolname.BinanceFuturesChangePositionMode
	ToolBinanceFuturesPlaceOrder          = toolname.BinanceFuturesPlaceOrder
	ToolBinanceFuturesPlaceBracketOrder   = toolname.BinanceFuturesPlaceBracketOrder
	ToolBinanceFuturesCancelOrder         = toolname.BinanceFuturesCancelOrder
	ToolBinanceFuturesCancelAllOrders     = toolname.BinanceFuturesCancelAllOrders
	ToolBinanceFuturesClosePosition       = toolname.BinanceFuturesClosePosition
	ToolBinanceFuturesModifyOrder         = toolname.BinanceFuturesModifyOrder
	ToolBinanceFuturesFundingRateHistory  = toolname.BinanceFuturesFundingRateHistory
	ToolBinanceFuturesPositionOverview    = toolname.BinanceFuturesPositionOverview
	ToolBinanceFuturesTASnapshot          = toolname.BinanceFuturesTASnapshot
	ToolBinanceFuturesCancelAlgoOrder     = toolname.BinanceFuturesCancelAlgoOrder
	ToolBinanceFuturesPositionHealth      = toolname.BinanceFuturesPositionHealth
	ToolBinanceFuturesCalcOrderSize       = toolname.BinanceFuturesCalcOrderSize
	ToolBinanceFuturesDailySummary        = toolname.BinanceFuturesDailySummary
	ToolBinanceFuturesMarketScan          = toolname.BinanceFuturesMarketScan
	ToolBinanceFuturesTASnapshotMulti     = toolname.BinanceFuturesTASnapshotMulti
	ToolBinanceFuturesUpdateSLTP          = toolname.BinanceFuturesUpdateSLTP
	ToolBinanceFuturesPositionBrief       = toolname.BinanceFuturesPositionBrief
)
