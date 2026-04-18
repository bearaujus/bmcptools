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

	if !dryRun {
		brief := tradeBrief{
			Action:    fmt.Sprintf("CLOSE %s position on %s (%s %s qty=%s via %s)", positionSideHuman(amt), symbol, side, symbol, trimFloatStr(closeQty), typ),
			Market:    []string{"Current position: " + fmt.Sprintf("%s %s @ %s (uPnL %s)", symbol, trimFloatStr(amt), pos.EntryPrice, pos.UnRealizedPnL)},
			Reasoning: reasoning,
		}.Render()
		ok, cerr := confirmOrSkip(ctx, "Close position", brief)
		if cerr != nil {
			return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
		}
		if !ok {
			return mcp.NewToolResultError("close_position rejected by user"), nil
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
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
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

// =====================================================================
// commission_rate
// =====================================================================

func commissionRateHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	p := url.Values{"symbol": []string{symbol}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/commissionRate", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
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
	brief := tradeBrief{
		Action:    fmt.Sprintf("Modify order %s on %s (%s)", ref, symbol, strings.Join(change, " ")),
		Reasoning: reasoning,
	}.Render()
	ok, cerr := confirmOrSkip(ctx, "Modify order", brief)
	if cerr != nil {
		return mcp.NewToolResultError("confirm failed: " + cerr.Error()), nil
	}
	if !ok {
		return mcp.NewToolResultError("modify_order rejected by user"), nil
	}

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
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
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
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
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
	}
	if err := unmarshalIgnore(acctBody, &acct); err == nil {
		out["account"] = map[string]interface{}{
			"fee_tier":                       acct.FeeTier,
			"total_wallet_balance":           acct.TotalWalletBalance,
			"total_margin_balance":           acct.TotalMarginBalance,
			"total_unrealized_profit":        acct.TotalUnrealizedProfit,
			"available_balance":              acct.AvailableBalance,
			"total_position_initial_margin":  acct.TotalPositionInitialMargin,
			"total_open_order_initial_margin": acct.TotalOpenOrderInitialMargin,
			"max_withdraw_amount":            acct.MaxWithdrawAmount,
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

	p := url.Values{
		"symbol":   []string{symbol},
		"interval": []string{interval},
		"limit":    []string{strconv.Itoa(limit)},
	}
	body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/klines", params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	var raw [][]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return mcp.NewToolResultError("parse klines: " + err.Error()), nil
	}
	if len(raw) < 21 {
		return mcp.NewToolResultError(fmt.Sprintf("not enough candles for indicators (got %d, need >= 21)", len(raw))), nil
	}

	// Extract OHLC.
	n := len(raw)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	opens := make([]float64, n)
	vols := make([]float64, n)
	openTimes := make([]int64, n)
	for i, c := range raw {
		if len(c) < 6 {
			return mcp.NewToolResultError("malformed kline row"), nil
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

	// ATR(14)
	atr14 := atr(highs, lows, closes, 14)
	// EMA 9/21/50
	ema9 := ema(closes, 9)
	ema21 := ema(closes, 21)
	var ema50 *float64
	if n >= 50 {
		v := ema(closes, 50)
		ema50 = &v
	}
	// RSI(14)
	rsi14 := rsi(closes, 14)
	// Bollinger(20, 2)
	bbMid, bbUp, bbLow := bollinger(closes, 20, 2.0)

	// Trend hint
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
			"atr_14":         roundTo(atr14, 8),
			"atr_14_pct":     roundTo(atr14/lastClose*100, 4),
			"ema_9":          roundTo(ema9, 8),
			"ema_21":         roundTo(ema21, 8),
			"ema_50":         ema50Value(ema50),
			"rsi_14":         roundTo(rsi14, 2),
			"bb_20_mid":      roundTo(bbMid, 8),
			"bb_20_upper":    roundTo(bbUp, 8),
			"bb_20_lower":    roundTo(bbLow, 8),
			"bb_width_pct":   roundTo((bbUp-bbLow)/bbMid*100, 4),
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
	out, _ := json.MarshalIndent(snap, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func ema50Value(p *float64) interface{} {
	if p == nil {
		return nil
	}
	return roundTo(*p, 8)
}

// ---- TA helpers ----

func ema(v []float64, period int) float64 {
	if len(v) < period {
		return math.NaN()
	}
	k := 2.0 / float64(period+1)
	// Seed with SMA of first `period`.
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
	rs := avgG / avgL
	return 100 - 100/(1+rs)
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
	// Wilder's smoothing.
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
