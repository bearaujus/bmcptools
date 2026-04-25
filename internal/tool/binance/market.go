package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// simpleGetHandler builds a handler that does a public GET and returns the body.
func simpleGetHandler(path string, buildParams func(req mcp.CallToolRequest) (url.Values, error)) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params, err := buildParams(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: path, params: params})
		if err != nil {
			return mcp.NewToolResultError(humanizeError(err)), nil
		}
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}
}

func pingHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/ping"})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	if err := syncTime(ctx); err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("ping ok; body=%s; time-sync failed: %v%s", string(body), err, rateLimitFooter(hdr))), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("ping ok; skew_ms=%d (server - local)%s", defaultClient.skewMs.Load(), rateLimitFooter(hdr))), nil
}

var exchangeInfoHandler = simpleGetHandler("/fapi/v1/exchangeInfo", func(req mcp.CallToolRequest) (url.Values, error) {
	p := url.Values{}
	if s := strings.TrimSpace(req.GetString("symbol", "")); s != "" {
		p.Set("symbol", strings.ToUpper(s))
	}
	return p, nil
})

func symbolSpecsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	si, err := fetchSymbolInfo(ctx, symbol)
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	filters := reduceSymbolFilters(si)

	// Leverage bracket — /fapi/v1/leverageBracket is auth-only; expose if creds present.
	var maxLeverage interface{}
	if _, _, err := apiKeys(); err == nil {
		lbBody, _, lbErr := request(ctx, requestOpts{
			method: http.MethodGet,
			path:   "/fapi/v1/leverageBracket",
			signed: true,
			params: url.Values{"symbol": []string{symbol}},
		})
		if lbErr == nil {
			var lb []struct {
				Symbol   string `json:"symbol"`
				Brackets []struct {
					InitialLeverage int `json:"initialLeverage"`
				} `json:"brackets"`
			}
			if err := json.Unmarshal(lbBody, &lb); err == nil {
				for _, e := range lb {
					if strings.EqualFold(e.Symbol, symbol) && len(e.Brackets) > 0 {
						maxLeverage = e.Brackets[0].InitialLeverage
						break
					}
				}
			}
		}
	}

	out := map[string]interface{}{
		"symbol":            si.Symbol,
		"pair":              si.Pair,
		"contractType":      si.ContractType,
		"status":            si.Status,
		"baseAsset":         si.BaseAsset,
		"quoteAsset":        si.QuoteAsset,
		"pricePrecision":    si.PricePrecision,
		"quantityPrecision": si.QuantityPrecision,
		"orderTypes":        si.OrderTypes,
		"timeInForce":       si.TimeInForce,
		"marginTypes":       []string{"ISOLATED", "CROSSED"},
	}
	if maxLeverage != nil {
		out["maxLeverage"] = maxLeverage
	}
	for _, k := range sortedKeys(filters) {
		out[k] = filters[k]
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

var klinesHandler = simpleGetHandler("/fapi/v1/klines", func(req mcp.CallToolRequest) (url.Values, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	interval := strings.TrimSpace(req.GetString("interval", ""))
	if interval == "" {
		return nil, fmt.Errorf("interval is required")
	}
	p := url.Values{}
	p.Set("symbol", symbol)
	p.Set("interval", interval)
	if limit := req.GetFloat("limit", 0); limit > 0 {
		p.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	if st := req.GetFloat("start_time_ms", 0); st > 0 {
		p.Set("startTime", strconv.FormatInt(int64(st), 10))
	}
	if et := req.GetFloat("end_time_ms", 0); et > 0 {
		p.Set("endTime", strconv.FormatInt(int64(et), 10))
	}
	return p, nil
})

var tickerPriceHandler = simpleGetHandler("/fapi/v1/ticker/price", func(req mcp.CallToolRequest) (url.Values, error) {
	p := url.Values{}
	if s := strings.TrimSpace(req.GetString("symbol", "")); s != "" {
		p.Set("symbol", strings.ToUpper(s))
	}
	return p, nil
})

var ticker24hrHandler = simpleGetHandler("/fapi/v1/ticker/24hr", func(req mcp.CallToolRequest) (url.Values, error) {
	p := url.Values{}
	if s := strings.TrimSpace(req.GetString("symbol", "")); s != "" {
		p.Set("symbol", strings.ToUpper(s))
	}
	return p, nil
})

// marketScanHandler fetches all 24hr tickers, filters by min volume, and returns
// the top N symbols sorted by quoteVolume or |priceChangePercent| — compact output
// for fast symbol selection without flooding the AI context with 500+ symbols.
func marketScanHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sortBy := strings.ToLower(strings.TrimSpace(req.GetString("sort_by", "volume")))
	if sortBy != "volume" && sortBy != "change_pct" {
		return mcp.NewToolResultError("sort_by must be \"volume\" or \"change_pct\""), nil
	}
	limit := int(req.GetFloat("limit", 20))
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	minVol := req.GetFloat("min_volume_usdt", 100_000_000)

	type tickerResult struct {
		body []byte
		hdr  http.Header
		err  error
	}
	type balResult struct {
		v  float64
		ok bool
	}
	tickerCh := make(chan tickerResult, 1)
	balCh := make(chan balResult, 1)
	go func() {
		b, h, e := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/ticker/24hr"})
		tickerCh <- tickerResult{b, h, e}
	}()
	go func() {
		v, ok := freeUSDTFloat(ctx)
		balCh <- balResult{v, ok}
	}()
	tickerRes := <-tickerCh
	balRes := <-balCh

	if tickerRes.err != nil {
		return mcp.NewToolResultError(humanizeError(tickerRes.err)), nil
	}
	body, hdr := tickerRes.body, tickerRes.hdr

	type ticker struct {
		Symbol             string `json:"symbol"`
		LastPrice          string `json:"lastPrice"`
		PriceChange        string `json:"priceChange"`
		PriceChangePercent string `json:"priceChangePercent"`
		HighPrice          string `json:"highPrice"`
		LowPrice           string `json:"lowPrice"`
		QuoteVolume        string `json:"quoteVolume"`
	}
	var tickers []ticker
	if err := json.Unmarshal(body, &tickers); err != nil {
		return mcp.NewToolResultError("failed to parse tickers: " + err.Error()), nil
	}

	type row struct {
		Symbol     string  `json:"symbol"`
		LastPrice  string  `json:"last_price"`
		Change24h  string  `json:"change_24h_pct"`
		High24h    string  `json:"high_24h"`
		Low24h     string  `json:"low_24h"`
		VolUSDT    float64 `json:"volume_usdt"`
		ChangePct  float64 `json:"-"`
	}

	var rows []row
	for _, t := range tickers {
		vol, _ := strconv.ParseFloat(t.QuoteVolume, 64)
		if vol < minVol {
			continue
		}
		pct, _ := strconv.ParseFloat(t.PriceChangePercent, 64)
		rows = append(rows, row{
			Symbol:    t.Symbol,
			LastPrice: t.LastPrice,
			Change24h: t.PriceChangePercent + "%",
			High24h:   t.HighPrice,
			Low24h:    t.LowPrice,
			VolUSDT:   math.Round(vol),
			ChangePct: math.Abs(pct),
		})
	}

	if sortBy == "change_pct" {
		sort.Slice(rows, func(i, j int) bool { return rows[i].ChangePct > rows[j].ChangePct })
	} else {
		sort.Slice(rows, func(i, j int) bool { return rows[i].VolUSDT > rows[j].VolUSDT })
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}

	result := map[string]interface{}{
		"sort_by": sortBy,
		"count":   len(rows),
		"symbols": rows,
	}
	if balRes.ok {
		result["free_margin_usdt"] = roundTo(balRes.v, 4)
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out) + rateLimitFooter(hdr)), nil
}

func orderBookHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	p := url.Values{"symbol": []string{symbol}}
	if limit := req.GetFloat("limit", 0); limit > 0 {
		p.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/depth", params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}

	// Parse bids/asks for summary computation.
	var raw struct {
		LastUpdateID int64           `json:"lastUpdateId"`
		Bids         [][]interface{} `json:"bids"`
		Asks         [][]interface{} `json:"asks"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}

	parseLevel := func(entry []interface{}) (price, qty float64) {
		if len(entry) < 2 {
			return 0, 0
		}
		ps, _ := entry[0].(string)
		qs, _ := entry[1].(string)
		price, _ = strconv.ParseFloat(ps, 64)
		qty, _ = strconv.ParseFloat(qs, 64)
		return
	}

	depthUSDT := func(levels [][]interface{}, n int) float64 {
		total := 0.0
		for i := 0; i < n && i < len(levels); i++ {
			p, q := parseLevel(levels[i])
			total += p * q
		}
		return total
	}

	summary := map[string]interface{}{}
	if len(raw.Bids) > 0 && len(raw.Asks) > 0 {
		bestBid, _ := parseLevel(raw.Bids[0])
		bestAsk, _ := parseLevel(raw.Asks[0])
		spread := bestAsk - bestBid
		mid := (bestBid + bestAsk) / 2
		spreadPct := 0.0
		if mid > 0 {
			spreadPct = spread / mid * 100
		}
		summary["best_bid"] = bestBid
		summary["best_ask"] = bestAsk
		summary["spread"] = roundTo(spread, 8)
		summary["spread_pct"] = roundTo(spreadPct, 6)
		summary["bid_depth_usdt_top5"] = roundTo(depthUSDT(raw.Bids, 5), 2)
		summary["ask_depth_usdt_top5"] = roundTo(depthUSDT(raw.Asks, 5), 2)
	}

	result := map[string]interface{}{
		"summary": summary,
		"bids":    raw.Bids,
		"asks":    raw.Asks,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(out) + rateLimitFooter(hdr)), nil
}

var markPriceHandler = simpleGetHandler("/fapi/v1/premiumIndex", func(req mcp.CallToolRequest) (url.Values, error) {
	p := url.Values{}
	if s := strings.TrimSpace(req.GetString("symbol", "")); s != "" {
		p.Set("symbol", strings.ToUpper(s))
	}
	return p, nil
})

func openInterestHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	history := req.GetBool("history", false)
	p := url.Values{"symbol": []string{symbol}}
	if !history {
		body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/openInterest", params: p})
		if err != nil {
			return mcp.NewToolResultError(humanizeError(err)), nil
		}
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}
	period := strings.TrimSpace(req.GetString("period", "1h"))
	p.Set("period", period)
	if limit := req.GetFloat("limit", 0); limit > 0 {
		p.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/futures/data/openInterestHist", params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}

func longShortRatioHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	mode := strings.ToLower(strings.TrimSpace(req.GetString("mode", "top")))
	path := "/futures/data/topLongShortAccountRatio"
	if mode == "global" {
		path = "/futures/data/globalLongShortAccountRatio"
	}
	p := url.Values{"symbol": []string{symbol}}
	p.Set("period", strings.TrimSpace(req.GetString("period", "1h")))
	if limit := req.GetFloat("limit", 0); limit > 0 {
		p.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: path, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}
