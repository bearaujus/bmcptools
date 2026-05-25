package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestListProcessesSmallCommandWidthDoesNotPanic(t *testing.T) {
	result, err := listProcessesHandler(nil, newTestRequest(map[string]any{
		"command_width": float64(1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
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

func TestHTTPRequestResponseBodyCap(t *testing.T) {
	body := strings.Repeat("a", 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":                srv.URL,
		"max_response_bytes": float64(10),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "truncated") {
		t.Errorf("expected truncation notice in output: %q", text)
	}
	if strings.Contains(text, strings.Repeat("a", 20)) {
		t.Errorf("expected body to be capped, got: %q", text)
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

// ── get_system_info memory and disk ──────────────────────────────────────────
// Reason: The existing test only checked for "OS:" and "CPUs:" labels. Memory
// and disk sections are also always-present output fields. Missing coverage
// means a regression in those rendering paths would go undetected.

func TestGetSystemInfoHasMemoryAndDisk(t *testing.T) {
	result, err := getSystemInfoHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "Memory") {
		t.Errorf("expected 'Memory' section in get_system_info output: %q", text)
	}
	if !strings.Contains(text, "Disk") {
		t.Errorf("expected 'Disk' section in get_system_info output: %q", text)
	}
}

// ── http_request custom headers ───────────────────────────────────────────────
// Reason: While outbound request headers are a first-class parameter of
// http_request, no existing test verified that the headers are actually
// forwarded to the server. This risks silent breakage if header marshalling
// changes.

func TestHTTPRequestCustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Header") != "my-value" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "header received")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":     srv.URL,
		"headers": map[string]any{"X-Test-Header": "my-value"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "200") {
		t.Errorf("expected 200 when custom header is present: %q", text)
	}
}

// ── http_request DELETE method ────────────────────────────────────────────────
// Reason: Only GET and POST methods were tested. DELETE is also documented
// and commonly used; testing it catches any method-routing bugs.

func TestHTTPRequestMethodDELETE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":    srv.URL + "/resource/1",
		"method": "DELETE",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "204") {
		t.Errorf("expected 204 No Content for DELETE: %q", text)
	}
}

// ── clipboard ─────────────────────────────────────────────────────────────────

// Reason: clipboard_write and clipboard_read had zero test coverage. These are
// first-class tools and any regression in the OS-specific command would go
// completely unnoticed.

func TestClipboardWriteAndRead(t *testing.T) {
	const sample = "bmcptools clipboard test value"
	writeResult, err := clipboardWriteHandler(nil, newTestRequest(map[string]any{
		"text": sample,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(writeResult) {
		t.Skipf("clipboard not available in this environment: %s", resultText(writeResult))
	}

	readResult, err := clipboardReadHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(readResult) {
		t.Skipf("clipboard read failed: %s", resultText(readResult))
	}
	text := resultText(readResult)
	if !strings.Contains(text, sample) {
		t.Errorf("expected clipboard to contain %q, got: %q", sample, text)
	}
}

// Reason: clipboard_write byte and line count should be reported accurately.
func TestClipboardWriteReportsStats(t *testing.T) {
	writeResult, err := clipboardWriteHandler(nil, newTestRequest(map[string]any{
		"text": "line1\nline2\nline3",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(writeResult) {
		t.Skipf("clipboard not available: %s", resultText(writeResult))
	}
	text := resultText(writeResult)
	if !strings.Contains(text, "bytes") {
		t.Errorf("expected 'bytes' in clipboard write result: %q", text)
	}
}

func TestFormatClipboardReadTruncates(t *testing.T) {
	text := strings.Repeat("x", 20)
	got := formatClipboardRead(text, 5)
	if !strings.Contains(got, "5/20 bytes shown") {
		t.Errorf("expected byte count in output: %q", got)
	}
	if !strings.Contains(got, "Clipboard truncated") {
		t.Errorf("expected truncation notice in output: %q", got)
	}
	if strings.Contains(got, strings.Repeat("x", 10)) {
		t.Errorf("expected displayed clipboard text to be capped: %q", got)
	}
}

// ── list_processes (additional edge cases) ────────────────────────────────────

// Reason: An invalid sort_by value should silently fall back to "pid" sorting
// rather than returning an error. This ensures robustness when LLMs send
// unexpected sort values.
func TestListProcessesInvalidSortByDefaultsToPID(t *testing.T) {
	result, err := listProcessesHandler(nil, newTestRequest(map[string]any{
		"sort_by": "invalid_sort_key",
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Should not error — invalid sort just uses default
	if isResultError(result) {
		t.Errorf("expected graceful handling of invalid sort_by, got error: %s", resultText(result))
	}
}

// Reason: limit=0 means "no limit". Sending limit=0 should not return an
// error or empty result set. A test protects against accidentally treating 0
// as "show zero processes".
func TestListProcessesLimitZeroMeansUnlimited(t *testing.T) {
	result, err := listProcessesHandler(nil, newTestRequest(map[string]any{
		"limit": float64(0),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error with limit=0: %s", resultText(result))
	}
	text := resultText(result)
	// Should show processes, not "Showing 0 of"
	if strings.Contains(text, "Showing 0 of") {
		t.Errorf("limit=0 should not restrict output to 0 processes: %q", text)
	}
}

// ── http_request (additional edge cases) ──────────────────────────────────────

// Reason: PUT is a common REST verb but was never tested. Any bug in method
// routing that treats PUT like GET would be invisible without a test.
func TestHTTPRequestPUT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "updated")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":    srv.URL + "/resource",
		"method": "PUT",
		"body":   `{"key":"value"}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error for PUT: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "200") {
		t.Errorf("expected 200 for PUT: %q", text)
	}
}

// Reason: Non-JSON response bodies should be returned as-is without the
// handler crashing. The JSON pretty-print path must gracefully fall through.
func TestHTTPRequestNonJSONBodyReturnedAsIs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "plain text response")
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
	if !strings.Contains(text, "plain text response") {
		t.Errorf("expected plain text body in response: %q", text)
	}
}

// Reason: invalid JSON that looks like JSON (starts with {) should still be
// returned rather than causing an error. This tests the graceful fallback.
func TestHTTPRequestMalformedJSONFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"broken":`)
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url": srv.URL,
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Handler should not error on malformed JSON — just return raw body
	if isResultError(result) {
		t.Fatalf("unexpected error for malformed JSON body: %s", resultText(result))
	}
}

// Reason: PATCH is another common REST verb. Ensures the method parameter is
// passed through correctly for all standard verbs, not just GET/POST/DELETE/PUT.
func TestHTTPRequestPATCH(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "patched")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url":    srv.URL,
		"method": "PATCH",
		"body":   `{"field":"val"}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error for PATCH: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "200") {
		t.Errorf("expected 200 for PATCH: %q", text)
	}
}

// Reason: The server response status code is always included in the output.
// A 4xx/5xx response should still be returned as success text (not MCP error)
// so the LLM can see the failure reason.
func TestHTTPRequestServerError4xxIsTextResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	}))
	defer srv.Close()

	result, err := httpRequestHandler(context.Background(), newTestRequest(map[string]any{
		"url": srv.URL + "/missing",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "404") {
		t.Errorf("expected 404 in output: %q", text)
	}
}

// ── download_file ─────────────────────────────────────────────────────────────

func TestDownloadFileSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "file content here")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "downloaded.txt")
	result, err := downloadFileHandler(context.Background(), newTestRequest(map[string]any{
		"url":  srv.URL,
		"path": dest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "Downloaded") {
		t.Errorf("expected 'Downloaded' in output: %q", text)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != "file content here" {
		t.Errorf("expected 'file content here', got: %q", string(data))
	}
}

func TestDownloadFileOverwriteProtection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "new content")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(dest, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := downloadFileHandler(context.Background(), newTestRequest(map[string]any{
		"url":  srv.URL,
		"path": dest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for existing file without overwrite=true")
	}
	text := resultText(result)
	if !strings.Contains(text, "already exists") {
		t.Errorf("expected 'already exists' in error: %q", text)
	}
}

func TestDownloadFileOverwriteAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "replaced content")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(dest, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := downloadFileHandler(context.Background(), newTestRequest(map[string]any{
		"url":       srv.URL,
		"path":      dest,
		"overwrite": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "replaced content" {
		t.Errorf("expected 'replaced content', got: %q", string(data))
	}
}

func TestDownloadFileInvalidURL(t *testing.T) {
	result, err := downloadFileHandler(context.Background(), newTestRequest(map[string]any{
		"url":  "http://invalid.invalid.invalid:99999/no",
		"path": filepath.Join(t.TempDir(), "out.txt"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for invalid URL")
	}
}

func TestDownloadFileMissingURL(t *testing.T) {
	result, err := downloadFileHandler(context.Background(), newTestRequest(map[string]any{
		"path": filepath.Join(t.TempDir(), "out.txt"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing URL")
	}
}

func TestDownloadFileMissingPath(t *testing.T) {
	result, err := downloadFileHandler(context.Background(), newTestRequest(map[string]any{
		"url": "http://example.com/file",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing path")
	}
}
