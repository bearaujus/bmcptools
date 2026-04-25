package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// placeOrderParams collects the params for a single /fapi/v1/order call.
type placeOrderParams struct {
	Symbol                  string
	Side                    string
	PositionSide            string
	Type                    string
	Quantity                string
	Price                   string
	StopPrice               string
	TimeInForce             string
	GoodTillDate            string
	ReduceOnly              string // "true" / "false" / ""
	ClosePosition           string
	WorkingType             string
	ActivationPrice         string
	CallbackRate            string
	SelfTradePreventionMode string
	PriceMatch              string
	PriceProtect            string
	NewClientOrderID        string
}

// legResult holds the outcome of one bracket leg placement.
type legResult struct {
	body []byte
	hdr  http.Header
	err  error
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

func isBinanceCode(err error, code int) bool {
	var be *binanceError
	return errors.As(err, &be) && be.Code == code
}

// placeAlgoConditional places a single SL or TP order directly via POST /fapi/v1/algoOrder.
// Since Dec 2025, Binance requires all STOP_MARKET/TAKE_PROFIT_MARKET to go through the Algo API.
// Uses closePosition=true (no fixed quantity) so the order always closes the full remaining
// position — matches Binance UI "TP/SL for position" behavior and avoids -2022 ReduceOnly
// rejections if the user partially closes the position manually between entry and trigger.
func placeAlgoConditional(ctx context.Context, symbol, side, orderType string, triggerPrice float64, workingType string) ([]byte, error) {
	// closePosition=true and reduceOnly are mutually exclusive on the Algo API (-1106).
	params := url.Values{
		"symbol":        {symbol},
		"side":          {side},
		"algoType":      {"CONDITIONAL"},
		"type":          {orderType},
		"closePosition": {"true"},
		"triggerPrice":  {trimFloatStr(triggerPrice)},
	}
	if workingType != "" {
		params.Set("workingType", workingType)
	}
	b, _, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/algoOrder", signed: true, params: params})
	if err != nil {
		return b, fmt.Errorf("algo order %s failed: %w | raw: %s", orderType, err, string(b))
	}
	return b, nil
}

// cancelAlgoOrder cancels a single algo order by algoId.
func cancelAlgoOrder(ctx context.Context, algoID string) error {
	_, _, err := request(ctx, requestOpts{
		method: http.MethodDelete,
		path:   "/fapi/v1/algoOrder",
		signed: true,
		params: url.Values{"algoId": {algoID}},
	})
	return err
}

// fetchOpenAlgoOrders returns all open conditional algo orders for a symbol.
// UseNumber is set on the decoder to preserve full integer precision for algoId.
func fetchOpenAlgoOrders(ctx context.Context, symbol string) ([]map[string]interface{}, error) {
	p := url.Values{}
	if symbol != "" {
		p.Set("symbol", symbol)
	}
	b, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/openAlgoOrders", signed: true, params: p})
	if err != nil {
		return nil, err
	}
	var rows []map[string]interface{}
	if err := jsonDecoderUseNumber(b).Decode(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// reconcileOrder handles 503-Unknown by polling for the client order ID using
// Binance-recommended exponential backoff (200ms → 400ms → 800ms).
func reconcileOrder(ctx context.Context, symbol, clientID string) (*mcp.CallToolResult, error) {
	delays := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond}
	for _, d := range delays {
		time.Sleep(d)
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
		fmt.Sprintf("order status unknown after HTTP 503 (origClientOrderId=%s) — check binance_futures_open_orders / order_history before retrying", clientID),
	), nil
}

// =====================================================================
// place_order — single order, MARKET/LIMIT only (not conditional).
// For conditional (STOP_MARKET, TAKE_PROFIT_MARKET), use bracket order.
// =====================================================================

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

	// Pre-validate minimum notional (best-effort; skipped for reduce-only/close-position).
	isReduceOnly := p.ReduceOnly == "true" || p.ClosePosition == "true"
	if merr := checkMinNotional(ctx, p.Symbol, req.GetFloat("quantity", 0), p.Type, p.Price, isReduceOnly); merr != nil {
		return mcp.NewToolResultError(merr.Error()), nil
	}

	var confirmOutcome confirmResult
	if !dryRun {
		ep := make([]editParam, 0, 3)
		if p.Quantity != "" {
			ep = append(ep, editParam{Key: "quantity", Label: "Quantity", Value: p.Quantity, Type: "number", Step: "any"})
		}
		if p.Price != "" {
			ep = append(ep, editParam{Key: "price", Label: "Price", Value: p.Price, Type: "number", Step: "any"})
		}
		if p.StopPrice != "" {
			ep = append(ep, editParam{Key: "stop_price", Label: "Stop Price", Value: p.StopPrice, Type: "number", Step: "any"})
		}
		brief := buildPlaceOrderBrief(ctx, p, reasoning)
		var editedParams map[string]string
		var cerr error
		confirmOutcome, editedParams, cerr = confirmOrSkipEditable(ctx, "Place order", brief, ep)
		if cerr != nil {
			return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
		}
		if confirmOutcome == confirmTimedOut {
			return mcp.NewToolResultError("place_order confirmation timed out — user did not respond"), nil
		}
		if confirmOutcome == confirmHumanRejected {
			return mcp.NewToolResultError("place_order rejected by user"), nil
		}
		p.Quantity = editedFloatStr(editedParams, "quantity", p.Quantity)
		p.Price = editedFloatStr(editedParams, "price", p.Price)
		p.StopPrice = editedFloatStr(editedParams, "stop_price", p.StopPrice)
	}
	path := "/fapi/v1/order"
	if dryRun {
		path = "/fapi/v1/order/test"
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: path, signed: true, params: p.toValues()})
	if err != nil {
		if !dryRun {
			var be *binanceError
			if errors.As(err, &be) && be.HTTPStatus == 503 && strings.Contains(strings.ToLower(be.Msg), "unknown") {
				return reconcileOrder(ctx, p.Symbol, p.NewClientOrderID)
			}
		}
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	if dryRun {
		return mcp.NewToolResultText(fmt.Sprintf("[dry-run] validated OK\nparams: %s%s", p.toValues().Encode(), rateLimitFooter(hdr))), nil
	}
	return mcp.NewToolResultText(confirmOutcome.prefix() + prettyJSON(body) + rateLimitFooter(hdr)), nil
}

func buildPlaceOrderBrief(ctx context.Context, p placeOrderParams, reasoning string) string {
	notional := ""
	if p.Price != "" && p.Quantity != "" {
		if pr, err1 := strconv.ParseFloat(p.Price, 64); err1 == nil {
			if q, err2 := strconv.ParseFloat(p.Quantity, 64); err2 == nil {
				notional = fmt.Sprintf("%.4f USDT (approx.)", pr*q)
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

// =====================================================================
// place_bracket_order — entry + SL + TP atomically.
// Entry via /fapi/v1/order, SL/TP directly via /fapi/v1/algoOrder.
// =====================================================================

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
	if typ == "LIMIT" {
		if side == "BUY" && !(sl < entryPrice && tp > entryPrice) {
			return mcp.NewToolResultError("BUY bracket: stop_loss_price < entry_price < take_profit_price required"), nil
		}
		if side == "SELL" && !(tp < entryPrice && sl > entryPrice) {
			return mcp.NewToolResultError("SELL bracket: take_profit_price < entry_price < stop_loss_price required"), nil
		}
	}

	dryRun := req.GetBool("dry_run", false)
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if !dryRun && reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true or dry_run=true"), nil
	}

	// Pre-validate minimum notional (best-effort).
	notionalPrice := trimFloatStr(entryPrice) // empty for MARKET — will use mark price
	if merr := checkMinNotional(ctx, symbol, qty, typ, notionalPrice, false); merr != nil {
		return mcp.NewToolResultError(merr.Error()), nil
	}

	leverage := int64(req.GetFloat("leverage", 0))
	marginType := strings.ToUpper(strings.TrimSpace(req.GetString("margin_type", "")))
	tif := strings.ToUpper(strings.TrimSpace(req.GetString("time_in_force", "GTC")))
	workingType := strings.ToUpper(strings.TrimSpace(req.GetString("working_type", "")))

	var bracketOutcome confirmResult
	if !dryRun {
		ep := make([]editParam, 0, 4)
		if typ == "LIMIT" {
			ep = append(ep, editParam{Key: "entry_price", Label: "Entry Price", Value: trimFloatStr(entryPrice), Type: "number", Step: "any"})
		}
		ep = append(ep,
			editParam{Key: "quantity", Label: "Quantity", Value: trimFloatStr(qty), Type: "number", Step: "any"},
			editParam{Key: "stop_loss_price", Label: "Stop Loss Price", Value: trimFloatStr(sl), Type: "number", Step: "any"},
			editParam{Key: "take_profit_price", Label: "Take Profit Price", Value: trimFloatStr(tp), Type: "number", Step: "any"},
		)
		brief := tradeBrief{
			Action:    fmt.Sprintf("BRACKET %s %s %.6g %s | SL %s | TP %s", typ, side, qty, symbol, trimFloatStr(sl), trimFloatStr(tp)),
			Leverage:  bracketLevString(leverage, marginType),
			Market:    []string{"Free margin: " + dash(freeUSDT(ctx)), "Positions: " + openPositionsSummary(ctx, symbol)},
			Reasoning: reasoning,
		}.Render()
		var editedParams map[string]string
		var cerr error
		bracketOutcome, editedParams, cerr = confirmOrSkipEditable(ctx, "Place bracket order", brief, ep)
		if cerr != nil {
			return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
		}
		if bracketOutcome == confirmTimedOut {
			return mcp.NewToolResultError("place_bracket_order confirmation timed out — user did not respond"), nil
		}
		if bracketOutcome == confirmHumanRejected {
			return mcp.NewToolResultError("place_bracket_order rejected by user"), nil
		}
		qty = editedFloat(editedParams, "quantity", qty)
		sl = editedFloat(editedParams, "stop_loss_price", sl)
		tp = editedFloat(editedParams, "take_profit_price", tp)
		if typ == "LIMIT" {
			entryPrice = editedFloat(editedParams, "entry_price", entryPrice)
		}
	}

	// --- Step 1: Prep leverage + margin type (skipped for dry_run) ---
	if !dryRun {
		if marginType != "" {
			p := url.Values{"symbol": []string{symbol}, "marginType": []string{marginType}}
			if _, _, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/marginType", signed: true, params: p}); err != nil {
				var be *binanceError
				if !errors.As(err, &be) || be.Code != -4046 { // -4046 = already set
					return mcp.NewToolResultError("set margin type: " + humanizeError(err)), nil
				}
			}
		}
		if leverage > 0 {
			p := url.Values{"symbol": []string{symbol}, "leverage": []string{strconv.FormatInt(leverage, 10)}}
			if _, _, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/leverage", signed: true, params: p}); err != nil {
				return mcp.NewToolResultError("set leverage: " + humanizeError(err)), nil
			}
		}
	}

	// --- Step 2 (dry-run only): validate entry leg ---
	if dryRun {
		entryParams := url.Values{
			"symbol":           {symbol},
			"side":             {side},
			"type":             {typ},
			"quantity":         {trimFloatStr(qty)},
			"newClientOrderId": {newClientOrderID()},
		}
		if typ == "LIMIT" {
			entryParams.Set("price", trimFloatStr(entryPrice))
			entryParams.Set("timeInForce", tif)
		}
		_, _, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/order/test", signed: true, params: entryParams})
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("[dry-run] entry leg FAILED: %s\n(SL/TP validated locally — Binance /order/test rejects conditional orders)", humanizeError(err))), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(
			"[dry-run] entry %s %s qty=%s: OK\nSL %s / TP %s: validated locally (algo API does not support dry-run)",
			typ, side, trimFloatStr(qty), trimFloatStr(sl), trimFloatStr(tp),
		)), nil
	}

	// --- Step 3: Place entry order ---
	entryClientID := newClientOrderID()
	entryParams := url.Values{
		"symbol":           {symbol},
		"side":             {side},
		"type":             {typ},
		"quantity":         {trimFloatStr(qty)},
		"newClientOrderId": {entryClientID},
	}
	if typ == "LIMIT" {
		entryParams.Set("price", trimFloatStr(entryPrice))
		entryParams.Set("timeInForce", tif)
	}

	entryBody, entryHdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/order", signed: true, params: entryParams})
	if err != nil {
		var be *binanceError
		if errors.As(err, &be) && be.HTTPStatus == 503 && strings.Contains(strings.ToLower(be.Msg), "unknown") {
			return reconcileOrder(ctx, symbol, entryClientID)
		}
		return mcp.NewToolResultError("entry order failed: " + humanizeError(err)), nil
	}

	// --- Step 4: Place SL + TP via Algo API directly ---
	exitSide := "SELL"
	if side == "SELL" {
		exitSide = "BUY"
	}

	slBody, slErr := placeAlgoConditional(ctx, symbol, exitSide, "STOP_MARKET", sl, workingType)
	tpBody, tpErr := placeAlgoConditional(ctx, symbol, exitSide, "TAKE_PROFIT_MARKET", tp, workingType)

	if slErr == nil && tpErr == nil {
		return mcp.NewToolResultText(bracketOutcome.prefix() + fmt.Sprintf(
			"✅ Bracket order placed successfully!\n\nEntry:\n%s\n\nSL:\n%s\n\nTP:\n%s%s",
			prettyJSON(entryBody), prettyJSON(slBody), prettyJSON(tpBody), rateLimitFooter(entryHdr),
		)), nil
	}

	// --- Step 5: SL/TP failed — report clearly with what succeeded ---
	// -4130 means an existing closePosition=true algo order already covers the full position.
	// This is not an error — the position is protected; skip alarming the user.
	slCovered := isCoveredByExistingAlgo(slErr)
	tpCovered := isCoveredByExistingAlgo(tpErr)

	if slCovered && tpCovered {
		return mcp.NewToolResultText(bracketOutcome.prefix() + fmt.Sprintf(
			"✅ Bracket order placed successfully!\n\nEntry:\n%s\n\nSL/TP: existing closePosition orders already cover the full position (no new legs needed).\nSL: %s | TP: %s%s",
			prettyJSON(entryBody), trimFloatStr(sl), trimFloatStr(tp), rateLimitFooter(entryHdr),
		)), nil
	}

	var report strings.Builder
	report.WriteString("[POSITION LIVE]\nEntry filled. ")
	if slErr != nil && tpErr != nil {
		report.WriteString("⚠️ BOTH SL and TP failed to set — set manually!\n")
	} else if slErr != nil {
		report.WriteString("⚠️ SL failed — TP is set, but add SL manually!\n")
	} else {
		report.WriteString("⚠️ TP failed — SL is set, but add TP manually.\n")
	}
	report.WriteString(fmt.Sprintf("\nEntry:\n%s\n", prettyJSON(entryBody)))
	if slErr != nil && !slCovered {
		report.WriteString(fmt.Sprintf("\nSL error: %s\n", slErr.Error()))
	} else if slCovered {
		report.WriteString(fmt.Sprintf("\nSL: existing closePosition order covers position (trigger: %s)\n", trimFloatStr(sl)))
	} else {
		report.WriteString(fmt.Sprintf("\nSL:\n%s\n", prettyJSON(slBody)))
	}
	if tpErr != nil && !tpCovered {
		report.WriteString(fmt.Sprintf("\nTP error: %s\n", tpErr.Error()))
	} else if tpCovered {
		report.WriteString(fmt.Sprintf("\nTP: existing closePosition order covers position (trigger: %s)\n", trimFloatStr(tp)))
	} else {
		report.WriteString(fmt.Sprintf("\nTP:\n%s\n", prettyJSON(tpBody)))
	}
	report.WriteString(fmt.Sprintf("\nManual values — SL: %s | TP: %s", trimFloatStr(sl), trimFloatStr(tp)))

	return mcp.NewToolResultError(report.String()), nil
}

