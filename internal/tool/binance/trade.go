package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// placeOrderParams collects the params for a single /fapi/v1/order call.
type placeOrderParams struct {
	Symbol         string
	Side           string
	PositionSide   string
	Type           string
	Quantity       string
	Price          string
	StopPrice      string
	TimeInForce    string
	GoodTillDate   string
	ReduceOnly     string // "true" / "false" / ""
	ClosePosition  string
	WorkingType    string
	ActivationPrice string
	CallbackRate   string
	SelfTradePreventionMode string
	PriceMatch     string
	PriceProtect   string
	NewClientOrderID string
}

func (p placeOrderParams) toValues() url.Values {
	v := url.Values{}
	add := func(k, val string) {
		if val != "" {
			v.Set(k, val)
		}
	}
	add("symbol", p.Symbol)
	add("side", p.Side)
	add("positionSide", p.PositionSide)
	add("type", p.Type)
	add("quantity", p.Quantity)
	add("price", p.Price)
	add("stopPrice", p.StopPrice)
	add("timeInForce", p.TimeInForce)
	add("goodTillDate", p.GoodTillDate)
	add("reduceOnly", p.ReduceOnly)
	add("closePosition", p.ClosePosition)
	add("workingType", p.WorkingType)
	add("activationPrice", p.ActivationPrice)
	add("callbackRate", p.CallbackRate)
	add("selfTradePreventionMode", p.SelfTradePreventionMode)
	add("priceMatch", p.PriceMatch)
	add("priceProtect", p.PriceProtect)
	add("newClientOrderId", p.NewClientOrderID)
	return v
}

func readPlaceOrderParams(req mcp.CallToolRequest) (placeOrderParams, error) {
	p := placeOrderParams{
		Symbol:                  strings.ToUpper(strings.TrimSpace(req.GetString("symbol", ""))),
		Side:                    strings.ToUpper(strings.TrimSpace(req.GetString("side", ""))),
		PositionSide:            strings.ToUpper(strings.TrimSpace(req.GetString("position_side", ""))),
		Type:                    strings.ToUpper(strings.TrimSpace(req.GetString("type", ""))),
		Quantity:                trimFloatStr(req.GetFloat("quantity", 0)),
		Price:                   trimFloatStr(req.GetFloat("price", 0)),
		StopPrice:               trimFloatStr(req.GetFloat("stop_price", 0)),
		TimeInForce:             strings.ToUpper(strings.TrimSpace(req.GetString("time_in_force", ""))),
		GoodTillDate:            trimFloatStr(req.GetFloat("good_till_date_ms", 0)),
		WorkingType:             strings.ToUpper(strings.TrimSpace(req.GetString("working_type", ""))),
		ActivationPrice:         trimFloatStr(req.GetFloat("activation_price", 0)),
		CallbackRate:            trimFloatStr(req.GetFloat("callback_rate", 0)),
		SelfTradePreventionMode: strings.ToUpper(strings.TrimSpace(req.GetString("self_trade_prevention_mode", ""))),
		PriceMatch:              strings.ToUpper(strings.TrimSpace(req.GetString("price_match", ""))),
		NewClientOrderID:        strings.TrimSpace(req.GetString("new_client_order_id", "")),
	}
	if req.GetBool("reduce_only", false) {
		p.ReduceOnly = "true"
	}
	if req.GetBool("close_position", false) {
		p.ClosePosition = "true"
	}
	if req.GetBool("price_protect", false) {
		p.PriceProtect = "true"
	}
	if p.Symbol == "" || p.Side == "" || p.Type == "" {
		return p, fmt.Errorf("symbol, side and type are required")
	}
	if p.NewClientOrderID == "" {
		p.NewClientOrderID = newClientOrderID()
	}
	return p, nil
}

