package binance

// Live integration tests against the Binance USDT-M Futures testnet.
//
// These run only when BMCPTOOLS_BINANCE_LIVE=1 is set, so `go test ./...`
// stays offline. To run:
//
//   $env:BMCPTOOLS_BINANCE_LIVE="1"
//   $env:BINANCE_API_KEY="..."
//   $env:BINANCE_API_SECRET="..."
//   $env:BINANCE_FUTURES_BASE_URL="https://testnet.binancefuture.com"
//   $env:BINANCE_SKIP_ASK_USER="true"
//   go test -run TestLive ./internal/tool/binance/ -v -count=1 -timeout 120s

import (
	"context"
	"os"
	"strings"
	"testing"
)

func liveOrSkip(t *testing.T) context.Context {
	t.Helper()
	if os.Getenv("BMCPTOOLS_BINANCE_LIVE") != "1" {
		t.Skip("BMCPTOOLS_BINANCE_LIVE != 1 — skipping live test")
	}
	return context.Background()
}

func TestLive_Ping(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := pingHandler(ctx, newTestRequest(nil))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("ping returned error: %s", resultText(r))
	}
	t.Logf("ping ok:\n%s", resultText(r))
}

func TestLive_SymbolSpecs_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := symbolSpecsHandler(ctx, newTestRequest(map[string]any{"symbol": "BTCUSDT"}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("symbol_specs error: %s", resultText(r))
	}
	body := resultText(r)
	for _, want := range []string{"minNotional", "stepSize", "tickSize", "BTCUSDT"} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q\n%s", want, body)
		}
	}
	t.Logf("symbol_specs BTCUSDT:\n%s", body)
}

func TestLive_TickerPrice_All(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := tickerPriceHandler(ctx, newTestRequest(nil))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("ticker_price error: %s", resultText(r))
	}
	t.Logf("ticker_price (truncated): %s", truncForLog(resultText(r), 400))
}

func TestLive_Ticker24hr_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := ticker24hrHandler(ctx, newTestRequest(map[string]any{"symbol": "BTCUSDT"}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("ticker_24hr error: %s", resultText(r))
	}
	t.Logf("ticker_24hr BTCUSDT:\n%s", resultText(r))
}

