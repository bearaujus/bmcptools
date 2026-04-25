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

// =====================================================================
// close_position
// =====================================================================

func closePositionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	dryRun := req.GetBool("dry_run", false)
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if !dryRun && reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true or dry_run=true"), nil
	}

	// Discover current position size/side.
	posSideFilter := strings.ToUpper(strings.TrimSpace(req.GetString("position_side", "")))
	pos, err := fetchSinglePosition(ctx, symbol, posSideFilter)
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	amt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
	if amt == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("no open position on %s — nothing to close", symbol)), nil
	}

	// Determine close side + quantity.
	side := "SELL" // closing a LONG
	if amt < 0 {
		side = "BUY" // closing a SHORT
	}
	absAmt := math.Abs(amt)
	closeQty := absAmt
	if q := req.GetFloat("quantity", 0); q > 0 {
		if q > absAmt {
			return mcp.NewToolResultError(fmt.Sprintf("quantity %s exceeds current position size %s", trimFloatStr(q), trimFloatStr(absAmt))), nil
		}
		closeQty = q
	}

	price := req.GetFloat("price", 0)
	typ := "MARKET"
	tif := ""
	if price > 0 {
		typ = "LIMIT"
		tif = strings.ToUpper(strings.TrimSpace(req.GetString("time_in_force", "GTC")))
	}

	p := placeOrderParams{
		Symbol:           symbol,
		Side:             side,
		Type:             typ,
		Quantity:         trimFloatStr(closeQty),
		ReduceOnly:       "true",
		PositionSide:     posSideFilter,
		NewClientOrderID: newClientOrderID(),
	}
	if price > 0 {
		p.Price = trimFloatStr(price)
		p.TimeInForce = tif
	}

	// In hedge mode, reduceOnly is implicit — don't send reduceOnly if positionSide is set.
	if posSideFilter != "" {
		p.ReduceOnly = ""
	}

	var confirmOutcome confirmResult
	if !dryRun {
		ep := []editParam{
			{Key: "quantity", Label: "Quantity", Value: trimFloatStr(closeQty), Type: "number", Step: "any"},
		}
		if price > 0 {
			ep = append(ep, editParam{Key: "price", Label: "Limit Price", Value: trimFloatStr(price), Type: "number", Step: "any"})
		}
		brief := tradeBrief{
			Action:    fmt.Sprintf("CLOSE %s position on %s (%s %s qty=%s via %s)", positionSideHuman(amt), symbol, side, symbol, trimFloatStr(closeQty), typ),
			Market:    []string{"Current position: " + fmt.Sprintf("%s %s @ %s (uPnL %s)", symbol, trimFloatStr(amt), pos.EntryPrice, pos.UnRealizedPnL)},
			Reasoning: reasoning,
		}.Render()
		var editedParams map[string]string
		var cerr error
		confirmOutcome, editedParams, cerr = confirmOrSkipEditable(ctx, "Close position", brief, ep)
		if cerr != nil {
			return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
		}
		if confirmOutcome == confirmTimedOut {
			return mcp.NewToolResultError("close_position confirmation timed out — user did not respond"), nil
		}
		if confirmOutcome == confirmHumanRejected {
			return mcp.NewToolResultError("close_position rejected by user"), nil
		}
		if newQty := editedFloat(editedParams, "quantity", closeQty); newQty > 0 && newQty <= absAmt {
			closeQty = newQty
			p.Quantity = trimFloatStr(closeQty)
		}
		if price > 0 {
			if newPrice := editedFloat(editedParams, "price", price); newPrice > 0 {
				p.Price = trimFloatStr(newPrice)
			}
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
			return reconcileOrder(ctx, symbol, p.NewClientOrderID)
		}
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	if dryRun {
		return mcp.NewToolResultText(fmt.Sprintf(
			"[dry-run] validated by /fapi/v1/order/test (no execution)\nparams: %s%s",
			p.toValues().Encode(), rateLimitFooter(hdr),
		)), nil
	}
	return mcp.NewToolResultText(confirmOutcome.prefix() + prettyJSON(body) + rateLimitFooter(hdr)), nil
}

type positionRow struct {
	Symbol        string `json:"symbol"`
	PositionAmt   string `json:"positionAmt"`
	EntryPrice    string `json:"entryPrice"`
	MarkPrice     string `json:"markPrice"`
	UnRealizedPnL string `json:"unRealizedProfit"`
	Leverage      string `json:"leverage"`
	MarginType    string `json:"marginType"`
	PositionSide  string `json:"positionSide"`
	IsolatedWallet string `json:"isolatedWallet"`
	Notional      string `json:"notional"`
	LiquidationPrice string `json:"liquidationPrice"`
}