// isCoveredByExistingAlgo returns true when the error is Binance -4130,
// meaning an existing closePosition=true algo order already covers the position.
func isCoveredByExistingAlgo(err error) bool {
	if err == nil {
		return false
	}
	var be *binanceError
	return errors.As(err, &be) && be.Code == -4130
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

// =====================================================================
// cancel_order — cancels a single regular order by orderId or clientOrderId
// =====================================================================

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
	outcome, err := confirmOrSkip(ctx, "Cancel order", tradeBrief{
		Action:    fmt.Sprintf("Cancel order %s on %s", ref, symbol),
		Reasoning: reasoning,
	}.Render())
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if outcome == confirmTimedOut {
		return mcp.NewToolResultError("cancel_order confirmation timed out — user did not respond"), nil
	}
	if outcome == confirmHumanRejected {
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
	return mcp.NewToolResultText(outcome.prefix() + prettyJSON(body) + rateLimitFooter(hdr)), nil
}

// =====================================================================
// cancel_all_open_orders — cancels ALL open orders for a symbol:
// both regular orders (/fapi/v1/allOpenOrders) AND algo/conditional
// orders (/fapi/v1/openAlgoOrders → /fapi/v1/algoOrder per ID).
// This is critical because SL/TP from bracket orders are algo orders
// and are invisible to the regular cancel endpoint.
// =====================================================================

func cancelAllOrdersHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}
	outcome, err := confirmOrSkip(ctx, "Cancel all orders", tradeBrief{
		Action:    "Cancel ALL open orders (regular + algo/conditional) on " + symbol,
		Reasoning: reasoning,
	}.Render())
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if outcome == confirmTimedOut {
		return mcp.NewToolResultError("cancel_all_open_orders confirmation timed out — user did not respond"), nil
	}
	if outcome == confirmHumanRejected {
		return mcp.NewToolResultError("cancel_all_open_orders rejected by user"), nil
	}

	var report strings.Builder

	// 1. Cancel regular orders
	p := url.Values{"symbol": []string{symbol}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodDelete, path: "/fapi/v1/allOpenOrders", signed: true, params: p})
	if err != nil {
		report.WriteString(fmt.Sprintf("regular orders: FAILED — %s\n", humanizeError(err)))
	} else {
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		report.WriteString(fmt.Sprintf("regular orders: cancelled (code=%v)\n", result["code"]))
	}

	// 2. Cancel algo/conditional orders (SL/TP from bracket orders live here)
	algoOrders, err := fetchOpenAlgoOrders(ctx, symbol)
	if err != nil {
		report.WriteString(fmt.Sprintf("algo orders: fetch failed — %s\n", humanizeError(err)))
	} else if len(algoOrders) == 0 {
		report.WriteString("algo orders: none found\n")
	} else {
		cancelled, failed := 0, 0
		for _, o := range algoOrders {
			algoID := algoIDStr(o["algoId"])
			if cerr := cancelAlgoOrder(ctx, algoID); cerr != nil {
				failed++
				report.WriteString(fmt.Sprintf("algo order %s: FAILED — %s\n", algoID, humanizeError(cerr)))
			} else {
				cancelled++
			}
		}
		report.WriteString(fmt.Sprintf("algo orders: %d cancelled, %d failed\n", cancelled, failed))
	}

	return mcp.NewToolResultText(outcome.prefix() + report.String() + rateLimitFooter(hdr)), nil
}

