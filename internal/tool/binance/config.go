package binance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func configureSymbolHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.GetString("symbol", "")))
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}

	lev := int64(req.GetFloat("leverage", 0))
	mt := strings.ToUpper(strings.TrimSpace(req.GetString("margin_type", "")))

	if lev <= 0 && mt == "" {
		return mcp.NewToolResultError("at least one of leverage or margin_type must be provided"), nil
	}
	if mt != "" && mt != "ISOLATED" && mt != "CROSSED" {
		return mcp.NewToolResultError("margin_type must be ISOLATED or CROSSED"), nil
	}

	reasoning := strings.TrimSpace(req.GetString("reasoning", ""))
	if reasoning == "" && !skipAskUser() {
		return mcp.NewToolResultError("reasoning is required unless BINANCE_SKIP_ASK_USER=true"), nil
	}

	var actions []string
	if mt != "" {
		actions = append(actions, fmt.Sprintf("margin_type -> %s", mt))
	}
	if lev > 0 {
		actions = append(actions, fmt.Sprintf("leverage -> %dx", lev))
	}

	brief := tradeBrief{
		Action:    fmt.Sprintf("Configure %s: %s", symbol, strings.Join(actions, ", ")),
		Market:    []string{"Free margin: " + dash(freeUSDT(ctx)), "Open positions: " + openPositionsSummary(ctx, symbol)},
		Reasoning: reasoning,
	}.Render()
	ep := make([]editParam, 0, 2)
	if lev > 0 {
		ep = append(ep, editParam{Key: "leverage", Label: "Leverage", Value: strconv.FormatInt(lev, 10), Type: "number", Step: "1", Min: "1"})
	}
	if mt != "" {
		ep = append(ep, editParam{Key: "margin_type", Label: "Margin Type (ISOLATED/CROSSED)", Value: mt, Type: "text"})
	}
	outcome, editedParams, err := confirmOrSkipEditable(ctx, "Configure symbol", brief, ep)
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if outcome == confirmTimedOut {
		return mcp.NewToolResultError("configure_symbol confirmation timed out — user did not respond"), nil
	}
	if outcome == confirmHumanRejected {
		return mcp.NewToolResultError("configure_symbol rejected by user"), nil
	}
	if lev > 0 {
		if v, ok := editedParams["leverage"]; ok && v != "" {
			if f, err2 := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err2 == nil && f > 0 {
				lev = f
			}
		}
	}
	if mt != "" {
		if v, ok := editedParams["margin_type"]; ok {
			v = strings.ToUpper(strings.TrimSpace(v))
			if v == "ISOLATED" || v == "CROSSED" {
				mt = v
			}
		}
	}

	var sb strings.Builder
	var lastHdr http.Header

	// margin_type first (matches bracketOrderHandler order; -4046 = already set, treat as no-op)
	if mt != "" {
		p := url.Values{"symbol": []string{symbol}, "marginType": []string{mt}}
		body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/marginType", signed: true, params: p})
		lastHdr = hdr
		if err != nil {
			var be *binanceError
			if errors.As(err, &be) && be.Code == -4046 {
				sb.WriteString(fmt.Sprintf("[margin_type] already %s (no change needed)\n", mt))
			} else {
				return mcp.NewToolResultError(fmt.Sprintf("set margin_type: %s", humanizeError(err))), nil
			}
		} else {
			sb.WriteString(fmt.Sprintf("[margin_type] %s\n", prettyJSON(body)))
		}
	}

	// leverage second
	if lev > 0 {
		p := url.Values{"symbol": []string{symbol}, "leverage": []string{strconv.FormatInt(lev, 10)}}
		body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/leverage", signed: true, params: p})
		lastHdr = hdr
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("set leverage: %s", humanizeError(err))), nil
		}
		sb.WriteString(fmt.Sprintf("[leverage] %s\n", prettyJSON(body)))
	}

	return mcp.NewToolResultText(outcome.prefix() + sb.String() + rateLimitFooter(lastHdr)), nil
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
	outcome, err := confirmOrSkip(ctx, "Change position mode", brief)
	if err != nil {
		return mcp.NewToolResultError("confirm failed: " + err.Error()), nil
	}
	if outcome == confirmTimedOut {
		return mcp.NewToolResultError("change_position_mode confirmation timed out — user did not respond"), nil
	}
	if outcome == confirmHumanRejected {
		return mcp.NewToolResultError("change_position_mode rejected by user"), nil
	}

	p := url.Values{"dualSidePosition": []string{strconv.FormatBool(dual)}}
	body, hdr, err := request(ctx, requestOpts{method: http.MethodPost, path: "/fapi/v1/positionSide/dual", signed: true, params: p})
	if err != nil {
		return mcp.NewToolResultError(humanizeError(err)), nil
	}
	return mcp.NewToolResultText(outcome.prefix() + prettyJSON(body) + rateLimitFooter(hdr)), nil
}