func fetchSinglePosition(ctx context.Context, symbol, posSideFilter string) (*positionRow, error) {
	p := url.Values{"symbol": []string{symbol}}
	body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/positionRisk", signed: true, params: p})
	if err != nil {
		return nil, err
	}
	var rows []positionRow
	if err := unmarshalIgnore(body, &rows); err != nil {
		return nil, err
	}
	// Prefer non-zero position. In one-way mode, positionSide="BOTH".
	var fallback *positionRow
	for i := range rows {
		r := &rows[i]
		if posSideFilter != "" && !strings.EqualFold(r.PositionSide, posSideFilter) {
			continue
		}
		amt, _ := strconv.ParseFloat(r.PositionAmt, 64)
		if amt != 0 {
			return r, nil
		}
		if fallback == nil {
			fallback = r
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("no position row found for %s", symbol)
}

func positionSideHuman(amt float64) string {
	if amt > 0 {
		return "LONG"
	}
	if amt < 0 {
		return "SHORT"
	}
	return "FLAT"
}

func positionModeStr(dual bool) string {
	if dual {
		return "hedge"
	}
	return "one-way"
}

// =====================================================================
// modify_order
// =====================================================================

func modifyOrderHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	side := strings.ToUpper(strings.TrimSpace(req.GetString("side", "")))
	if symbol == "" || (side != "BUY" && side != "SELL") {
		return mcp.NewToolResultError("symbol and side=BUY|SELL are required"), nil
	}
	orderID := strings.TrimSpace(req.GetString("order_id", ""))
	clientID := strings.TrimSpace(req.GetString("orig_client_order_id", ""))
	if orderID == "" && clientID == "" {
		return mcp.NewToolResultError("either order_id or orig_client_order_id is required"), nil
	}
	qty := req.GetFloat("quantity", 0)
	price := req.GetFloat("price", 0)
	if qty <= 0 && price <= 0 {
		return mcp.NewToolResultError("at least one of quantity or price must be provided"), nil
	}

	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}

	ref := orderID
	if ref == "" {
		ref = clientID
	}
	change := []string{}
	if qty > 0 {
		change = append(change, "qty="+trimFloatStr(qty))
	}
	if price > 0 {
		change = append(change, "price="+trimFloatStr(price))
	}
	ep := make([]editParam, 0, 2)
	if qty > 0 {
		ep = append(ep, editParam{Key: "quantity", Label: "New Quantity", Value: trimFloatStr(qty), Type: "number", Step: "any"})
	}
	if price > 0 {
		ep = append(ep, editParam{Key: "price", Label: "New Price", Value: trimFloatStr(price), Type: "number", Step: "any"})
	}
	brief := tradeBrief{
		Action:    fmt.Sprintf("Modify order %s on %s (%s)", ref, symbol, strings.Join(change, " ")),
		Reasoning: reasoning,
	}.Render()
	outcome, editedParams, cerr := confirmOrSkipEditable(ctx, "Modify order", brief, ep)
	if cerr != nil {
		return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
	}
	if outcome == confirmTimedOut {
		return mcp.NewToolResultError("modify_order confirmation timed out — user did not respond"), nil
	}
	if outcome == confirmHumanRejected {
		return mcp.NewToolResultError("modify_order rejected by user"), nil
	}
	qty = editedFloat(editedParams, "quantity", qty)
	price = editedFloat(editedParams, "price", price)

	p := url.Values{
		"symbol": []string{symbol},
		"side":   []string{side},
	}
	if orderID != "" {
		p.Set("orderId", orderID)
	} else {
		p.Set("origClientOrderId", clientID)
	}
	if qty > 0 {
		p.Set("quantity", trimFloatStr(qty))
	}
	if price > 0 {
		p.Set("price", trimFloatStr(price))
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodPut, path: "/fapi/v1/order", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(outcome.prefix() + prettyJSON(body) + rateLimitFooter(hdr)), nil
}

// =====================================================================
// funding_rate_history
// =====================================================================

func fundingRateHistoryHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	p := url.Values{"symbol": []string{symbol}}
	if limit := req.GetFloat("limit", 0); limit > 0 {
		p.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	if st := req.GetFloat("start_time_ms", 0); st > 0 {
		p.Set("startTime", strconv.FormatInt(int64(st), 10))
	}
	if et := req.GetFloat("end_time_ms", 0); et > 0 {
		p.Set("endTime", strconv.FormatInt(int64(et), 10))
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/fundingRate", params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	// Enrich each entry with human-readable timestamp and rate direction.
	var entries []map[string]interface{}
	if jsonErr := jsonDecoderUseNumber(body).Decode(&entries); jsonErr != nil {
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}
	for _, e := range entries {
		if ts, ok := e["fundingTime"]; ok {
			if tsNum, ok2 := ts.(json.Number); ok2 {
				if msInt, err2 := tsNum.Int64(); err2 == nil {
					e["funding_time_utc"] = time.UnixMilli(msInt).UTC().Format(time.RFC3339)
				}
			}
		}
		if rate, ok := e["fundingRate"]; ok {
			var rateF float64
			var parsed bool
			switch v := rate.(type) {
			case json.Number:
				if f, err2 := v.Float64(); err2 == nil {
					rateF, parsed = f, true
				}
			case string:
				if f, err2 := strconv.ParseFloat(v, 64); err2 == nil {
					rateF, parsed = f, true
				}
			}
			if parsed {
				switch {
				case rateF > 0:
					e["direction"] = "longs_pay_shorts"
				case rateF < 0:
					e["direction"] = "shorts_pay_longs"
				default:
					e["direction"] = "neutral"
				}
			}
		}
	}
	out, _ := json.MarshalIndent(entries, "", "  ")
	return mcp.NewToolResultText(string(out) + rateLimitFooter(hdr)), nil
}

// =====================================================================
// position_overview
// =====================================================================

func positionOverviewHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))

	out := map[string]interface{}{}

	// Account: walletBalance, availableBalance, totalUnrealizedProfit, totalMarginBalance, positionMode.
	acctBody, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/account", signed: true})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	var acct struct {
		FeeTier                     int    `json:"feeTier"`
		TotalWalletBalance          string `json:"totalWalletBalance"`
		TotalUnrealizedProfit       string `json:"totalUnrealizedProfit"`
		TotalMarginBalance          string `json:"totalMarginBalance"`
		TotalPositionInitialMargin  string `json:"totalPositionInitialMargin"`
		TotalOpenOrderInitialMargin string `json:"totalOpenOrderInitialMargin"`
		AvailableBalance            string `json:"availableBalance"`
		MaxWithdrawAmount           string `json:"maxWithdrawAmount"`
		DualSidePosition            bool   `json:"dualSidePosition"`
	}
	if err := unmarshalIgnore(acctBody, &acct); err == nil {
		out["account"] = map[string]interface{}{
			"fee_tier":                        acct.FeeTier,
			"total_wallet_balance":            acct.TotalWalletBalance,
			"total_margin_balance":            acct.TotalMarginBalance,
			"total_unrealized_profit":         acct.TotalUnrealizedProfit,
			"available_balance":               acct.AvailableBalance,
			"total_position_initial_margin":   acct.TotalPositionInitialMargin,
			"total_open_order_initial_margin": acct.TotalOpenOrderInitialMargin,
			"max_withdraw_amount":             acct.MaxWithdrawAmount,
			"position_mode":                   positionModeStr(acct.DualSidePosition),
		}
	}

	// Positions (filtered if symbol provided, else all non-zero).
	posParams := url.Values{}
	if symbol != "" {
		posParams.Set("symbol", symbol)
	}
	posBody, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/positionRisk", signed: true, params: posParams})
	if err == nil {
		var rows []positionRow
		if uerr := unmarshalIgnore(posBody, &rows); uerr == nil {
			var open []map[string]interface{}
			for _, r := range rows {
				amt, _ := strconv.ParseFloat(r.PositionAmt, 64)
				if amt == 0 {
					continue
				}
				open = append(open, map[string]interface{}{
					"symbol":            r.Symbol,
					"position_amt":      r.PositionAmt,
					"side":              positionSideHuman(amt),
					"entry_price":       r.EntryPrice,
					"mark_price":        r.MarkPrice,
					"unrealized_pnl":    r.UnRealizedPnL,
					"leverage":          r.Leverage,
					"margin_type":       r.MarginType,
					"position_side":     r.PositionSide,
					"liquidation_price": r.LiquidationPrice,
					"notional":          r.Notional,
				})
			}
			out["open_positions"] = open
		}
	}

	// Commission rate for the specific symbol.
	if symbol != "" {
		cp := url.Values{"symbol": []string{symbol}}
		crBody, _, cerr := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/commissionRate", signed: true, params: cp})
		if cerr == nil {
			var cr struct {
				Symbol              string `json:"symbol"`
				MakerCommissionRate string `json:"makerCommissionRate"`
				TakerCommissionRate string `json:"takerCommissionRate"`
			}
			if err := unmarshalIgnore(crBody, &cr); err == nil {
				out["commission_rate"] = map[string]interface{}{
					"symbol":                cr.Symbol,
					"maker_commission_rate": cr.MakerCommissionRate,
					"taker_commission_rate": cr.TakerCommissionRate,
				}
			}
		}
		// Also include mark price + funding for context.
		mpBody, _, mperr := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/premiumIndex", params: url.Values{"symbol": []string{symbol}}})
		if mperr == nil {
			var mp map[string]interface{}
			if err := json.Unmarshal(mpBody, &mp); err == nil {
				out["mark"] = mp
			}
		}
	}

	b, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// =====================================================================