func trimFloatStr(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// placeOrderHandler places one order, with optional dry-run and confirmation.
func placeOrderHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p, err := readPlaceOrderParams(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	dryRun := req.GetBool("dry_run", false)
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if !dryRun && reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true or dry_run=true"), nil
	}

	if !dryRun {
		brief := buildPlaceOrderBrief(ctx, p, reasoning)
		ok, cerr := confirmOrSkip(ctx, "Place order", brief)
		if cerr != nil {
			return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
		}
		if !ok {
			return mcp.NewToolResultError("place_order rejected by user"), nil
		}
	}

	path := "/fapi/v1/order"
	if dryRun {
		path = "/fapi/v1/order/test"
	}

	body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: path, signed: true, params: p.toValues()})
	if err != nil {
		var be *binanceError
		if !dryRun && errors.As(err, &be) && be.HTTPStatus == 503 && strings.Contains(strings.ToLower(be.Msg), "unknown") {
			return reconcileOrder(ctx, p.Symbol, p.NewClientOrderID)
		}
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	prefix := ""
	if dryRun {
		// /fapi/v1/order/test returns an inflated empty struct on success — not useful to display.
		// Just confirm validation passed and echo the params we sent.
		footer := rateLimitFooter(hdr)
		return mcp.NewToolResultText(fmt.Sprintf(
			"[dry-run] validated by /fapi/v1/order/test (no execution)\nparams: %s%s",
			p.toValues().Encode(), footer,
		)), nil
	}
	return mcp.NewToolResultText(prefix + prettyJSON(body) + rateLimitFooter(hdr)), nil
}

// reconcileOrder handles 503-Unknown by polling for the client order ID.
func reconcileOrder(ctx context.Context, symbol, clientID string) (*mcp.CallToolResult, error) {
	for i := 0; i < 2; i++ {
		time.Sleep(1 * time.Second)
		body, _, err := request(ctx, requestOpts{
			method: http.MethodGet,
			path:   "/fapi/v1/order",
			signed: true,
			params: url.Values{"symbol": []string{symbol}, "origClientOrderId": []string{clientID}},
		})
		if err == nil {
			return mcp.NewToolResultText("[reconciled after 503-Unknown] " + prettyJSON(body)), nil
		}
	}
	return mcp.NewToolResultError(
		fmt.Sprintf("order status unknown after HTTP 503 (origClientOrderId=%s) — verify with binance_futures_open_orders / order_history before retrying", clientID),
	), nil
}

func buildPlaceOrderBrief(ctx context.Context, p placeOrderParams, reasoning string) string {
	notional := ""
	if p.Price != "" && p.Quantity != "" {
		if pr, err1 := strconv.ParseFloat(p.Price, 64); err1 == nil {
			if q, err2 := strconv.ParseFloat(p.Quantity, 64); err2 == nil {
				notional = fmt.Sprintf("%.4f %s", pr*q, "USDT (approx.)")
			}
		}
	}
	action := fmt.Sprintf("%s %s %s %s", p.Type, p.Side, p.Quantity, p.Symbol)
	if p.Price != "" {
		action += " @ " + p.Price
	}
	if p.StopPrice != "" {
		action += " stop=" + p.StopPrice
	}
	return tradeBrief{
		Action:    action,
		Notional:  notional,
		Market:    []string{"Free margin (USDT): " + dash(freeUSDT(ctx)), "Open positions: " + openPositionsSummary(ctx, p.Symbol)},
		Reasoning: reasoning,
	}.Render()
}

