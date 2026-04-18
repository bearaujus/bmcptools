package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

var orderBookHandler = simpleGetHandler("/fapi/v1/depth", func(req mcp.CallToolRequest) (url.Values, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	p := url.Values{"symbol": []string{symbol}}
	if limit := req.GetFloat("limit", 0); limit > 0 {
		p.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	return p, nil
})

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