// ta_snapshot
// =====================================================================

// computeTASnapshot is the core TA computation shared by taSnapshotHandler and taSnapshotMultiHandler.
func computeTASnapshot(ctx context.Context, symbol, interval string, limit int, includeCandles bool) (map[string]interface{}, error) {
	p := url.Values{
		"symbol":   []string{symbol},
		"interval": []string{interval},
		"limit":    []string{strconv.Itoa(limit)},
	}
	body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/klines", params: p})
	if err != nil {
		return nil, fmt.Errorf("%s", humanizeError(err))
	}
	var raw [][]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse klines: %s", err.Error())
	}
	if len(raw) < 21 {
		return nil, fmt.Errorf("not enough candles for indicators (got %d, need >= 21)", len(raw))
	}

	n := len(raw)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	opens := make([]float64, n)
	vols := make([]float64, n)
	openTimes := make([]int64, n)
	for i, c := range raw {
		if len(c) < 6 {
			return nil, fmt.Errorf("malformed kline row")
		}
		openTimes[i] = toInt64(c[0])
		opens[i] = toFloat(c[1])
		highs[i] = toFloat(c[2])
		lows[i] = toFloat(c[3])
		closes[i] = toFloat(c[4])
		vols[i] = toFloat(c[5])
	}

	lastClose := closes[n-1]
	lastOpen := opens[n-1]
	lastTime := time.UnixMilli(openTimes[n-1]).UTC().Format(time.RFC3339)

	atr14 := atr(highs, lows, closes, 14)
	ema9 := ema(closes, 9)
	ema21 := ema(closes, 21)
	var ema50 *float64
	if n >= 50 {
		v := ema(closes, 50)
		ema50 = &v
	}
	rsi14 := rsi(closes, 14)
	bbMid, bbUp, bbLow := bollinger(closes, 20, 2.0)

	trend := "neutral"
	if ema9 > ema21 && (ema50 == nil || ema21 > *ema50) {
		trend = "uptrend"
	} else if ema9 < ema21 && (ema50 == nil || ema21 < *ema50) {
		trend = "downtrend"
	}

	rsiHint := "neutral"
	switch {
	case rsi14 >= 70:
		rsiHint = "overbought"
	case rsi14 <= 30:
		rsiHint = "oversold"
	}

	bbPos := "inside"
	if lastClose > bbUp {
		bbPos = "above_upper"
	} else if lastClose < bbLow {
		bbPos = "below_lower"
	}

	snap := map[string]interface{}{
		"symbol":   symbol,
		"interval": interval,
		"candles":  n,
		"last": map[string]interface{}{
			"open_time_utc": lastTime,
			"open":          lastOpen,
			"close":         lastClose,
			"high":          highs[n-1],
			"low":           lows[n-1],
			"volume":        vols[n-1],
		},
		"indicators": map[string]interface{}{
			"atr_14":       roundTo(atr14, 8),
			"atr_14_pct":   roundTo(atr14/lastClose*100, 4),
			"ema_9":        roundTo(ema9, 8),
			"ema_21":       roundTo(ema21, 8),
			"ema_50":       ema50Value(ema50),
			"rsi_14":       roundTo(rsi14, 2),
			"bb_20_mid":    roundTo(bbMid, 8),
			"bb_20_upper":  roundTo(bbUp, 8),
			"bb_20_lower":  roundTo(bbLow, 8),
			"bb_width_pct": roundTo((bbUp-bbLow)/bbMid*100, 4),
		},
		"hints": map[string]interface{}{
			"trend":          trend,
			"rsi":            rsiHint,
			"bollinger":      bbPos,
			"sl_hint_2x_atr": roundTo(atr14*2, 8),
			"sl_hint_3x_atr": roundTo(atr14*3, 8),
		},
	}
	if includeCandles {
		snap["klines"] = raw
	}
	return snap, nil
}

func taSnapshotHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	interval := strings.TrimSpace(req.GetString("interval", ""))
	if symbol == "" || interval == "" {
		return mcp.NewToolResultError("symbol and interval are required"), nil
	}
	limit := int(req.GetFloat("limit", 100))
	if limit < 20 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	includeCandles := req.GetBool("include_candles", false)

	snap, err := computeTASnapshot(ctx, symbol, interval, limit, includeCandles)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(snap, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func ema50Value(p *float64) interface{} {
	if p == nil {
		return nil
	}
	return roundTo(*p, 8)
}

// =====================================================================
// ta_snapshot_multi
// =====================================================================

func taSnapshotMultiHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbolsRaw := strings.TrimSpace(req.GetString("symbols", ""))
	if symbolsRaw == "" {
		return mcp.NewToolResultError("symbols is required (comma-separated list, e.g. BTCUSDT,ETHUSDT)"), nil
	}
	interval := strings.TrimSpace(req.GetString("interval", ""))
	if interval == "" {
		return mcp.NewToolResultError("interval is required"), nil
	}
	limit := int(req.GetFloat("limit", 100))
	if limit < 20 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	includeCandles := req.GetBool("include_candles", false)

	parts := strings.Split(symbolsRaw, ",")
	symbols := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != "" {
			symbols = append(symbols, s)
		}
	}
	if len(symbols) == 0 {
		return mcp.NewToolResultError("no valid symbols provided"), nil
	}

	type snapResult struct {
		symbol string
		snap   map[string]interface{}
		err    error
	}

	ch := make(chan snapResult, len(symbols))
	for _, sym := range symbols {
		go func(sym string) {
			snap, err := computeTASnapshot(ctx, sym, interval, limit, includeCandles)
			ch <- snapResult{symbol: sym, snap: snap, err: err}
		}(sym)
	}

	snapshots := make([]map[string]interface{}, 0, len(symbols))
	errsMap := make(map[string]string)
	for range symbols {
		r := <-ch
		if r.err != nil {
			errsMap[r.symbol] = r.err.Error()
		} else {
			snapshots = append(snapshots, r.snap)
		}
	}

	out := map[string]interface{}{
		"interval":  interval,
		"snapshots": snapshots,
		"errors":    errsMap,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// =====================================================================
// position_health
// =====================================================================

func positionHealthHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))

	type posResult struct {
		rows []positionRow
		err  error
	}
	type algoResult struct {
		orders []map[string]interface{}
		err    error
	}

	posCh := make(chan posResult, 1)
	algoCh := make(chan algoResult, 1)

	go func() {
		p := url.Values{}
		if symbol != "" {
			p.Set("symbol", symbol)
		}
		body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/positionRisk", signed: true, params: p})
		if err != nil {
			posCh <- posResult{err: err}
			return
		}
		var rows []positionRow
		_ = unmarshalIgnore(body, &rows)
		posCh <- posResult{rows: rows}
	}()

	go func() {
		orders, err := fetchOpenAlgoOrders(ctx, symbol)
		algoCh <- algoResult{orders: orders, err: err}
	}()

	posRes := <-posCh
	algoRes := <-algoCh

	if posRes.err != nil {
		return mcp.NewToolResultError(humanizeError(posRes.err)), nil
	}

	// Build lookup: positionSide → list of algo orders (keyed by symbol+positionSide).
	type algoKey struct{ symbol, positionSide string }
	algoMap := make(map[algoKey][]map[string]interface{})
	if algoRes.err == nil {
		for _, o := range algoRes.orders {
			sym, _ := o["symbol"].(string)
			pSide, _ := o["positionSide"].(string)
			k := algoKey{sym, pSide}
			algoMap[k] = append(algoMap[k], o)
		}
	}

	algoOrderSummary := func(o map[string]interface{}) map[string]interface{} {
		algoId := o["algoId"]
		trigger, _ := o["triggerPrice"].(string)
		workingType, _ := o["workingType"].(string)
		return map[string]interface{}{
			"algo_id":       algoId,
			"trigger_price": trigger,
			"working_type":  workingType,
		}
	}

	isStopType := func(ot string) bool {
		ot = strings.ToUpper(ot)
		return ot == "STOP_MARKET" || ot == "STOP"
	}
	isTakeProfitType := func(ot string) bool {
		ot = strings.ToUpper(ot)
		return ot == "TAKE_PROFIT_MARKET" || ot == "TAKE_PROFIT"
	}

	var positions []map[string]interface{}
	totalUPnL := 0.0

	for _, pos := range posRes.rows {
		amt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if amt == 0 {
			continue
		}
		markPrice, _ := strconv.ParseFloat(pos.MarkPrice, 64)
		entryPrice, _ := strconv.ParseFloat(pos.EntryPrice, 64)
		uPnL, _ := strconv.ParseFloat(pos.UnRealizedPnL, 64)
		lev, _ := strconv.ParseFloat(pos.Leverage, 64)
		totalUPnL += uPnL
		isLong := amt > 0

		pnlPct := 0.0
		if entryPrice > 0 {
			pnlPct = uPnL / (math.Abs(amt) * entryPrice) * 100
		}
		pnlPctOnMargin := 0.0
		if lev > 0 {
			pnlPctOnMargin = pnlPct * lev
		}

		// Match by symbol + positionSide (works in both one-way and hedge mode).
		matchedOrders := algoMap[algoKey{pos.Symbol, pos.PositionSide}]

		var slOrders, tpOrders []map[string]interface{}
		for _, o := range matchedOrders {
			orderType, _ := o["orderType"].(string)
			switch {
			case isStopType(orderType):
				slOrders = append(slOrders, o)
			case isTakeProfitType(orderType):
				tpOrders = append(tpOrders, o)
			}
		}

		slSummaries := make([]map[string]interface{}, 0, len(slOrders))
		for _, o := range slOrders {
			slSummaries = append(slSummaries, algoOrderSummary(o))
		}
		tpSummaries := make([]map[string]interface{}, 0, len(tpOrders))
		for _, o := range tpOrders {
			tpSummaries = append(tpSummaries, algoOrderSummary(o))
		}

		posEntry := map[string]interface{}{
			"symbol":            pos.Symbol,
			"side":              positionSideHuman(amt),
			"position_side":     pos.PositionSide,
			"position_amt":      pos.PositionAmt,
			"entry_price":       pos.EntryPrice,
			"mark_price":        pos.MarkPrice,
			"unrealized_pnl":    pos.UnRealizedPnL,
			"pnl_pct":           roundTo(pnlPct, 4),
			"pnl_pct_on_margin": roundTo(pnlPctOnMargin, 4),
			"leverage":          pos.Leverage,
			"margin_type":       pos.MarginType,
			"liquidation_price": pos.LiquidationPrice,
			"notional":          pos.Notional,
			"sl_orders":         slSummaries,
			"tp_orders":         tpSummaries,
			"has_duplicates":    len(slOrders) > 1 || len(tpOrders) > 1,
		}

		// Compute SL/TP distances and structural validity when primary orders exist.
		if len(slOrders) > 0 && markPrice > 0 {
			slTrigger, err := strconv.ParseFloat(slOrders[0]["triggerPrice"].(string), 64)
			if err == nil {
				d := math.Abs(markPrice-slTrigger) / markPrice * 100
				posEntry["sl_distance_from_mark_pct"] = roundTo(d, 4)

				if len(tpOrders) > 0 {
					tpTrigger, err2 := strconv.ParseFloat(tpOrders[0]["triggerPrice"].(string), 64)
					if err2 == nil {
						d2 := math.Abs(tpTrigger-markPrice) / markPrice * 100
						posEntry["tp_distance_from_mark_pct"] = roundTo(d2, 4)

						// Structural validity: SL must be on loss side, TP on profit side.
						valid := false
						if isLong {
							valid = slTrigger < entryPrice && tpTrigger > entryPrice
						} else {
							valid = slTrigger > entryPrice && tpTrigger < entryPrice
						}
						posEntry["sl_tp_structurally_valid"] = valid

						if valid && d > 0 {
							positionNotional := math.Abs(amt) * markPrice
							posEntry["remaining_rr_ratio"] = roundTo(d2/d, 4)
							posEntry["remaining_profit_usdt"] = roundTo(positionNotional*d2/100, 2)
							posEntry["remaining_risk_usdt"] = roundTo(positionNotional*d/100, 2)
						}
					}
				}
			}
		}

		positions = append(positions, posEntry)
	}

	if positions == nil {
		positions = []map[string]interface{}{}
	}

	result := map[string]interface{}{
		"positions":            positions,
		"total_unrealized_pnl": roundTo(totalUPnL, 6),
	}
	if algoRes.err != nil {
		result["algo_orders_error"] = humanizeError(algoRes.err)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// =====================================================================
// calc_order_size
// =====================================================================

func calcOrderSizeHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	desiredNotional := req.GetFloat("desired_notional_usdt", 0)
	if desiredNotional <= 0 {
		return mcp.NewToolResultError("desired_notional_usdt must be > 0"), nil
	}
	providedPrice := req.GetFloat("price", 0)
	orderType := strings.ToUpper(strings.TrimSpace(req.GetString("order_type", "MARKET")))
	if orderType != "MARKET" && orderType != "LIMIT" {
		orderType = "MARKET"
	}

	type infoResult struct {
		info *symbolInfo
		err  error
	}
	type priceResult struct {
		price float64
		err   error
	}
	type balResult struct {
		v  float64
		ok bool
	}

	infoCh := make(chan infoResult, 1)
	priceCh := make(chan priceResult, 1)
	balCh := make(chan balResult, 1)

	go func() {
		info, err := fetchSymbolInfo(ctx, symbol)
		infoCh <- infoResult{info, err}
	}()
	go func() {
		if providedPrice > 0 {
			priceCh <- priceResult{price: providedPrice}
			return
		}
		p, err := fetchMarkPriceF(ctx, symbol)
		priceCh <- priceResult{price: p, err: err}
	}()
	go func() {
		v, ok := freeUSDTFloat(ctx)
		balCh <- balResult{v, ok}
	}()

	infoRes := <-infoCh
	priceRes := <-priceCh
	balRes := <-balCh

	if infoRes.err != nil {
		return mcp.NewToolResultError("symbol info: " + humanizeError(infoRes.err)), nil
	}
	if priceRes.err != nil {
		return mcp.NewToolResultError("mark price: " + humanizeError(priceRes.err)), nil
	}

	info := infoRes.info
	priceUsed := priceRes.price
	if priceUsed <= 0 {
		return mcp.NewToolResultError("price is zero or negative — provide a price manually"), nil
	}

	filters := reduceSymbolFilters(info)

	// Prefer market lot filters for MARKET orders, fall back to LOT_SIZE.
	stepSizeStr := ""
	minQtyStr := ""
	if orderType == "MARKET" {
		stepSizeStr, _ = filters["marketStepSize"].(string)
		minQtyStr, _ = filters["marketMinQty"].(string)
	}
	if stepSizeStr == "" {
		stepSizeStr, _ = filters["stepSize"].(string)
	}
	if minQtyStr == "" {
		minQtyStr, _ = filters["minQty"].(string)
	}
	maxQtyStr, _ := filters["maxQty"].(string)
	minNotionalStr, _ := filters["minNotional"].(string)

	stepSize, err := strconv.ParseFloat(stepSizeStr, 64)
	if err != nil || stepSize <= 0 {
		stepSize = 1
	}
	minQty, _ := strconv.ParseFloat(minQtyStr, 64)
	maxQty, _ := strconv.ParseFloat(maxQtyStr, 64)
	minNotional, _ := strconv.ParseFloat(minNotionalStr, 64)

	qtyRaw := desiredNotional / priceUsed
	qtyRounded := math.Floor(qtyRaw/stepSize) * stepSize
	qtyFormatted := strconv.FormatFloat(qtyRounded, 'f', info.QuantityPrecision, 64)
	actualNotional := qtyRounded * priceUsed

	passesMinNotional := minNotional <= 0 || actualNotional >= minNotional
	passesMinQty := minQty <= 0 || qtyRounded >= minQty
	passesMaxQty := maxQty <= 0 || qtyRounded <= maxQty

	priceSource := "mark_price"
	if providedPrice > 0 {
		priceSource = "provided"
	}

	result := map[string]interface{}{
		"symbol":                symbol,
		"order_type":            orderType,
		"quantity":              qtyFormatted,
		"price_used":            priceUsed,
		"price_source":          priceSource,
		"desired_notional_usdt": desiredNotional,
		"actual_notional_usdt":  roundTo(actualNotional, 6),
		"min_notional_usdt":     minNotional,
		"passes_min_notional":   passesMinNotional,
		"min_qty":               minQtyStr,
		"max_qty":               maxQtyStr,
		"passes_min_qty":        passesMinQty,
		"passes_max_qty":        passesMaxQty,
		"step_size":             stepSizeStr,
		"quantity_precision":    info.QuantityPrecision,
	}
	if balRes.ok {
		result["free_margin_usdt"] = roundTo(balRes.v, 4)
		// Required margin = actual_notional / leverage. Without knowing leverage here, we
		// show the 1x worst-case so the AI can compute: required_margin = actual_notional / leverage.
		result["balance_note"] = fmt.Sprintf("required margin = %.4f / leverage. Free margin available: %.4f USDT", actualNotional, balRes.v)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// =====================================================================
// daily_summary
// =====================================================================

func dailySummaryHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dateStr := strings.TrimSpace(req.GetString("date", ""))
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))

	var startDay time.Time
	if dateStr != "" {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return mcp.NewToolResultError("invalid date format, use YYYY-MM-DD"), nil
		}
		startDay = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		now := time.Now().UTC()
		startDay = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		dateStr = now.Format("2006-01-02")
	}
	endDay := startDay.Add(24*time.Hour - time.Millisecond)

	p := url.Values{
		"startTime": []string{strconv.FormatInt(startDay.UnixMilli(), 10)},
		"endTime":   []string{strconv.FormatInt(endDay.UnixMilli(), 10)},
		"limit":     []string{"1000"},
	}
	if symbol != "" {
		p.Set("symbol", symbol)
	}

	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/income", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}

	var items []struct {
		Symbol     string `json:"symbol"`
		IncomeType string `json:"incomeType"`
		Income     string `json:"income"`
		Asset      string `json:"asset"`
		Info       string `json:"info"`
		Time       int64  `json:"time"`
		TradeId    string `json:"tradeId"`
	}
	if err := unmarshalIgnore(body, &items); err != nil {
		return mcp.NewToolResultError("parse income: " + err.Error()), nil
	}

	sums := map[string]float64{}
	counts := map[string]int{}
	for _, item := range items {
		v, _ := strconv.ParseFloat(item.Income, 64)
		sums[item.IncomeType] += v
		counts[item.IncomeType]++
	}

	realizedPnL := sums["REALIZED_PNL"]
	commission := sums["COMMISSION"]
	fundingFee := sums["FUNDING_FEE"]
	otherIncome := 0.0
	for k, v := range sums {
		if k != "REALIZED_PNL" && k != "COMMISSION" && k != "FUNDING_FEE" {
			otherIncome += v
		}
	}

	result := map[string]interface{}{
		"date":                     dateStr,
		"realized_pnl":             roundTo(realizedPnL, 6),
		"commission":               roundTo(commission, 6),
		"funding_fee":              roundTo(fundingFee, 6),
		"other_income":             roundTo(otherIncome, 6),
		"trading_net_profit":       roundTo(realizedPnL+commission+fundingFee, 6),
		"account_net_profit":       roundTo(realizedPnL+commission+fundingFee+otherIncome, 6),
		"realized_pnl_event_count": counts["REALIZED_PNL"],
		"total_income_events":      len(items),
	}
	if len(items) >= 1000 {
		result["warning"] = "result may be incomplete — 1000 income events returned (API limit). Use income_history with custom time ranges to paginate manually."
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out) + rateLimitFooter(hdr)), nil
}