// bracketOrderHandler builds entry + SL + TP and posts them via /fapi/v1/batchOrders.
func bracketOrderHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	side := strings.ToUpper(strings.TrimSpace(req.GetString("side", "")))
	typ := strings.ToUpper(strings.TrimSpace(req.GetString("type", "MARKET")))
	qty := req.GetFloat("quantity", 0)
	entryPrice := req.GetFloat("entry_price", 0)
	sl := req.GetFloat("stop_loss_price", 0)
	tp := req.GetFloat("take_profit_price", 0)
	if symbol == "" || (side != "BUY" && side != "SELL") || qty <= 0 || sl <= 0 || tp <= 0 {
		return mcp.NewToolResultError("symbol, side=BUY|SELL, quantity, stop_loss_price, take_profit_price are required"), nil
	}
	if typ != "MARKET" && typ != "LIMIT" {
		return mcp.NewToolResultError("type must be MARKET or LIMIT"), nil
	}
	if typ == "LIMIT" && entryPrice <= 0 {
		return mcp.NewToolResultError("entry_price is required for LIMIT entry"), nil
	}

	dryRun := req.GetBool("dry_run", false)
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if !dryRun && reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true or dry_run=true"), nil
	}

	// Client-side validation: SL on the loss side, TP on the profit side.
	refPrice := entryPrice
	if typ == "MARKET" {
		refPrice = 0 // unknown — skip relative check when unknown
	}
	if refPrice > 0 {
		if side == "BUY" && !(sl < refPrice && tp > refPrice) {
			return mcp.NewToolResultError("for BUY: need stop_loss_price < entry_price < take_profit_price"), nil
		}
		if side == "SELL" && !(sl > refPrice && tp < refPrice) {
			return mcp.NewToolResultError("for SELL: need take_profit_price < entry_price < stop_loss_price"), nil
		}
	}

	leverage := int64(req.GetFloat("leverage", 0))
	marginType := strings.ToUpper(strings.TrimSpace(req.GetString("margin_type", "")))
	tif := strings.ToUpper(strings.TrimSpace(req.GetString("time_in_force", "GTC")))
	workingType := strings.ToUpper(strings.TrimSpace(req.GetString("working_type", "")))
	onFail := strings.ToLower(strings.TrimSpace(req.GetString("on_partial_failure", "rollback")))
	if onFail != "rollback" && onFail != "warn" {
		return mcp.NewToolResultError("on_partial_failure must be rollback or warn"), nil
	}

	if !dryRun {
		brief := tradeBrief{
			Action:    fmt.Sprintf("BRACKET %s %s %s %s (SL %s, TP %s)", typ, side, trimFloatStr(qty), symbol, trimFloatStr(sl), trimFloatStr(tp)),
			Leverage:  bracketLevString(leverage, marginType),
			Market:    []string{"Free margin (USDT): " + dash(freeUSDT(ctx)), "Open positions: " + openPositionsSummary(ctx, symbol)},
			Reasoning: reasoning,
		}.Render()
		ok, cerr := confirmOrSkip(ctx, "Place bracket order", brief)
		if cerr != nil {
			return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
		}
		if !ok {
			return mcp.NewToolResultError("place_bracket_order rejected by user"), nil
		}
	}

	// Optional prep: change margin type + leverage first.
	if marginType != "" {
		p := url.Values{"symbol": []string{symbol}, "marginType": []string{marginType}}
		if _, _, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/marginType", signed: true, params: p}); err != nil {
			// Code -4046 = "No need to change margin type" — harmless.
			var be *binanceError
			if !errors.As(err, &be) || be.Code != -4046 {
				return mcp.NewToolResultError("prep margin type: " + humanizeError(err)), nil
			}
		}
	}
	if leverage > 0 {
		p := url.Values{"symbol": []string{symbol}, "leverage": []string{strconv.FormatInt(leverage, 10)}}
		if _, _, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/leverage", signed: true, params: p}); err != nil {
			return mcp.NewToolResultError("prep leverage: " + humanizeError(err)), nil
		}
	}

	exitSide := "SELL"
	if side == "SELL" {
		exitSide = "BUY"
	}

	mkOrder := func(base map[string]interface{}) map[string]interface{} {
		base["symbol"] = symbol
		if base["newClientOrderId"] == nil {
			base["newClientOrderId"] = newClientOrderID()
		}
		return base
	}

	entryOrder := map[string]interface{}{
		"side":     side,
		"type":     typ,
		"quantity": trimFloatStr(qty),
	}
	if typ == "LIMIT" {
		entryOrder["price"] = trimFloatStr(entryPrice)
		entryOrder["timeInForce"] = tif
	}

	slOrder := map[string]interface{}{
		"side":          exitSide,
		"type":          "STOP_MARKET",
		"stopPrice":     trimFloatStr(sl),
		"closePosition": "true",
	}
	tpOrder := map[string]interface{}{
		"side":          exitSide,
		"type":          "TAKE_PROFIT_MARKET",
		"stopPrice":     trimFloatStr(tp),
		"closePosition": "true",
	}
	if workingType != "" {
		slOrder["workingType"] = workingType
		tpOrder["workingType"] = workingType
	}

	orders := []map[string]interface{}{mkOrder(entryOrder), mkOrder(slOrder), mkOrder(tpOrder)}

	if dryRun {
		// Only the entry leg is testable; /fapi/v1/order/test rejects STOP_MARKET / TAKE_PROFIT_MARKET (-4120).
		var results []string
		entryParams := url.Values{}
		for k, v := range orders[0] {
			entryParams.Set(k, fmt.Sprintf("%v", v))
		}
		if _, _, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/order/test", signed: true, params: entryParams}); err != nil {
			results = append(results, fmt.Sprintf("leg 0 (entry %s %s): ERROR %s", typ, side, humanizeError(err)))
		} else {
			results = append(results, fmt.Sprintf("leg 0 (entry %s %s qty=%s): validated by /fapi/v1/order/test", typ, side, trimFloatStr(qty)))
		}
		results = append(results,
			fmt.Sprintf("leg 1 (SL STOP_MARKET stopPrice=%s closePosition=true): validated locally — Binance /order/test does not accept conditional orders", trimFloatStr(sl)),
			fmt.Sprintf("leg 2 (TP TAKE_PROFIT_MARKET stopPrice=%s closePosition=true): validated locally — Binance /order/test does not accept conditional orders", trimFloatStr(tp)),
		)
		return mcp.NewToolResultText("[dry-run bracket]\n" + strings.Join(results, "\n")), nil
	}

	ordersJSON, err := json.Marshal(orders)
	if err != nil {
		return mcp.NewToolResultError("encode batchOrders: " + err.Error()), nil
	}
	params := url.Values{"batchOrders": []string{string(ordersJSON)}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/batchOrders", signed: true, params: params})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}

	// Parse per-leg results.
	var legResults []map[string]interface{}
	if err := json.Unmarshal(body, &legResults); err != nil {
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}
	var failedIdx []int
	var succeededIDs []string
	for i, lr := range legResults {
		if code, ok := lr["code"]; ok {
			if n, ok := code.(float64); ok && n != 0 {
				failedIdx = append(failedIdx, i)
				continue
			}
		}
		if cid, ok := lr["clientOrderId"].(string); ok && cid != "" {
			succeededIDs = append(succeededIDs, cid)
		}
	}
	if len(failedIdx) == 0 {
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}
	if onFail == "warn" {
		return mcp.NewToolResultText(fmt.Sprintf("[PARTIAL] legs failed=%v succeeded=%v\n%s%s", failedIdx, succeededIDs, prettyJSON(body), rateLimitFooter(hdr))), nil
	}
	// rollback: cancel succeeded legs.
	var rollback []string
	for _, cid := range succeededIDs {
		p := url.Values{"symbol": []string{symbol}, "origClientOrderId": []string{cid}}
		if _, _, cerr := request(ctx, requestOpts{method: http.MethodDelete, path: "/fapi/v1/order", signed: true, params: p}); cerr != nil {
			rollback = append(rollback, cid+":cancel_failed:"+humanizeError(cerr))
		} else {
			rollback = append(rollback, cid+":cancelled")
		}
	}
	return mcp.NewToolResultError(fmt.Sprintf("bracket failed on legs %v; rolled back: %v\noriginal response: %s", failedIdx, rollback, string(body))), nil
}