// =====================================================================
// cancel_algo_order — cancels a single algo/conditional order by algoId.
// Use this to remove orphaned SL/TP orders after manual position close.
// =====================================================================

func cancelAlgoOrderHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	algoID := strings.TrimSpace(req.GetString("algo_id", ""))
	if algoID == "" {
		return mcp.NewToolResultError("algo_id is required"), nil
	}
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}
	outcome, err := confirmOrSkip(ctx, "Cancel algo order", tradeBrief{
		Action:    "Cancel algo order algoId=" + algoID,
		Reasoning: reasoning,
	}.Render())
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if outcome == confirmTimedOut {
		return mcp.NewToolResultError("cancel_algo_order confirmation timed out — user did not respond"), nil
	}
	if outcome == confirmHumanRejected {
		return mcp.NewToolResultError("cancel_algo_order rejected by user"), nil
	}
	b, _, err := request(ctx, requestOpts{
		method: http.MethodDelete,
		path:   "/fapi/v1/algoOrder",
		signed: true,
		params: url.Values{"algoId": {algoID}},
	})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(outcome.prefix() + prettyJSON(b)), nil
}

// =====================================================================
// TA helpers (used by taSnapshotHandler in extras.go)
// =====================================================================

func ema(v []float64, period int) float64 {
	if len(v) < period {
		return math.NaN()
	}
	k := 2.0 / float64(period+1)
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += v[i]
	}
	e := sum / float64(period)
	for i := period; i < len(v); i++ {
		e = v[i]*k + e*(1-k)
	}
	return e
}

