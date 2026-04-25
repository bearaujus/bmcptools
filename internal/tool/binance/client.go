// Package binance provides MCP tools for interacting with the Binance USDT-M Futures REST API.
//
// Market-data tools require no credentials. Account and trading tools require
// BINANCE_API_KEY and BINANCE_API_SECRET environment variables. Mutating calls
// (change leverage/margin/mode, place/cancel orders) prompt the user for
// confirmation via pkg/confirm unless BINANCE_SKIP_ASK_USER=true.
//
// Base URL is https://fapi.binance.com by default; set BINANCE_TESTNET=true to
// switch to the testnet at https://testnet.binancefuture.com.
package binance

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Environment variable names.
const (
	envAPIKey       = "BINANCE_API_KEY"
	envAPISecret    = "BINANCE_API_SECRET"
	envTestnet      = "BINANCE_TESTNET"
	envSkipAskUser  = "BINANCE_SKIP_ASK_USER"
	envRecvWindowMs = "BINANCE_RECV_WINDOW_MS"
)

const (
	mainnetBaseURL  = "https://fapi.binance.com"
	testnetBaseURL  = "https://demo-fapi.binance.com"
	defaultRecvWin  = 5000
	maxRecvWindowMs = 60000
	clientIDPrefix  = "bmt-"
)

// bclient is the package-level HTTP client. It caches server-time skew so
// signed requests stay inside recvWindow even when the local clock drifts.
type bclient struct {
	http    *http.Client
	skewMs  atomic.Int64 // serverMs - localMs
	mu      sync.Mutex
}

var defaultClient = &bclient{
	http: &http.Client{Timeout: 30 * time.Second},
}

func isTestnetEnv() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(envTestnet)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func baseURL() string {
	if isTestnetEnv() {
		return testnetBaseURL
	}
	return mainnetBaseURL
}

func recvWindowMs() int64 {
	raw := strings.TrimSpace(os.Getenv(envRecvWindowMs))
	if raw == "" {
		return defaultRecvWin
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return defaultRecvWin
	}
	if n > maxRecvWindowMs {
		return maxRecvWindowMs
	}
	return n
}

// skipAskUser reports whether BINANCE_SKIP_ASK_USER is truthy.
func skipAskUser() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(envSkipAskUser)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// apiKeys returns (key, secret) or an error if either is unset.
func apiKeys() (string, string, error) {
	k := strings.TrimSpace(os.Getenv(envAPIKey))
	s := strings.TrimSpace(os.Getenv(envAPISecret))
	if k == "" || s == "" {
		return "", "", fmt.Errorf("missing %s / %s environment variables", envAPIKey, envAPISecret)
	}
	return k, s, nil
}

// newClientOrderID generates an idempotent client order ID. Format: bmt-<hex32>.
func newClientOrderID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return clientIDPrefix + hex.EncodeToString(b[:])
}

// timestampMs returns the current time in ms adjusted by the cached server skew.
func timestampMs() int64 {
	return time.Now().UnixMilli() + defaultClient.skewMs.Load()
}

// syncTime fetches /fapi/v1/time and updates the cached skew.
func syncTime(ctx context.Context) error {
	defaultClient.mu.Lock()
	defer defaultClient.mu.Unlock()

	u := baseURL() + "/fapi/v1/time"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	localBefore := time.Now().UnixMilli()
	resp, err := defaultClient.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("time sync failed: HTTP %s: %s", resp.Status, string(body))
	}
	var payload struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("time sync parse: %w", err)
	}
	localAfter := time.Now().UnixMilli()
	localMid := (localBefore + localAfter) / 2
	defaultClient.skewMs.Store(payload.ServerTime - localMid)
	return nil
}

// binanceError represents a structured API error.
type binanceError struct {
	HTTPStatus int
	Code       int    `json:"code"`
	Msg        string `json:"msg"`
	Raw        string // raw body when not decodable
}