func bracketLevString(lev int64, marginType string) string {
	if lev <= 0 && marginType == "" {
		return ""
	}
	l := ""
	if lev > 0 {
		l = strconv.FormatInt(lev, 10) + "x"
	}
	return strings.TrimSpace(l + " " + marginType)
}

func cancelOrderHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	orderID := strings.TrimSpace(req.GetString("order_id", ""))
	clientID := strings.TrimSpace(req.GetString("orig_client_order_id", ""))
	if orderID == "" && clientID == "" {
		return mcp.NewToolResultError("either order_id or orig_client_order_id is required"), nil
	}
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}

	ref := orderID
	if ref == "" {
		ref = clientID
	}
	brief := tradeBrief{
		Action:    fmt.Sprintf("Cancel order %s on %s", ref, symbol),
		Reasoning: reasoning,
	}.Render()
	ok, err := confirmOrSkip(ctx, "Cancel order", brief)
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if !ok {
		return mcp.NewToolResultError("cancel_order rejected by user"), nil
	}

	p := url.Values{"symbol": []string{symbol}}
	if orderID != "" {
		p.Set("orderId", orderID)
	} else {
		p.Set("origClientOrderId", clientID)
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodDelete, path: "/fapi/v1/order", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}

func cancelAllOrdersHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}
	brief := tradeBrief{
		Action:    "Cancel ALL open orders on " + symbol,
		Reasoning: reasoning,
	}.Render()
	ok, err := confirmOrSkip(ctx, "Cancel all orders", brief)
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if !ok {
		return mcp.NewToolResultError("cancel_all_open_orders rejected by user"), nil
	}
	p := url.Values{"symbol": []string{symbol}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodDelete, path: "/fapi/v1/allOpenOrders", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}