// =====================================================================
// update_sl_tp
// =====================================================================

func updateSLTPHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}

	newSL := req.GetFloat("stop_loss_price", 0)
	newTP := req.GetFloat("take_profit_price", 0)
	if newSL <= 0 && newTP <= 0 {
		return mcp.NewToolResultError("at least one of stop_loss_price or take_profit_price is required"), nil
	}

	workingType := strings.ToUpper(strings.TrimSpace(req.GetString("working_type", "")))
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}

	type algoRes struct {
		orders []map[string]interface{}
		err    error
	}
	type posRes struct {
		pos *positionRow
		err error
	}

	algoCh := make(chan algoRes, 1)
	posCh := make(chan posRes, 1)

	go func() {
		orders, err := fetchOpenAlgoOrders(ctx, symbol)
		algoCh <- algoRes{orders: orders, err: err}
	}()
	go func() {
		pos, err := fetchSinglePosition(ctx, symbol, "")
		posCh <- posRes{pos: pos, err: err}
	}()

	aRes := <-algoCh
	pRes := <-posCh

	if aRes.err != nil {
		return mcp.NewToolResultError("fetch algo orders: " + humanizeError(aRes.err)), nil
	}
	if pRes.err != nil {
		return mcp.NewToolResultError("fetch position: " + humanizeError(pRes.err)), nil
	}

	pos := pRes.pos
	amt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
	if amt == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("no open position on %s", symbol)), nil
	}
	isLong := amt > 0
	entryPrice, _ := strconv.ParseFloat(pos.EntryPrice, 64)

	if newSL > 0 && entryPrice > 0 {
		if isLong && newSL >= entryPrice {
			return mcp.NewToolResultError(fmt.Sprintf("invalid stop_loss_price: for LONG position, SL must be below entry price (%s)", pos.EntryPrice)), nil
		}
		if !isLong && newSL <= entryPrice {
			return mcp.NewToolResultError(fmt.Sprintf("invalid stop_loss_price: for SHORT position, SL must be above entry price (%s)", pos.EntryPrice)), nil
		}
	}
	if newTP > 0 && entryPrice > 0 {
		if isLong && newTP <= entryPrice {
			return mcp.NewToolResultError(fmt.Sprintf("invalid take_profit_price: for LONG position, TP must be above entry price (%s)", pos.EntryPrice)), nil
		}
		if !isLong && newTP >= entryPrice {
			return mcp.NewToolResultError(fmt.Sprintf("invalid take_profit_price: for SHORT position, TP must be below entry price (%s)", pos.EntryPrice)), nil
		}
	}

	isStopType := func(ot string) bool {
		ot = strings.ToUpper(ot)
		return ot == "STOP_MARKET" || ot == "STOP"
	}
	isTakeProfitType := func(ot string) bool {
		ot = strings.ToUpper(ot)
		return ot == "TAKE_PROFIT_MARKET" || ot == "TAKE_PROFIT"
	}

	var existingSL, existingTP map[string]interface{}
	for _, o := range aRes.orders {
		sym, _ := o["symbol"].(string)
		if sym != symbol {
			continue
		}
		orderType, _ := o["orderType"].(string)
		if isStopType(orderType) && existingSL == nil {
			existingSL = o
		}
		if isTakeProfitType(orderType) && existingTP == nil {
			existingTP = o
		}
	}

	currentSLStr := "none"
	if existingSL != nil {
		if tp, ok := existingSL["triggerPrice"].(string); ok {
			currentSLStr = tp
		}
	}
	currentTPStr := "none"
	if existingTP != nil {
		if tp, ok := existingTP["triggerPrice"].(string); ok {
			currentTPStr = tp
		}
	}
	newSLStr := "unchanged"
	if newSL > 0 {
		newSLStr = trimFloatStr(newSL)
	}
	newTPStr := "unchanged"
	if newTP > 0 {
		newTPStr = trimFloatStr(newTP)
	}

	brief := tradeBrief{
		Action: fmt.Sprintf("Update SL/TP for %s %s position", positionSideHuman(amt), symbol),
		Market: []string{
			fmt.Sprintf("Entry: %s | Mark: %s | Amt: %s", pos.EntryPrice, pos.MarkPrice, pos.PositionAmt),
			fmt.Sprintf("SL: %s → %s", currentSLStr, newSLStr),
			fmt.Sprintf("TP: %s → %s", currentTPStr, newTPStr),
		},
		Reasoning: reasoning,
	}.Render()

	ep := make([]editParam, 0, 2)
	if newSL > 0 {
		ep = append(ep, editParam{Key: "stop_loss_price", Label: "Stop Loss Price", Value: trimFloatStr(newSL), Type: "number", Step: "any"})
	}
	if newTP > 0 {
		ep = append(ep, editParam{Key: "take_profit_price", Label: "Take Profit Price", Value: trimFloatStr(newTP), Type: "number", Step: "any"})
	}
	outcome, editedParams, cerr := confirmOrSkipEditable(ctx, "Update SL/TP", brief, ep)
	if cerr != nil {
		return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
	}
	if outcome == confirmTimedOut {
		return mcp.NewToolResultError("update_sl_tp confirmation timed out — user did not respond"), nil
	}
	if outcome == confirmHumanRejected {
		return mcp.NewToolResultError("update_sl_tp rejected by user"), nil
	}
	newSL = editedFloat(editedParams, "stop_loss_price", newSL)
	newTP = editedFloat(editedParams, "take_profit_price", newTP)

	exitSide := "SELL"
	if !isLong {
		exitSide = "BUY"
	}

	var report strings.Builder

	if newSL > 0 {
		if existingSL != nil {
			algoID := algoIDStr(existingSL["algoId"])
			if err := cancelAlgoOrder(ctx, algoID); err != nil {
				report.WriteString(fmt.Sprintf("cancel existing SL (algoId=%s): FAILED — %s\n", algoID, humanizeError(err)))
			} else {
				report.WriteString(fmt.Sprintf("cancel existing SL (algoId=%s): OK\n", algoID))
			}
		}
		slBody, slErr := placeAlgoConditional(ctx, symbol, exitSide, "STOP_MARKET", newSL, workingType)
		if slErr != nil {
			report.WriteString(fmt.Sprintf("place new SL @ %s: FAILED — %s\n", trimFloatStr(newSL), humanizeError(slErr)))
		} else {
			report.WriteString(fmt.Sprintf("place new SL @ %s: OK\n%s\n", trimFloatStr(newSL), prettyJSON(slBody)))
		}
	}

	if newTP > 0 {
		if existingTP != nil {
			algoID := algoIDStr(existingTP["algoId"])
			if err := cancelAlgoOrder(ctx, algoID); err != nil {
				report.WriteString(fmt.Sprintf("cancel existing TP (algoId=%s): FAILED — %s\n", algoID, humanizeError(err)))
			} else {
				report.WriteString(fmt.Sprintf("cancel existing TP (algoId=%s): OK\n", algoID))
			}
		}
		tpBody, tpErr := placeAlgoConditional(ctx, symbol, exitSide, "TAKE_PROFIT_MARKET", newTP, workingType)
		if tpErr != nil {
			report.WriteString(fmt.Sprintf("place new TP @ %s: FAILED — %s\n", trimFloatStr(newTP), humanizeError(tpErr)))
		} else {
			report.WriteString(fmt.Sprintf("place new TP @ %s: OK\n%s\n", trimFloatStr(newTP), prettyJSON(tpBody)))
		}
	}

	return mcp.NewToolResultText(outcome.prefix() + report.String()), nil
}

