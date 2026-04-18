// Package binance provides MCP tools for interacting with the Binance USDT-M Futures REST API.
//
// Market-data tools require no credentials. Account and trading tools require
// BINANCE_API_KEY and BINANCE_API_SECRET environment variables. Mutating calls
// (change leverage/margin/mode, place/cancel orders) prompt the user for
// confirmation via pkg/confirm unless BINANCE_SKIP_ASK_USER=true.
//
// Base URL defaults to https://fapi.binance.com; set BINANCE_FUTURES_BASE_URL
// to switch to testnet (https://testnet.binancefuture.com) or another endpoint.
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
	envBaseURL      = "BINANCE_FUTURES_BASE_URL"
	envSkipAskUser  = "BINANCE_SKIP_ASK_USER"
	envRecvWindowMs = "BINANCE_RECV_WINDOW_MS"
)

const (
	defaultBaseURL  = "https://fapi.binance.com"
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

func baseURL() string {
	if v := strings.TrimRight(os.Getenv(envBaseURL), "/"); v != "" {
		return v
	}
	return defaultBaseURL
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
	-1008: "system throttled — back off; reduce-only / close-position are exempt",
	-1021: "timestamp outside recvWindow — clock skew auto-resync attempted",
	-1111: "precision over maximum — check stepSize / tickSize via binance_futures_symbol_specs",
	-1121: "invalid symbol — verify via binance_futures_exchange_info",
	-2010: "order rejected — filter violation or would have triggered immediately",
	-2019: "insufficient margin — reduce quantity or leverage",
	-2022: "reduce-only rejected — no matching position to reduce",
	-4046: "no need to change margin type — already set",
	-4067: "cannot change while open orders exist — cancel via binance_futures_cancel_all_open_orders first",
	-4120: "order type not supported by this endpoint. Note: Binance Futures TESTNET does not support STOP_MARKET / TAKE_PROFIT_MARKET / TRAILING_STOP_MARKET via /fapi/v1/order or /fapi/v1/batchOrders — those are only available on mainnet (or via the Algo Order API). On testnet, simulate SL/TP with conditional STOP / TAKE_PROFIT limit orders instead, or validate bracket orders on mainnet with minimum size.",
	-4131: "PERCENT_PRICE filter failed — price too far from mark",
	-4164: "notional below MIN_NOTIONAL — increase quantity (check binance_futures_symbol_specs)",
}

// humanizeError returns a user-friendly string for an error.
func humanizeError(err error) string {
	if err == nil {
		return ""
	}
	var be *binanceError
	if errors.As(err, &be) {
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

	u := baseURL() + o.path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	method := strings.ToUpper(o.method)
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, nil, err
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