func (e *binanceError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("binance error %d: %s", e.Code, e.Msg)
	}
	return fmt.Sprintf("binance HTTP %d: %s", e.HTTPStatus, e.Raw)
}

// errorGuidance maps known codes to short hints appended to the error message.
var errorGuidance = map[int]string{
	-1003: "too many requests — IP rate-limited; back off before retrying",
	-1006: "unexpected response from exchange — retry",
	-1007: "exchange timeout — retry with exponential backoff",
	-1008: "system throttled — back off; reduce-only / close-position are exempt",
	-1015: "too many open orders — cancel existing orders before placing more",
	-1021: "timestamp outside recvWindow — clock skew auto-resync attempted",
	-1100: "illegal characters in a parameter — check field values for special chars",
	-1111: "precision over maximum — check stepSize / tickSize via binance_futures_symbol_specs",
	-1121: "invalid symbol — verify via binance_futures_exchange_info",
	-2010: "order rejected — filter violation or would have triggered immediately",
	-2011: "cancel rejected — order not found or already filled/cancelled",
	-2013: "order not found — check order_id or orig_client_order_id",
	-2019: "insufficient margin — reduce quantity or leverage",
	-2021: "order would immediately trigger — adjust price/stopPrice away from current mark",
	-2022: "reduce-only rejected — no matching position to reduce",
	-4028: "invalid leverage — exceeds maximum allowed for this symbol",
	-4046: "no need to change margin type — already set",
	-4058: "invalid position mode — account may already be in target mode",
	-4067: "cannot change while open orders exist — cancel via binance_futures_cancel_all_orders first",
	-4114: "client order ID too long — max 36 characters",
	-4120: "STOP_ORDER_SWITCH_ALGO — order auto-retried via /fapi/v1/algoOrder (algoType=CONDITIONAL); if you see this it means the retry also failed",
	-4131: "PERCENT_PRICE filter failed — price too far from mark",
	-4164: "notional below MIN_NOTIONAL — increase quantity (check binance_futures_symbol_specs)",
	-4400: "invalid order status for this operation",
}

// humanizeError returns a user-friendly string for an error.
func humanizeError(err error) string {
	if err == nil {
		return ""
	}
	var be *binanceError
	if errors.As(err, &be) {
		switch be.HTTPStatus {
		case 429:
			return "binance HTTP 429: rate limit exceeded — back off before retrying; " + be.Raw
		case 418:
			return "binance HTTP 418: IP auto-banned — stop all requests immediately; " + be.Raw
		case 503:
			if strings.Contains(strings.ToLower(be.Raw), "service unavailable") {
				return "binance HTTP 503: Service Unavailable — retry with exponential backoff (200ms → 400ms → 800ms)"
			}
		}
		if g, ok := errorGuidance[be.Code]; ok {
			return fmt.Sprintf("binance error %d: %s (%s)", be.Code, be.Msg, g)
		}
		if be.Code != 0 {
			return fmt.Sprintf("binance error %d: %s", be.Code, be.Msg)
		}
		return fmt.Sprintf("binance HTTP %d: %s", be.HTTPStatus, be.Raw)
	}
	return err.Error()
}

// signParams appends timestamp + recvWindow + signature to params.
func signParams(secret string, params url.Values) url.Values {
	params.Set("timestamp", strconv.FormatInt(timestampMs(), 10))
	if params.Get("recvWindow") == "" {
		params.Set("recvWindow", strconv.FormatInt(recvWindowMs(), 10))
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(params.Encode()))
	params.Set("signature", hex.EncodeToString(mac.Sum(nil)))
	return params
}

