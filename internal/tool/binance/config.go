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

func changeLeverageHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	lev := int64(req.GetFloat("leverage", 0))
	if lev <= 0 {
		return mcp.NewToolResultError("leverage must be > 0"), nil
	}
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}

	brief := tradeBrief{
		Action:    fmt.Sprintf("Change leverage for %s -> %dx", symbol, lev),
		Market:    []string{"Free margin: " + dash(freeUSDT(ctx)), "Open positions: " + openPositionsSummary(ctx, symbol)},
		Reasoning: reasoning,
	}.Render()
	ok, err := confirmOrSkip(ctx, "Change leverage", brief)
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if !ok {
		return mcp.NewToolResultError("change_leverage rejected by user"), nil
	}

	p := url.Values{"symbol": []string{symbol}, "leverage": []string{strconv.FormatInt(lev, 10)}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/leverage", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}

func changeMarginTypeHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}
	mt := strings.ToUpper(strings.TrimSpace(req.GetString("margin_type", "")))
	if mt != "ISOLATED" && mt != "CROSSED" {
		return mcp.NewToolResultError("margin_type must be ISOLATED or CROSSED"), nil
	}
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}

	brief := tradeBrief{
		Action:    fmt.Sprintf("Change margin type for %s -> %s", symbol, mt),
		Market:    []string{"Open positions: " + openPositionsSummary(ctx, symbol)},
		Reasoning: reasoning,
	}.Render()
	ok, err := confirmOrSkip(ctx, "Change margin type", brief)
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if !ok {
		return mcp.NewToolResultError("change_margin_type rejected by user"), nil
	}

	p := url.Values{"symbol": []string{symbol}, "marginType": []string{mt}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/marginType", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}

func changePositionModeHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dual := req.GetBool("dual_side_position", false)
	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}

	mode := "one-way"
	if dual {
		mode = "hedge (dual-side)"
	}
	brief := tradeBrief{
		Action:    fmt.Sprintf("Switch position mode -> %s (account-wide)", mode),
		Market:    []string{"Open positions: " + openPositionsSummary(ctx, "")},
		Reasoning: reasoning,
	}.Render()
	ok, err := confirmOrSkip(ctx, "Change position mode", brief)
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if !ok {
		return mcp.NewToolResultError("change_position_mode rejected by user"), nil
	}

	p := url.Values{"dualSidePosition": []string{strconv.FormatBool(dual)}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/positionSide/dual", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(prettyJSON(body) + rateLimitFooter(hdr)), nil
}