func rsi(v []float64, period int) float64 {
	if len(v) <= period {
		return math.NaN()
	}
	var gain, loss float64
	for i := 1; i <= period; i++ {
		d := v[i] - v[i-1]
		if d >= 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	avgG := gain / float64(period)
	avgL := loss / float64(period)
	for i := period + 1; i < len(v); i++ {
		d := v[i] - v[i-1]
		g, l := 0.0, 0.0
		if d >= 0 {
			g = d
		} else {
			l = -d
		}
		avgG = (avgG*float64(period-1) + g) / float64(period)
		avgL = (avgL*float64(period-1) + l) / float64(period)
	}
	if avgL == 0 {
		return 100
	}
	return 100 - 100/(1+avgG/avgL)
}

func atr(h, l, c []float64, period int) float64 {
	if len(c) <= period {
		return math.NaN()
	}
	trs := make([]float64, len(c))
	for i := 1; i < len(c); i++ {
		tr1 := h[i] - l[i]
		tr2 := math.Abs(h[i] - c[i-1])
		tr3 := math.Abs(l[i] - c[i-1])
		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	a := sum / float64(period)
	for i := period + 1; i < len(trs); i++ {
		a = (a*float64(period-1) + trs[i]) / float64(period)
	}
	return a
}

func bollinger(v []float64, period int, mult float64) (mid, upper, lower float64) {
	if len(v) < period {
		return math.NaN(), math.NaN(), math.NaN()
	}
	start := len(v) - period
	sum := 0.0
	for i := start; i < len(v); i++ {
		sum += v[i]
	}
	mid = sum / float64(period)
	var sq float64
	for i := start; i < len(v); i++ {
		d := v[i] - mid
		sq += d * d
	}
	sd := math.Sqrt(sq / float64(period))
	upper = mid + mult*sd
	lower = mid - mult*sd
	return
}

func toFloat(x interface{}) float64 {
	switch t := x.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func toInt64(x interface{}) int64 {
	switch t := x.(type) {
	case float64:
		return int64(t)
	case string:
		i, _ := strconv.ParseInt(t, 10, 64)
		return i
	}
	return 0
}

func roundTo(v float64, decimals int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	p := math.Pow10(decimals)
	return math.Round(v*p) / p
}
