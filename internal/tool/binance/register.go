package binance

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/bearaujus/bmcptools/pkg/toolreg"
)

// Register registers all binance_futures_* tools with s.
func Register(s toolreg.ToolRegistrar) {
	// --- Market data (no auth) ---
	s.AddTool(mcp.NewTool(toolname.BinanceFuturesPing,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesPing)),
	), pingHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesExchangeInfo,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesExchangeInfo)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesExchangeInfo, "symbol"))),
	), exchangeInfoHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesSymbolSpecs,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesSymbolSpecs)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesSymbolSpecs, "symbol"))),
	), symbolSpecsHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesKlines,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesKlines)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesKlines, "symbol"))),
		mcp.WithString("interval", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesKlines, "interval"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesKlines, "limit"))),
		mcp.WithNumber("start_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesKlines, "start_time_ms"))),
		mcp.WithNumber("end_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesKlines, "end_time_ms"))),
	), klinesHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesTickerPrice,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesTickerPrice)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesTickerPrice, "symbol"))),
	), tickerPriceHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesTicker24hr,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesTicker24hr)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesTicker24hr, "symbol"))),
	), ticker24hrHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesOrderBook,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesOrderBook)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOrderBook, "symbol"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOrderBook, "limit"))),
	), orderBookHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesMarkPrice,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesMarkPrice)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesMarkPrice, "symbol"))),
	), markPriceHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesOpenInterest,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesOpenInterest)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOpenInterest, "symbol"))),
		mcp.WithBoolean("history", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOpenInterest, "history"))),
		mcp.WithString("period", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOpenInterest, "period"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOpenInterest, "limit"))),
	), openInterestHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesLongShortRatio,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesLongShortRatio)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesLongShortRatio, "symbol"))),
		mcp.WithString("period", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesLongShortRatio, "period"))),
		mcp.WithString("mode", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesLongShortRatio, "mode"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesLongShortRatio, "limit"))),
	), longShortRatioHandler)

	// --- Account read (auth) ---
	s.AddTool(mcp.NewTool(toolname.BinanceFuturesAccountInfo,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesAccountInfo)),
	), accountInfoHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesPositionRisk,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesPositionRisk)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPositionRisk, "symbol"))),
	), positionRiskHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesOpenOrders,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesOpenOrders)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOpenOrders, "symbol"))),
	), openOrdersHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesOrderHistory,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesOrderHistory)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOrderHistory, "symbol"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOrderHistory, "limit"))),
		mcp.WithNumber("start_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOrderHistory, "start_time_ms"))),
		mcp.WithNumber("end_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesOrderHistory, "end_time_ms"))),
	), orderHistoryHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesIncomeHistory,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesIncomeHistory)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesIncomeHistory, "symbol"))),
		mcp.WithString("income_type", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesIncomeHistory, "income_type"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesIncomeHistory, "limit"))),
		mcp.WithNumber("start_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesIncomeHistory, "start_time_ms"))),
		mcp.WithNumber("end_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesIncomeHistory, "end_time_ms"))),
	), incomeHistoryHandler)

	// --- Config (gated) ---
	s.AddTool(mcp.NewTool(toolname.BinanceFuturesChangeLeverage,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesChangeLeverage)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangeLeverage, "symbol"))),
		mcp.WithNumber("leverage", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangeLeverage, "leverage"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangeLeverage, "reasoning"))),
	), changeLeverageHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesChangeMarginType,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesChangeMarginType)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangeMarginType, "symbol"))),
		mcp.WithString("margin_type", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangeMarginType, "margin_type"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangeMarginType, "reasoning"))),
	), changeMarginTypeHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesChangePositionMode,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesChangePositionMode)),
		mcp.WithBoolean("dual_side_position", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangePositionMode, "dual_side_position"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesChangePositionMode, "reasoning"))),
	), changePositionModeHandler)

	// --- Trade (gated) ---
	s.AddTool(mcp.NewTool(toolname.BinanceFuturesPlaceOrder,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesPlaceOrder)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "symbol"))),
		mcp.WithString("side", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "side"))),
		mcp.WithString("type", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "type"))),
		mcp.WithNumber("quantity", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "quantity"))),
		mcp.WithNumber("price", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "price"))),
		mcp.WithNumber("stop_price", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "stop_price"))),
		mcp.WithString("time_in_force", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "time_in_force"))),
		mcp.WithNumber("good_till_date_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "good_till_date_ms"))),
		mcp.WithBoolean("reduce_only", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "reduce_only"))),
		mcp.WithBoolean("close_position", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "close_position"))),
		mcp.WithString("position_side", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "position_side"))),
		mcp.WithString("working_type", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "working_type"))),
		mcp.WithNumber("activation_price", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "activation_price"))),
		mcp.WithNumber("callback_rate", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "callback_rate"))),
		mcp.WithString("self_trade_prevention_mode", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "self_trade_prevention_mode"))),
		mcp.WithString("price_match", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "price_match"))),
		mcp.WithBoolean("price_protect", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "price_protect"))),
		mcp.WithString("new_client_order_id", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "new_client_order_id"))),
		mcp.WithBoolean("dry_run", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "dry_run"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceOrder, "reasoning"))),
	), placeOrderHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesPlaceBracketOrder,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesPlaceBracketOrder)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "symbol"))),
		mcp.WithString("side", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "side"))),
		mcp.WithString("type", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "type"))),
		mcp.WithNumber("quantity", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "quantity"))),
		mcp.WithNumber("entry_price", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "entry_price"))),
		mcp.WithNumber("stop_loss_price", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "stop_loss_price"))),
		mcp.WithNumber("take_profit_price", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "take_profit_price"))),
		mcp.WithNumber("leverage", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "leverage"))),
		mcp.WithString("margin_type", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "margin_type"))),
		mcp.WithString("working_type", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "working_type"))),
		mcp.WithString("time_in_force", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "time_in_force"))),
		mcp.WithString("on_partial_failure", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "on_partial_failure"))),
		mcp.WithBoolean("dry_run", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "dry_run"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPlaceBracketOrder, "reasoning"))),
	), bracketOrderHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesCancelOrder,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesCancelOrder)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesCancelOrder, "symbol"))),
		mcp.WithString("order_id", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesCancelOrder, "order_id"))),
		mcp.WithString("orig_client_order_id", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesCancelOrder, "orig_client_order_id"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesCancelOrder, "reasoning"))),
	), cancelOrderHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesCancelAllOrders,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesCancelAllOrders)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesCancelAllOrders, "symbol"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesCancelAllOrders, "reasoning"))),
	), cancelAllOrdersHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesClosePosition,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesClosePosition)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesClosePosition, "symbol"))),
		mcp.WithNumber("quantity", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesClosePosition, "quantity"))),
		mcp.WithNumber("price", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesClosePosition, "price"))),
		mcp.WithString("time_in_force", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesClosePosition, "time_in_force"))),
		mcp.WithString("position_side", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesClosePosition, "position_side"))),
		mcp.WithBoolean("dry_run", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesClosePosition, "dry_run"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesClosePosition, "reasoning"))),
	), closePositionHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesCommissionRate,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesCommissionRate)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesCommissionRate, "symbol"))),
	), commissionRateHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesModifyOrder,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesModifyOrder)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesModifyOrder, "symbol"))),
		mcp.WithString("side", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesModifyOrder, "side"))),
		mcp.WithString("order_id", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesModifyOrder, "order_id"))),
		mcp.WithString("orig_client_order_id", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesModifyOrder, "orig_client_order_id"))),
		mcp.WithNumber("quantity", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesModifyOrder, "quantity"))),
		mcp.WithNumber("price", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesModifyOrder, "price"))),
		mcp.WithString("reasoning", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesModifyOrder, "reasoning"))),
	), modifyOrderHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesFundingRateHistory,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesFundingRateHistory)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesFundingRateHistory, "symbol"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesFundingRateHistory, "limit"))),
		mcp.WithNumber("start_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesFundingRateHistory, "start_time_ms"))),
		mcp.WithNumber("end_time_ms", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesFundingRateHistory, "end_time_ms"))),
	), fundingRateHistoryHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesPositionOverview,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesPositionOverview)),
		mcp.WithString("symbol", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesPositionOverview, "symbol"))),
	), positionOverviewHandler)

	s.AddTool(mcp.NewTool(toolname.BinanceFuturesTASnapshot,
		mcp.WithDescription(asset.ToolDesc(toolname.BinanceFuturesTASnapshot)),
		mcp.WithString("symbol", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesTASnapshot, "symbol"))),
		mcp.WithString("interval", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.BinanceFuturesTASnapshot, "interval"))),
		mcp.WithNumber("limit", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesTASnapshot, "limit"))),
		mcp.WithBoolean("include_candles", mcp.Description(asset.ParamDesc(toolname.BinanceFuturesTASnapshot, "include_candles"))),
	), taSnapshotHandler)
}
