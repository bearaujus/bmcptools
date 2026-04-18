package binance

import (
	"context"
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

func accountInfoHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return signedGetText(ctx, "/fapi/v2/account", nil)
}

func positionRiskHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := url.Values{}
	if s := strings.TrimSpace(req.GetString("symbol", "")); s != "" {
		p.Set("symbol", strings.ToUpper(s))
	}
	return signedGetText(ctx, "/fapi/v2/positionRisk", p)
}

func openOrdersHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := url.Values{}
	if s := strings.TrimSpace(req.GetString("symbol", "")); s != "" {
		p.Set("symbol", strings.ToUpper(s))
	}
	return signedGetText(ctx, "/fapi/v1/openOrders", p)
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
	body, _, err := request(ctx, requestOpts{method: http.MethodGet, path: "/fapi/v2/account", signed: true})
	if err != nil {
		return ""
	}
	var payload struct {
		Assets []struct {
			Asset            string `json:"asset"`
			AvailableBalance string `json:"availableBalance"`
		} `json:"assets"`
	}
	if err := unmarshalIgnore(body, &payload); err != nil {
		return ""
	}
	for _, a := range payload.Assets {
		if a.Asset == "USDT" {
			return a.AvailableBalance
		}
	}
	return ""
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