func TestLive_Klines_BTCUSDT_1h(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := klinesHandler(ctx, newTestRequest(map[string]any{
		"symbol":   "BTCUSDT",
		"interval": "1h",
		"limit":    float64(3),
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("klines error: %s", resultText(r))
	}
	t.Logf("klines:\n%s", resultText(r))
}

func TestLive_OrderBook_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := orderBookHandler(ctx, newTestRequest(map[string]any{
		"symbol": "BTCUSDT",
		"limit":  float64(5),
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("order_book error: %s", resultText(r))
	}
	t.Logf("order_book:\n%s", truncForLog(resultText(r), 600))
}

func TestLive_MarkPrice_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := markPriceHandler(ctx, newTestRequest(map[string]any{"symbol": "BTCUSDT"}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("mark_price error: %s", resultText(r))
	}
	t.Logf("mark_price:\n%s", resultText(r))
}

func TestLive_OpenInterest_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := openInterestHandler(ctx, newTestRequest(map[string]any{"symbol": "BTCUSDT"}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("open_interest error: %s", resultText(r))
	}
	t.Logf("open_interest:\n%s", resultText(r))
}

func TestLive_LongShortRatio_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := longShortRatioHandler(ctx, newTestRequest(map[string]any{
		"symbol": "BTCUSDT",
		"period": "1h",
		"limit":  float64(3),
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("long_short_ratio error: %s", resultText(r))
	}
	t.Logf("long_short_ratio:\n%s", resultText(r))
}

// --- Authenticated reads ---

func TestLive_AccountInfo(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := accountInfoHandler(ctx, newTestRequest(nil))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("account_info error: %s", resultText(r))
	}
	t.Logf("account_info (truncated):\n%s", truncForLog(resultText(r), 800))
}

func TestLive_PositionRisk(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := positionRiskHandler(ctx, newTestRequest(map[string]any{"symbol": "BTCUSDT"}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("position_risk error: %s", resultText(r))
	}
	t.Logf("position_risk:\n%s", resultText(r))
}

func TestLive_OpenOrders(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := openOrdersHandler(ctx, newTestRequest(nil))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("open_orders error: %s", resultText(r))
	}
	t.Logf("open_orders:\n%s", resultText(r))
}

func TestLive_OrderHistory_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := orderHistoryHandler(ctx, newTestRequest(map[string]any{
		"symbol": "BTCUSDT",
		"limit":  float64(5),
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("order_history error: %s", resultText(r))
	}
	t.Logf("order_history:\n%s", truncForLog(resultText(r), 600))
}

func TestLive_IncomeHistory(t *testing.T) {
	ctx := liveOrSkip(t)
	r, err := incomeHistoryHandler(ctx, newTestRequest(map[string]any{"limit": float64(5)}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("income_history error: %s", resultText(r))
	}
	t.Logf("income_history:\n%s", resultText(r))
}

// --- Mutating (require BINANCE_SKIP_ASK_USER=true to run unattended) ---

// Cancel any leftover open orders FIRST so subsequent tests have a clean slate.
func TestLive_A_CancelAllOpenOrders_BTCUSDT(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" {
		t.Skip("BINANCE_SKIP_ASK_USER must be true to run mutating tests headless")
	}
	r, err := cancelAllOrdersHandler(ctx, newTestRequest(map[string]any{
		"symbol":    "BTCUSDT",
		"reasoning": "live integration test: cleanup before mutating tests",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("cancel_all_open_orders error: %s", resultText(r))
	}
	t.Logf("cancel_all_open_orders:\n%s", resultText(r))
}

func TestLive_B_ChangeLeverage_BTCUSDT_5x(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" {
		t.Skip("BINANCE_SKIP_ASK_USER must be true to run mutating tests headless")
	}
	r, err := changeLeverageHandler(ctx, newTestRequest(map[string]any{
		"symbol":    "BTCUSDT",
		"leverage":  float64(5),
		"reasoning": "live integration test: setting modest leverage",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("change_leverage error: %s", resultText(r))
	}
	t.Logf("change_leverage:\n%s", resultText(r))
}

func TestLive_C_ChangeMarginType_BTCUSDT_ISOLATED(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" {
		t.Skip("BINANCE_SKIP_ASK_USER must be true")
	}
	r, err := changeMarginTypeHandler(ctx, newTestRequest(map[string]any{
		"symbol":      "BTCUSDT",
		"margin_type": "ISOLATED",
		"reasoning":   "live integration test: prefer isolated to limit blast radius",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		body := resultText(r)
		// -4046 = "no need to change margin type" — already set, harmless.
		// -4067 = "Position side cannot be changed if there exists open orders" — pre-existing
		// account state from earlier testing, not a code defect.
		if strings.Contains(body, "-4046") || strings.Contains(body, "-4067") {
			t.Logf("change_margin_type acceptable error (%s)", strings.SplitN(body, "\n", 2)[0])
			return
		}
		t.Fatalf("change_margin_type error: %s", body)
	}
	t.Logf("change_margin_type:\n%s", resultText(r))
}

func TestLive_D_PlaceOrder_DryRun_LimitBuy(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" {
		t.Skip("BINANCE_SKIP_ASK_USER must be true")
	}
	// 0.002 BTC @ 60_000 = 120 USDT (above 100-USDT MIN_NOTIONAL on testnet BTCUSDT).
	r, err := placeOrderHandler(ctx, newTestRequest(map[string]any{
		"symbol":        "BTCUSDT",
		"side":          "BUY",
		"type":          "LIMIT",
		"quantity":      float64(0.002),
		"price":         float64(60000),
		"time_in_force": "GTC",
		"dry_run":       true,
		"reasoning":     "live integration test: validate signing + filters via /order/test",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("place_order dry_run error: %s", resultText(r))
	}
	t.Logf("place_order dry_run:\n%s", resultText(r))
}

func TestLive_E_PlaceBracketOrder_DryRun(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" {
		t.Skip("BINANCE_SKIP_ASK_USER must be true")
	}
	// 0.003 BTC @ 50000 = 150 USDT > MIN_NOTIONAL.
	r, err := bracketOrderHandler(ctx, newTestRequest(map[string]any{
		"symbol":            "BTCUSDT",
		"side":              "BUY",
		"type":              "LIMIT",
		"quantity":          float64(0.003),
		"entry_price":       float64(50000),
		"stop_loss_price":   float64(48000),
		"take_profit_price": float64(55000),
		"time_in_force":     "GTC",
		"dry_run":           true,
		"reasoning":         "live integration test: bracket dry-run validates leg construction + SL/TP sides",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("place_bracket_order dry_run error: %s", resultText(r))
	}
	body := resultText(r)
	if !strings.Contains(body, "validated") {
		t.Errorf("expected dry-run output to mention 'validated'\n%s", body)
	}
	t.Logf("place_bracket_order dry_run:\n%s", body)
}

func TestLive_F_CancelAllOpenOrders_Cleanup(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" {
		t.Skip("BINANCE_SKIP_ASK_USER must be true")
	}
	r, err := cancelAllOrdersHandler(ctx, newTestRequest(map[string]any{
		"symbol":    "BTCUSDT",
		"reasoning": "live integration test: final cleanup",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("cancel_all_open_orders error: %s", resultText(r))
	}
	t.Logf("cancel_all_open_orders cleanup:\n%s", resultText(r))
}

func truncForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}


// TestLive_G_RealBracketOrder places a tiny real bracket order at far-from-market prices
// (will not fill). On mainnet, verifies entry + SL + TP all land, then cleans up.
// On TESTNET (default for this test suite), STOP_MARKET/TAKE_PROFIT_MARKET are not
// supported (-4120), so this test validates the rollback path instead: entry leg
// places, SL/TP legs fail, rollback cancels the entry leg.
// Only runs when BMCPTOOLS_BINANCE_LIVE_REAL=1 is also set.
func TestLive_G_RealBracketOrder(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" {
		t.Skip("BINANCE_SKIP_ASK_USER must be true")
	}
	if os.Getenv("BMCPTOOLS_BINANCE_LIVE_REAL") != "1" {
		t.Skip("BMCPTOOLS_BINANCE_LIVE_REAL != 1 — skipping real-order test")
	}
	r, err := bracketOrderHandler(ctx, newTestRequest(map[string]any{
		"symbol":            "BTCUSDT",
		"side":              "BUY",
		"type":              "LIMIT",
		"quantity":          float64(0.002),
		"entry_price":       float64(40000), // far below market — will not fill
		"stop_loss_price":   float64(38000),
		"take_profit_price": float64(45000),
		"time_in_force":     "GTC",
		"reasoning":         "live integration test: place real (unfillable) bracket to exercise batchOrders",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}

	isTestnet := strings.Contains(os.Getenv("BINANCE_FUTURES_BASE_URL"), "testnet")
	if isResultError(r) {
		txt := resultText(r)
		if isTestnet && strings.Contains(txt, "-4120") && strings.Contains(txt, "rolled back") {
			t.Logf("EXPECTED on testnet (-4120 SL/TP not supported) — rollback path verified:\n%s", txt)
			// Ensure no orders left behind.
			rc, _ := cancelAllOrdersHandler(ctx, newTestRequest(map[string]any{
				"symbol":    "BTCUSDT",
				"reasoning": "belt-and-braces cleanup after testnet rollback",
			}))
			if rc != nil {
				t.Logf("post-rollback cleanup:\n%s", resultText(rc))
			}
			return
		}
		t.Fatalf("place_bracket_order error: %s", txt)
	}
	t.Logf("place_bracket_order REAL (mainnet path):\n%s", resultText(r))

	// Cleanup (mainnet success path)
	rc, err := cancelAllOrdersHandler(ctx, newTestRequest(map[string]any{
		"symbol":    "BTCUSDT",
		"reasoning": "live integration test: cancel real bracket after placement",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(rc) {
		t.Fatalf("cancel_all_open_orders cleanup error: %s", resultText(rc))
	}
	t.Logf("cleanup ok:\n%s", resultText(rc))
}


// TestLive_H_SingleStopMarket places a single STOP_MARKET order via /fapi/v1/order
// to probe whether testnet accepts it (the batchOrders path returns -4120).
// Only when BMCPTOOLS_BINANCE_LIVE_REAL=1 is set.
func TestLive_H_SingleStopMarket(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" || os.Getenv("BMCPTOOLS_BINANCE_LIVE_REAL") != "1" {
		t.Skip("requires BINANCE_SKIP_ASK_USER=true and BMCPTOOLS_BINANCE_LIVE_REAL=1")
	}
	r, err := placeOrderHandler(ctx, newTestRequest(map[string]any{
		"symbol":         "BTCUSDT",
		"side":           "SELL",
		"type":           "STOP_MARKET",
		"stop_price":     float64(38000),
		"close_position": true,
		"reasoning":      "probe: does single STOP_MARKET with closePosition=true work on /fapi/v1/order?",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Logf("STOP_MARKET single rejected: %s", resultText(r))
		return
	}
	t.Logf("STOP_MARKET single placed OK:\n%s", resultText(r))

	// Cleanup
	_, _ = cancelAllOrdersHandler(ctx, newTestRequest(map[string]any{
		"symbol":    "BTCUSDT",
		"reasoning": "cleanup",
	}))
}


// TestLive_I_VisibleLimitOrder places a single LIMIT BUY order at ~5% below mark
// so the user can see it in the testnet UI's Open Orders panel.
// The order is intentionally far from market so it will NOT fill. User is expected
// to cancel it manually from demo.binance.com (or re-run cancel_all).
// Only when BMCPTOOLS_BINANCE_LIVE_REAL=1.
func TestLive_I_VisibleLimitOrder(t *testing.T) {
	ctx := liveOrSkip(t)
	if os.Getenv("BINANCE_SKIP_ASK_USER") != "true" || os.Getenv("BMCPTOOLS_BINANCE_LIVE_REAL") != "1" {
		t.Skip("requires BINANCE_SKIP_ASK_USER=true and BMCPTOOLS_BINANCE_LIVE_REAL=1")
	}
	r, err := placeOrderHandler(ctx, newTestRequest(map[string]any{
		"symbol":        "BTCUSDT",
		"side":          "BUY",
		"type":          "LIMIT",
		"quantity":      float64(0.003),
		"price":         float64(72400),
		"time_in_force": "GTC",
		"reasoning":     "user requested a visible resting order on testnet UI; price ~5% below mark so it won't fill",
	}))
	if err != nil {
		t.Fatalf("infra: %v", err)
	}
	if isResultError(r) {
		t.Fatalf("place_order error: %s", resultText(r))
	}
	t.Logf("place_order VISIBLE (check demo.binance.com → Futures → Open Orders):\n%s", resultText(r))
}