// =====================================================================
// position_brief
// =====================================================================

func positionBriefHandler(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	now := time.Now().UTC()
	startDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	endDay := startDay.Add(24*time.Hour - time.Millisecond)
	dateStr := now.Format("2006-01-02")

	type posResult struct {
		rows []positionRow
		err  error
	}
	type algoResult struct {
		orders []map[string]interface{}
		err    error
	}
	type incomeItem struct {
		IncomeType string `json:"incomeType"`
		Income     string `json:"income"`
	}
	type incomeResult struct {
		items []incomeItem
		err   error
	}
	type marginResult struct {
		v  float64
		ok bool
	}

	posCh := make(chan posResult, 1)
	algoCh := make(chan algoResult, 1)
	incomeCh := make(chan incomeResult, 1)
	marginCh := make(chan marginResult, 1)

	go func() {
		body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/positionRisk", signed: true})
		if err != nil {
			posCh <- posResult{err: err}
			return
		}
		var rows []positionRow
		_ = unmarshalIgnore(body, &rows)
		posCh <- posResult{rows: rows}
	}()

	go func() {
		orders, err := fetchOpenAlgoOrders(ctx, "")
		algoCh <- algoResult{orders: orders, err: err}
	}()

	go func() {
		p := url.Values{
			"startTime": []string{strconv.FormatInt(startDay.UnixMilli(), 10)},
			"endTime":   []string{strconv.FormatInt(endDay.UnixMilli(), 10)},
			"limit":     []string{"1000"},
		}
		body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/income", signed: true, params: p})
		if err != nil {
			incomeCh <- incomeResult{err: err}
			return
		}
		var items []incomeItem
		_ = unmarshalIgnore(body, &items)
		incomeCh <- incomeResult{items: items}
	}()

	go func() {
		v, ok := freeUSDTFloat(ctx)
		marginCh <- marginResult{v: v, ok: ok}
	}()

	posRes := <-posCh
	algoRes := <-algoCh
	incomeRes := <-incomeCh
	marginRes := <-marginCh

	if posRes.err != nil {
		return mcp.NewToolResultError("fetch positions: " + humanizeError(posRes.err)), nil
	}

	type algoKey struct{ symbol, positionSide string }
	algoMap := make(map[algoKey][]map[string]interface{})
	if algoRes.err == nil {
		for _, o := range algoRes.orders {
			sym, _ := o["symbol"].(string)
			pSide, _ := o["positionSide"].(string)
			k := algoKey{sym, pSide}
			algoMap[k] = append(algoMap[k], o)
		}
	}

	algoOrderSummary := func(o map[string]interface{}) map[string]interface{} {
		trigger, _ := o["triggerPrice"].(string)
		workingType, _ := o["workingType"].(string)
		return map[string]interface{}{
			"algo_id":       o["algoId"],
			"trigger_price": trigger,
			"working_type":  workingType,
		}
	}

	isStopType := func(ot string) bool {
		ot = strings.ToUpper(ot)
		return ot == "STOP_MARKET" || ot == "STOP"
	}
	isTakeProfitType := func(ot string) bool {
		ot = strings.ToUpper(ot)
		return ot == "TAKE_PROFIT_MARKET" || ot == "TAKE_PROFIT"
	}

	var positions []map[string]interface{}
	totalUPnL := 0.0
	attentionNeeded := make([]string, 0)

	for _, pos := range posRes.rows {
		amt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if amt == 0 {
			continue
		}
		markPrice, _ := strconv.ParseFloat(pos.MarkPrice, 64)
		entryPrice, _ := strconv.ParseFloat(pos.EntryPrice, 64)
		uPnL, _ := strconv.ParseFloat(pos.UnRealizedPnL, 64)
		lev, _ := strconv.ParseFloat(pos.Leverage, 64)
		totalUPnL += uPnL
		isLong := amt > 0

		pnlPct := 0.0
		if entryPrice > 0 {
			pnlPct = uPnL / (math.Abs(amt) * entryPrice) * 100
		}
		pnlPctOnMargin := 0.0
		if lev > 0 {
			pnlPctOnMargin = pnlPct * lev
		}

		matchedOrders := algoMap[algoKey{pos.Symbol, pos.PositionSide}]

		var slOrders, tpOrders []map[string]interface{}
		for _, o := range matchedOrders {
			orderType, _ := o["orderType"].(string)
			switch {
			case isStopType(orderType):
				slOrders = append(slOrders, o)
			case isTakeProfitType(orderType):
				tpOrders = append(tpOrders, o)
			}
		}

		noSL := len(slOrders) == 0
		noTP := len(tpOrders) == 0
		hasDuplicates := len(slOrders) > 1 || len(tpOrders) > 1

		slSummaries := make([]map[string]interface{}, 0, len(slOrders))
		for _, o := range slOrders {
			slSummaries = append(slSummaries, algoOrderSummary(o))
		}
		tpSummaries := make([]map[string]interface{}, 0, len(tpOrders))
		for _, o := range tpOrders {
			tpSummaries = append(tpSummaries, algoOrderSummary(o))
		}

		posEntry := map[string]interface{}{
			"symbol":            pos.Symbol,
			"side":              positionSideHuman(amt),
			"position_side":     pos.PositionSide,
			"position_amt":      pos.PositionAmt,
			"entry_price":       pos.EntryPrice,
			"mark_price":        pos.MarkPrice,
			"unrealized_pnl":    pos.UnRealizedPnL,
			"pnl_pct":           roundTo(pnlPct, 4),
			"pnl_pct_on_margin": roundTo(pnlPctOnMargin, 4),
			"leverage":          pos.Leverage,
			"margin_type":       pos.MarginType,
			"liquidation_price": pos.LiquidationPrice,
			"notional":          pos.Notional,
			"sl_orders":         slSummaries,
			"tp_orders":         tpSummaries,
			"has_duplicates":    hasDuplicates,
		}

		if len(slOrders) > 0 && markPrice > 0 {
			slTrigger, err := strconv.ParseFloat(slOrders[0]["triggerPrice"].(string), 64)
			if err == nil {
				d := math.Abs(markPrice-slTrigger) / markPrice * 100
				posEntry["sl_distance_from_mark_pct"] = roundTo(d, 4)

				if len(tpOrders) > 0 {
					tpTrigger, err2 := strconv.ParseFloat(tpOrders[0]["triggerPrice"].(string), 64)
					if err2 == nil {
						d2 := math.Abs(tpTrigger-markPrice) / markPrice * 100
						posEntry["tp_distance_from_mark_pct"] = roundTo(d2, 4)

						valid := false
						if isLong {
							valid = slTrigger < entryPrice && tpTrigger > entryPrice
						} else {
							valid = slTrigger > entryPrice && tpTrigger < entryPrice
						}
						posEntry["sl_tp_structurally_valid"] = valid

						if valid && d > 0 {
							positionNotional := math.Abs(amt) * markPrice
							rr := roundTo(d2/d, 4)
							posEntry["remaining_rr_ratio"] = rr
							posEntry["remaining_profit_usdt"] = roundTo(positionNotional*d2/100, 2)
							posEntry["remaining_risk_usdt"] = roundTo(positionNotional*d/100, 2)
							if rr < 1.0 {
								attentionNeeded = append(attentionNeeded, fmt.Sprintf("%s: R:R below 1.0 (%.2f)", pos.Symbol, rr))
							}
						}
					}
				}
			}
		}

		if noSL {
			attentionNeeded = append(attentionNeeded, pos.Symbol+": no stop-loss set")
		}
		if noTP {
			attentionNeeded = append(attentionNeeded, pos.Symbol+": no take-profit set")
		}
		if hasDuplicates {
			attentionNeeded = append(attentionNeeded, pos.Symbol+": duplicate SL or TP orders found")
		}

		positions = append(positions, posEntry)
	}

	if positions == nil {
		positions = []map[string]interface{}{}
	}

	sums := map[string]float64{}
	for _, item := range incomeRes.items {
		v, _ := strconv.ParseFloat(item.Income, 64)
		sums[item.IncomeType] += v
	}
	realizedPnL := sums["REALIZED_PNL"]
	commission := sums["COMMISSION"]
	fundingFee := sums["FUNDING_FEE"]

	todaysPnL := map[string]interface{}{
		"date":               dateStr,
		"realized_pnl":       roundTo(realizedPnL, 6),
		"commission":         roundTo(commission, 6),
		"funding_fee":        roundTo(fundingFee, 6),
		"trading_net_profit": roundTo(realizedPnL+commission+fundingFee, 6),
	}
	if incomeRes.err != nil {
		todaysPnL["error"] = humanizeError(incomeRes.err)
	}

	result := map[string]interface{}{
		"generated_at_utc":     now.Format(time.RFC3339),
		"free_margin_usdt":     roundTo(marginRes.v, 4),
		"total_unrealized_pnl": roundTo(totalUPnL, 6),
		"positions":            positions,
		"todays_pnl":           todaysPnL,
		"attention_needed":     attentionNeeded,
	}
	if algoRes.err != nil {
		result["algo_orders_error"] = humanizeError(algoRes.err)
	}

	b, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}
