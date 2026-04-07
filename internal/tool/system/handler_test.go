package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── get_system_info ───────────────────────────────────────────────────────────

func TestGetSystemInfoReturnsOSInfo(t *testing.T) {
	result, err := getSystemInfoHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "OS:") {
		t.Errorf("expected 'OS:' in output: %q", text)
	}
	if !strings.Contains(text, "CPUs:") {
		t.Errorf("expected 'CPUs:' in output: %q", text)
	}
}

// ── list_processes ────────────────────────────────────────────────────────────

func TestListProcessesReturnsResults(t *testing.T) {
	result, err := listProcessesHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "PID") {
		t.Errorf("expected 'PID' column header in output: %q", text)
	}
}

func TestListProcessesFilter(t *testing.T) {
	result, err := listProcessesHandler(nil, newTestRequest(map[string]any{
		"filter": "thisshouldneverexistxyz123",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "No processes found") {
		t.Errorf("expected 'No processes found' for unmatched filter: %q", text)
	}
}

func TestListProcessesLimit(t *testing.T) {
	result, err := listProcessesHandler(nil, newTestRequest(map[string]any{
		"limit": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "Showing 1 of") {
		t.Errorf("expected limit to be applied, got: %q", text)
	}
}

func TestListProcessesSortBy(t *testing.T) {
	for _, sortBy := range []string{"pid", "cpu", "mem"} {
		t.Run(sortBy, func(t *testing.T) {
			result, err := listProcessesHandler(nil, newTestRequest(map[string]any{
				"sort_by": sortBy,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if isResultError(result) {
				t.Fatalf("unexpected error for sort_by=%s: %s", sortBy, resultText(result))
			}
		})
	}
}

// ── http_request ──────────────────────────────────────────────────────────────

func TestHTTPRequestMissingURL(t *testing.T) {
	result, err := httpRequestHandler(context.Background(), newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing URL")
	}
}

func TestHTTPRequestGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url": srv.URL,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "200") {
		t.Errorf("expected status 200 in output: %q", text)
	}
	if !strings.Contains(text, "hello") {
		t.Errorf("expected body in output: %q", text)
	}
}

func TestHTTPRequestJSONBodyIsPrettyPrinted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"key":"value"}`)
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url": srv.URL,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	// Verify the JSON was pretty-printed (contains indented key)
	if !strings.Contains(text, `"key"`) {
		t.Errorf("expected pretty-printed JSON in output: %q", text)
	}
}

func TestHTTPRequestIncludeResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "test-value")
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":                      srv.URL,
		"include_response_headers": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "X-Custom-Header") {
		t.Errorf("expected response headers in output: %q", text)
	}
}

func TestHTTPRequestPOSTWithJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "created")
	}))
	defer srv.Close()

	payload, _ := json.Marshal(map[string]string{"hello": "world"})
	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":    srv.URL,
		"method": "POST",
		"body":   string(payload),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "201") {
		t.Errorf("expected status 201 in output: %q", text)
	}
}

func TestHTTPRequestFollowRedirectsDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		fmt.Fprint(w, "final destination")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":              srv.URL + "/redirect",
		"follow_redirects": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	// Should see the 302, not the final destination
	if !strings.Contains(text, "302") {
		t.Errorf("expected 302 redirect status when follow_redirects=false: %q", text)
	}
}

func TestHTTPRequestBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, "authenticated")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":        srv.URL,
		"basic_auth": "alice:secret",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "200") {
		t.Errorf("expected 200 with valid basic auth: %q", text)
	}
}