// rateLimitFooter extracts the common Binance rate-limit headers into a short footer.
func rateLimitFooter(h http.Header) string {
	keys := []string{"X-Mbx-Used-Weight-1m", "X-Mbx-Order-Count-10s", "X-Mbx-Order-Count-1m"}
	var parts []string
	for _, k := range keys {
		if v := h.Get(k); v != "" {
			parts = append(parts, strings.ToLower(strings.TrimPrefix(k, "X-Mbx-"))+"="+v)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n[rate-limit] " + strings.Join(parts, " ")
}

// requestOpts tunes a single HTTP call.
type requestOpts struct {
	method string
	path   string // e.g. /fapi/v1/order
	params url.Values
	signed bool // sign + send APIKEY header
	apiKey bool // send APIKEY header only; signed implies apiKey
}

// request executes one HTTP call. On signed requests it auto-syncs the clock
// once on -1021. Returns body, response headers, or an error.
func request(ctx context.Context, o requestOpts) ([]byte, http.Header, error) {
	return requestWithRetry(ctx, o, true)
}

func requestWithRetry(ctx context.Context, o requestOpts, allowResync bool) ([]byte, http.Header, error) {
	if o.params == nil {
		o.params = url.Values{}
	}

	var apiKey, secret string
	if o.signed || o.apiKey {
		k, s, err := apiKeys()
		if err != nil {
			return nil, nil, err
		}
		apiKey = k
		secret = s
	}

	params := cloneValues(o.params)
	if o.signed {
		params = signParams(secret, params)
	}

	method := strings.ToUpper(o.method)
	u := baseURL() + o.path

	// Per Binance best-practice: GET uses query string; POST/PUT/DELETE send params
	// as an application/x-www-form-urlencoded request body so credentials and
	// signatures are not exposed in server access logs or URL length limits.
	var reqBody io.Reader
	if method == http.MethodGet {
		if len(params) > 0 {
			u += "?" + params.Encode()
		}
	} else {
		if len(params) > 0 {
			reqBody = strings.NewReader(params.Encode())
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, nil, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if o.signed || o.apiKey {
		req.Header.Set("X-MBX-APIKEY", apiKey)
	}

	resp, err := defaultClient.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, resp.Header, err
	}

	// 429 and 418 are rate-limit / ban responses — surface them distinctly.
	if resp.StatusCode == 429 || resp.StatusCode == 418 {
		retryAfter := resp.Header.Get("Retry-After")
		be := &binanceError{HTTPStatus: resp.StatusCode, Raw: retryAfter}
		return body, resp.Header, be
	}

	if resp.StatusCode >= 400 {
		be := &binanceError{HTTPStatus: resp.StatusCode, Raw: string(body)}
		_ = json.Unmarshal(body, be)
		if allowResync && o.signed && be.Code == -1021 {
			if err := syncTime(ctx); err == nil {
				return requestWithRetry(ctx, o, false)
			}
		}
		return body, resp.Header, be
	}
	return body, resp.Header, nil
}

func cloneValues(src url.Values) url.Values {
	dst := url.Values{}
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
	return dst
}

// prettyJSON pretty-prints raw JSON. If invalid JSON, returns as-is.
func prettyJSON(b []byte) string {
	var tmp interface{}
	if err := json.Unmarshal(b, &tmp); err != nil {
		return string(b)
	}
	out, err := json.MarshalIndent(tmp, "", "  ")
	if err != nil {
		return string(b)
	}
	return string(out)
}

// symbolInfo is the subset of exchangeInfo we care about.
type symbolInfo struct {
	Symbol            string                   `json:"symbol"`
	Pair              string                   `json:"pair"`
	ContractType      string                   `json:"contractType"`
	Status            string                   `json:"status"`
	BaseAsset         string                   `json:"baseAsset"`
	QuoteAsset        string                   `json:"quoteAsset"`
	PricePrecision    int                      `json:"pricePrecision"`
	QuantityPrecision int                      `json:"quantityPrecision"`
	OrderTypes        []string                 `json:"orderTypes"`
	TimeInForce       []string                 `json:"timeInForce"`
	Filters           []map[string]interface{} `json:"filters"`
}

func fetchSymbolInfo(ctx context.Context, symbol string) (*symbolInfo, error) {
	body, _, err := request(ctx, requestOpts{
		method: http.MethodGet,
		path:   "/fapi/v1/exchangeInfo",
		params: url.Values{"symbol": []string{strings.ToUpper(symbol)}},
	})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Symbols []symbolInfo `json:"symbols"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	for i := range payload.Symbols {
		if strings.EqualFold(payload.Symbols[i].Symbol, symbol) {
			return &payload.Symbols[i], nil
		}
	}
	return nil, fmt.Errorf("symbol %s not found", symbol)
}

func reduceSymbolFilters(s *symbolInfo) map[string]interface{} {
	out := map[string]interface{}{}
	for _, f := range s.Filters {
		t, _ := f["filterType"].(string)
		switch t {
		case "PRICE_FILTER":
			out["tickSize"] = f["tickSize"]
			out["minPrice"] = f["minPrice"]
			out["maxPrice"] = f["maxPrice"]
		case "LOT_SIZE":
			out["stepSize"] = f["stepSize"]
			out["minQty"] = f["minQty"]
			out["maxQty"] = f["maxQty"]
		case "MIN_NOTIONAL":
			out["minNotional"] = f["notional"]
		case "MARKET_LOT_SIZE":
			out["marketStepSize"] = f["stepSize"]
			out["marketMinQty"] = f["minQty"]
		case "MAX_NUM_ORDERS":
			out["maxNumOrders"] = f["limit"]
		case "PERCENT_PRICE":
			out["percentPriceMultiplierUp"] = f["multiplierUp"]
			out["percentPriceMultiplierDown"] = f["multiplierDown"]
		}
	}
	return out
}

func sortedKeys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// fetchMarkPriceF returns the current mark price for a symbol as a float64.
// Used by checkMinNotional for MARKET-type order sizing.
func fetchMarkPriceF(ctx context.Context, symbol string) (float64, error) {
	body, _, err := request(ctx, requestOpts{
		method: http.MethodGet,
		path:   "/fapi/v1/premiumIndex",
		params: url.Values{"symbol": []string{strings.ToUpper(symbol)}},
	})
	if err != nil {
		return 0, err
	}
	var payload struct {
		MarkPrice string `json:"markPrice"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(payload.MarkPrice, 64)
}

// checkMinNotional validates that an order's notional value meets the symbol's
// MIN_NOTIONAL requirement. Returns a descriptive error when it fails.
// On transient network errors, silently returns nil (best-effort check).
// Skips the check for reduce-only orders and trailing-stop-market orders.
func checkMinNotional(ctx context.Context, symbol string, qty float64, orderType, price string, isReduceOnly bool) error {
	if isReduceOnly || orderType == "TRAILING_STOP_MARKET" || qty <= 0 {
		return nil
	}
	info, err := fetchSymbolInfo(ctx, symbol)
	if err != nil {
		return nil // transient failure: skip check
	}
	filters := reduceSymbolFilters(info)
	minNotionalStr, _ := filters["minNotional"].(string)
	if minNotionalStr == "" {
		return nil
	}
	minNotional, err := strconv.ParseFloat(minNotionalStr, 64)
	if err != nil || minNotional <= 0 {
		return nil
	}

	var notional float64
	switch orderType {
	case "MARKET", "STOP_MARKET", "TAKE_PROFIT_MARKET":
		markPrice, err := fetchMarkPriceF(ctx, symbol)
		if err != nil {
			return nil // transient failure: skip check
		}
		notional = qty * markPrice
	default: // LIMIT, STOP, TAKE_PROFIT
		priceF, err := strconv.ParseFloat(price, 64)
		if err != nil || priceF <= 0 {
			return nil // can't compute: skip
		}
		notional = qty * priceF
	}

	if notional < minNotional {
		return fmt.Errorf("order notional %.4f USDT is below symbol minimum %.4f USDT — increase quantity or use a higher price (Binance error -4164)", notional, minNotional)
	}
	return nil
}
