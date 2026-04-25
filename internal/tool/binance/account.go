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

func signedGetText(ctx context.Context, path string, params url.Values) (*mcp.CallToolResult, error) {
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: path, signed: true, params: params})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}

func openOrdersHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := url.Values{}
	symbol := strings.TrimSpace(req.GetString("symbol", ""))
	if symbol != "" {
		p.Set("symbol", strings.ToUpper(symbol))
	}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v1/openOrders", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}

	if !req.GetBool("include_algo_orders", true) {
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}

	// Parse regular orders preserving numeric precision.
	dec := jsonDecoderUseNumber(body)
	var regularOrders []interface{}
	if err := dec.Decode(&regularOrders); err != nil {
		return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
	}
	if regularOrders == nil {
		regularOrders = []interface{}{}
	}

	algoOrders, algoErr := fetchOpenAlgoOrders(ctx, strings.ToUpper(symbol))

	type openOrdersResponse struct {
		RegularOrders   []interface{}            `json:"regular_orders"`
		AlgoOrders      []map[string]interface{} `json:"algo_orders,omitempty"`
		AlgoOrdersError string                   `json:"algo_orders_error,omitempty"`
		Total           int                      `json:"total"`
	}
	resp := openOrdersResponse{RegularOrders: regularOrders}
	if algoErr != nil {
		resp.AlgoOrdersError = humanizeError(algoErr)
		resp.Total = len(regularOrders)
	} else {
		if algoOrders == nil {
			algoOrders = []map[string]interface{}{}
		}
		slimmed := make([]map[string]interface{}, len(algoOrders))
		for i, o := range algoOrders {
			slimmed[i] = slimAlgoOrder(o)
		}
		resp.AlgoOrders = slimmed
		resp.Total = len(regularOrders) + len(algoOrders)
	}

	out, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(out) + rateLimitFooter(hdr)), nil
}

func orderHistoryHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	return signedGetText(ctx, "/fapi/v1/allOrders", p)
}

func incomeHistoryHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := url.Values{}
	if s := strings.TrimSpace(req.GetString("symbol", "")); s != "" {
		p.Set("symbol", strings.ToUpper(s))
	}
	if it := strings.TrimSpace(req.GetString("income_type", "")); it != "" {
		p.Set("incomeType", strings.ToUpper(it))
	}
	if limit := req.GetFloat("limit", 0); limit > 0 {
		p.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	if st := req.GetFloat("start_time_ms", 0); st > 0 {
		p.Set("startTime", strconv.FormatInt(int64(st), 10))
	}
	if et := req.GetFloat("end_time_ms", 0); et > 0 {
		p.Set("endTime", strconv.FormatInt(int64(et), 10))
	}
	return signedGetText(ctx, "/fapi/v1/income", p)
}

// freeUSDT returns the AvailableBalance of the USDT asset from /fapi/v2/account.
// Used by the trade brief builder.
func freeUSDT(ctx context.Context) string {
	v, ok := freeUSDTFloat(ctx)
	if !ok {
		return ""
	}
	return strconv.FormatFloat(v, 'f', 4, 64)
}

// freeUSDTFloat returns the account's available USDT balance as a float.
// Returns (0, false) on any error or missing credentials.
func freeUSDTFloat(ctx context.Context) (float64, bool) {
	body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/account", signed: true})
	if err != nil {
		return 0, false
	}
	var payload struct {
		Assets []struct {
			Asset            string `json:"asset"`
			AvailableBalance string `json:"availableBalance"`
		} `json:"assets"`
	}
	if err := unmarshalIgnore(body, &payload); err != nil {
		return 0, false
	}
	for _, a := range payload.Assets {
		if a.Asset == "USDT" {
			v, err := strconv.ParseFloat(a.AvailableBalance, 64)
			if err != nil {
				return 0, false
			}
			return v, true
		}
	}
	return 0, false
}

// openPositionsSummary returns a short human line about positions, filtered by symbol if non-empty.
func openPositionsSummary(ctx context.Context, symbol string) string {
	p := url.Values{}
	if symbol != "" {
		p.Set("symbol", symbol)
	}
	body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/positionRisk", signed: true, params: p})
	if err != nil {
		return ""
	}
	var positions []struct {
		Symbol        string `json:"symbol"`
		PositionAmt   string `json:"positionAmt"`
		EntryPrice    string `json:"entryPrice"`
		UnRealizedPnL string `json:"unRealizedProfit"`
		Leverage      string `json:"leverage"`
		MarginType    string `json:"marginType"`
	}
	if err := unmarshalIgnore(body, &positions); err != nil {
		return ""
	}
	var lines []string
	for _, pos := range positions {
		amt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if amt == 0 {
			continue
		}
		side := "LONG"
		if amt < 0 {
			side = "SHORT"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s @ %s (uPnL %s, lev %sx %s)", pos.Symbol, side, pos.PositionAmt, pos.EntryPrice, pos.UnRealizedPnL, pos.Leverage, pos.MarginType))
	}
	if len(lines) == 0 {
		return "no open positions"
	}
	return strings.Join(lines, "; ")
}

// unmarshalIgnore is json.Unmarshal that silently swallows parse errors of extra fields.
func unmarshalIgnore(data []byte, v interface{}) error {
	dec := jsonDecoderStrict(data)
	return dec.Decode(v)
}

// slimAlgoOrder trims noisy Binance algo order fields, keeping only what an AI trader needs.
func slimAlgoOrder(o map[string]interface{}) map[string]interface{} {
	keep := []string{"algoId", "symbol", "orderType", "side", "positionSide", "triggerPrice", "closePosition", "quantity", "algoStatus", "workingType", "bookTime"}
	out := make(map[string]interface{}, len(keep))
	for _, k := range keep {
		if v, ok := o[k]; ok {
			out[k] = v
		}
	}
	return out
}
