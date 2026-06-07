package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

const (
	defaultHTTPResponseMaxBytes = 256 * 1024
	maxHTTPResponseMaxBytes     = 10 * 1024 * 1024
	defaultHTTPBodyFilterCtx    = 120
	maxHTTPBodyFilterCtx        = 4096
	defaultHTTPBodyFilterMax    = 20
	maxHTTPBodyFilterCtxLines   = 20
)

func httpRequestHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := req.GetString("url", "")
	if strings.TrimSpace(rawURL) == "" {
		return mcp.NewToolResultError("url is required"), nil
	}

	method := strings.ToUpper(strings.TrimSpace(req.GetString("method", "GET")))
	if method == "" {
		method = "GET"
	}

	timeoutSec := req.GetFloat("timeout_seconds", 30)
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if timeoutSec > 300 {
		timeoutSec = 300
	}

	maxResponseBytes := int64(req.GetFloat("max_response_bytes", defaultHTTPResponseMaxBytes))
	if maxResponseBytes < 0 {
		maxResponseBytes = defaultHTTPResponseMaxBytes
	}
	if maxResponseBytes > maxHTTPResponseMaxBytes {
		maxResponseBytes = maxHTTPResponseMaxBytes
	}

	followRedirects := req.GetBool("follow_redirects", true)
	allowPrivate := req.GetBool("allow_private", false)
	includeRespHeaders := req.GetBool("include_response_headers", false)
	jsonFormat := normalizeHTTPJSONFormat(req.GetString("json_format", "compact"))
	bodyFilter, err := newHTTPBodyFilter(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	bodyStr := req.GetString("body", "")
	var bodyReader io.Reader
	if bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid request: %v", err)), nil
	}
	if err := validateHTTPRequestURL(ctx, httpReq.URL, allowPrivate); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if rawHeaders, ok := req.GetArguments()["headers"]; ok && rawHeaders != nil {
		if hmap, ok := rawHeaders.(map[string]any); ok {
			for k, v := range hmap {
				httpReq.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	if bodyStr != "" && httpReq.Header.Get("Content-Type") == "" {
		if json.Valid([]byte(bodyStr)) {
			httpReq.Header.Set("Content-Type", "application/json")
		} else {
			httpReq.Header.Set("Content-Type", "text/plain")
		}
	}

	if ba := req.GetString("basic_auth", ""); ba != "" {
		parts := strings.SplitN(ba, ":", 2)
		if len(parts) == 2 {
			httpReq.SetBasicAuth(parts[0], parts[1])
		}
	}

	timeout := time.Duration(timeoutSec * float64(time.Second))
	client := newGuardedHTTPClient(timeout, followRedirects, allowPrivate)

	start := time.Now()
	resp, err := client.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	respBody, truncated, err := readResponseBody(resp.Body, maxResponseBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading response body: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Status:  %s\n", resp.Status)
	fmt.Fprintf(&sb, "Elapsed: %s\n", elapsed.Round(time.Millisecond))
	if finalURL := resp.Request.URL.String(); finalURL != httpReq.URL.String() {
		fmt.Fprintf(&sb, "Final URL: %s\n", finalURL)
	}
	fmt.Fprintf(&sb, "Size:    %d bytes", len(respBody))
	if truncated {
		fmt.Fprintf(&sb, " shown (truncated at %s)", helper.HumanizeBytes(maxResponseBytes))
		if resp.ContentLength > int64(len(respBody)) {
			fmt.Fprintf(&sb, "; Content-Length: %s", helper.HumanizeBytes(resp.ContentLength))
		}
	}
	sb.WriteByte('\n')

	if includeRespHeaders {
		sb.WriteString("\n\u2500\u2500 Response Headers \u2500\u2500\n")
		keys := make([]string, 0, len(resp.Header))
		for k := range resp.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&sb, "  %s: %s\n", k, formatHTTPHeaderValue(k, resp.Header[k]))
		}
	}

	if len(respBody) > 0 {
		sb.WriteString("\n\u2500\u2500 Body \u2500\u2500\n")
		rendered, renderErr := renderHTTPBody(resp, respBody, jsonFormat)
		if renderErr != nil {
			sb.WriteString(renderErr.Error())
		} else if bodyFilter != nil {
			sb.WriteString(bodyFilter.Apply(rendered))
		} else {
			sb.WriteString(rendered)
		}
		if truncated {
			fmt.Fprintf(&sb, "\n\n[Response body truncated. Increase max_response_bytes up to %d, or use download_file for large/binary responses.]", maxHTTPResponseMaxBytes)
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func normalizeHTTPJSONFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "raw":
		return "raw"
	case "pretty":
		return "pretty"
	default:
		return "compact"
	}
}

func renderHTTPBody(resp *http.Response, body []byte, jsonFormat string) (string, error) {
	body = helper.StripBOM(body)
	if isHTTPBinaryResponse(resp, body) {
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = http.DetectContentType(firstHTTPBodyBytes(body, 512))
		}
		return "", fmt.Errorf("[BINARY RESPONSE] %s body (%s) omitted; use download_file for binary content",
			ct, helper.HumanizeBytes(int64(len(body))))
	}

	if jsonFormat != "raw" && isHTTPJSONResponse(resp, body) {
		var formatted bytes.Buffer
		var err error
		if jsonFormat == "pretty" {
			err = json.Indent(&formatted, body, "", "  ")
		} else {
			err = json.Compact(&formatted, body)
		}
		if err == nil {
			return formatted.String(), nil
		}
	}

	if !utf8.Valid(body) {
		return strings.ToValidUTF8(string(body), "\uFFFD"), nil
	}
	return string(body), nil
}

func isHTTPJSONResponse(resp *http.Response, body []byte) bool {
	return strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") || json.Valid(body)
}

func isHTTPBinaryResponse(resp *http.Response, body []byte) bool {
	sniff := firstHTTPBodyBytes(body, 512)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(sniff)
	}
	contentType = strings.ToLower(contentType)
	if helper.IsBinaryContent(sniff, contentType) {
		return true
	}
	if isHTTPTextContentType(contentType) {
		return false
	}
	return strings.TrimSpace(contentType) != ""
}

func isHTTPTextContentType(contentType string) bool {
	mediaType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json",
		"application/xml",
		"application/javascript",
		"application/x-javascript",
		"application/typescript",
		"application/x-yaml",
		"application/yaml",
		"application/toml",
		"application/x-www-form-urlencoded",
		"application/graphql":
		return true
	default:
		return strings.HasSuffix(mediaType, "+json") ||
			strings.HasSuffix(mediaType, "+xml") ||
			strings.HasSuffix(mediaType, "svg+xml")
	}
}

func firstHTTPBodyBytes(body []byte, n int) []byte {
	if len(body) < n {
		return body
	}
	return body[:n]
}

type httpBodyFilter struct {
	pattern      string
	mode         string
	re           *regexp.Regexp
	contextBytes int
	contextLines int
	maxMatches   int
}

func newHTTPBodyFilter(req mcp.CallToolRequest) (*httpBodyFilter, error) {
	pattern := req.GetString("body_filter", "")
	if pattern == "" {
		return nil, nil
	}

	mode := strings.ToLower(strings.TrimSpace(req.GetString("body_filter_mode", "matches")))
	switch mode {
	case "matches", "lines", "count":
	default:
		mode = "matches"
	}

	contextBytes := int(req.GetFloat("body_filter_context_bytes", defaultHTTPBodyFilterCtx))
	if contextBytes < 0 {
		contextBytes = defaultHTTPBodyFilterCtx
	}
	if contextBytes > maxHTTPBodyFilterCtx {
		contextBytes = maxHTTPBodyFilterCtx
	}

	contextLines := int(req.GetFloat("body_filter_context_lines", 0))
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > maxHTTPBodyFilterCtxLines {
		contextLines = maxHTTPBodyFilterCtxLines
	}

	maxMatches := int(req.GetFloat("body_filter_max_matches", defaultHTTPBodyFilterMax))
	if maxMatches < 0 {
		maxMatches = defaultHTTPBodyFilterMax
	}

	useRegex := req.GetBool("body_filter_regex", false)
	caseInsensitive := req.GetBool("body_filter_case_insensitive", false)
	filter := &httpBodyFilter{
		pattern:      pattern,
		mode:         mode,
		contextBytes: contextBytes,
		contextLines: contextLines,
		maxMatches:   maxMatches,
	}

	if useRegex || caseInsensitive {
		regexPattern := pattern
		if !useRegex {
			regexPattern = regexp.QuoteMeta(pattern)
		}
		if caseInsensitive {
			regexPattern = "(?i)" + regexPattern
		}
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid body_filter regex %q: %w", pattern, err)
		}
		filter.re = re
	}

	return filter, nil
}

func (f *httpBodyFilter) Apply(body string) string {
	switch f.mode {
	case "lines":
		return f.applyLines(body)
	case "count":
		return f.applyCount(body)
	default:
		return f.applyMatches(body)
	}
}

func (f *httpBodyFilter) applyMatches(body string) string {
	limit := f.maxMatches
	findLimit := limit
	if limit > 0 {
		findLimit = limit + 1
	} else if limit == 0 {
		findLimit = -1
	}
	locs := f.findAll(body, findLimit)
	limited := limit > 0 && len(locs) > limit
	if limited {
		locs = locs[:limit]
	}
	if len(locs) == 0 {
		return f.noMatchMessage(body)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Body filter matched %s for %q (mode=matches, context=%d bytes):\n\n",
		helper.Pluralize(len(locs), "occurrence"), f.pattern, f.contextBytes)
	for i, loc := range locs {
		snippet := f.snippet(body, loc[0], loc[1])
		fmt.Fprintf(&sb, "%d. bytes %d..%d: %s\n", i+1, loc[0], loc[1], snippet)
	}
	if limited {
		fmt.Fprintf(&sb, "\n[Result limit of %d reached. Increase body_filter_max_matches or use body_filter_mode=\"count\".]", limit)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (f *httpBodyFilter) applyLines(body string) string {
	lines := strings.Split(body, "\n")
	type lineHit struct {
		index int
		line  string
	}
	var hits []lineHit
	limited := false
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if !f.matches(line) {
			continue
		}
		if f.maxMatches > 0 && len(hits) >= f.maxMatches {
			limited = true
			break
		}
		hits = append(hits, lineHit{i, line})
	}
	if len(hits) == 0 {
		return f.noMatchMessage(body)
	}

	show := make(map[int]bool)
	for _, hit := range hits {
		start := max(0, hit.index-f.contextLines)
		end := min(len(lines)-1, hit.index+f.contextLines)
		for i := start; i <= end; i++ {
			show[i] = true
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Body filter matched %s for %q (mode=lines, context=%d lines):\n\n",
		helper.Pluralize(len(hits), "line"), f.pattern, f.contextLines)
	for i, raw := range lines {
		if !show[i] {
			continue
		}
		line := strings.TrimSuffix(raw, "\r")
		prefix := "-"
		if f.matches(line) {
			prefix = ":"
		}
		fmt.Fprintf(&sb, "%d%s %s\n", i+1, prefix, f.trimLine(line))
	}
	if limited {
		fmt.Fprintf(&sb, "\n[Result limit of %d matched lines reached. Increase body_filter_max_matches or use body_filter_mode=\"count\".]", f.maxMatches)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (f *httpBodyFilter) applyCount(body string) string {
	count := f.countAll(body)
	return fmt.Sprintf("Body filter found %s for %q in %s of rendered response body.",
		helper.Pluralize(count, "match"), f.pattern, helper.HumanizeBytes(int64(len(body))))
}

func (f *httpBodyFilter) noMatchMessage(body string) string {
	msg := fmt.Sprintf("Body filter found no matches for %q in %s of rendered response body.",
		f.pattern, helper.HumanizeBytes(int64(len(body))))
	if f.re == nil && containsHTTPRegexMetachars(f.pattern) {
		msg += "\nHint: body_filter is literal by default; set body_filter_regex=true to enable Go regex syntax."
	}
	return msg
}

func (f *httpBodyFilter) findAll(body string, n int) [][2]int {
	if f.re != nil {
		raw := f.re.FindAllStringIndex(body, n)
		locs := make([][2]int, 0, len(raw))
		for _, loc := range raw {
			locs = append(locs, [2]int{loc[0], loc[1]})
		}
		return locs
	}

	var locs [][2]int
	offset := 0
	for {
		if n >= 0 && len(locs) >= n {
			return locs
		}
		idx := strings.Index(body[offset:], f.pattern)
		if idx < 0 {
			return locs
		}
		start := offset + idx
		end := start + len(f.pattern)
		locs = append(locs, [2]int{start, end})
		offset = end
		if offset > len(body) {
			return locs
		}
	}
}

func (f *httpBodyFilter) countAll(body string) int {
	if f.re != nil {
		count := 0
		offset := 0
		for offset <= len(body) {
			loc := f.re.FindStringIndex(body[offset:])
			if loc == nil {
				return count
			}
			count++
			end := offset + loc[1]
			if loc[0] == loc[1] {
				if end >= len(body) {
					return count
				}
				_, size := utf8.DecodeRuneInString(body[end:])
				if size <= 0 {
					size = 1
				}
				offset = end + size
				continue
			}
			offset = end
		}
		return count
	}

	count := 0
	offset := 0
	for {
		idx := strings.Index(body[offset:], f.pattern)
		if idx < 0 {
			return count
		}
		count++
		offset += idx + len(f.pattern)
		if offset > len(body) {
			return count
		}
	}
}

func (f *httpBodyFilter) matches(s string) bool {
	if f.re != nil {
		return f.re.MatchString(s)
	}
	return strings.Contains(s, f.pattern)
}

func (f *httpBodyFilter) firstMatch(s string) (int, int, bool) {
	if f.re != nil {
		loc := f.re.FindStringIndex(s)
		if loc == nil {
			return 0, 0, false
		}
		return loc[0], loc[1], true
	}
	idx := strings.Index(s, f.pattern)
	if idx < 0 {
		return 0, 0, false
	}
	return idx, idx + len(f.pattern), true
}

func (f *httpBodyFilter) snippet(s string, start, end int) string {
	left := clampHTTPRuneBoundary(s, max(0, start-f.contextBytes), -1)
	right := clampHTTPRuneBoundary(s, min(len(s), end+f.contextBytes), 1)
	snippet := s[left:right]
	snippet = strings.ReplaceAll(snippet, "\r", "\\r")
	snippet = strings.ReplaceAll(snippet, "\n", "\\n")
	if left > 0 {
		snippet = "..." + snippet
	}
	if right < len(s) {
		snippet += "..."
	}
	return snippet
}

func (f *httpBodyFilter) trimLine(line string) string {
	const maxLineBytes = 2000
	if len(line) <= maxLineBytes {
		return line
	}
	start, end, ok := f.firstMatch(line)
	if !ok {
		right := clampHTTPRuneBoundary(line, maxLineBytes, -1)
		return line[:right] + "... [line truncated]"
	}
	return f.snippet(line, start, end) + " [line truncated]"
}

func clampHTTPRuneBoundary(s string, idx, direction int) int {
	if idx <= 0 {
		return 0
	}
	if idx >= len(s) {
		return len(s)
	}
	if direction < 0 {
		for idx > 0 && !utf8.RuneStart(s[idx]) {
			idx--
		}
		return idx
	}
	for idx < len(s) && !utf8.RuneStart(s[idx]) {
		idx++
	}
	return idx
}

func containsHTTPRegexMetachars(s string) bool {
	return strings.ContainsAny(s, `|.+*?^$()[]{}\`)
}

func readResponseBody(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes == 0 {
		body, err := io.ReadAll(r)
		return body, false, err
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > maxBytes {
		return body[:maxBytes], true, nil
	}
	return body, false, nil
}

func downloadFileHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rawURL := req.GetString("url", "")
	if strings.TrimSpace(rawURL) == "" {
		return mcp.NewToolResultError("url is required"), nil
	}
	path := req.GetString("path", "")
	if strings.TrimSpace(path) == "" {
		return mcp.NewToolResultError("path is required"), nil
	}
	overwrite := req.GetBool("overwrite", false)
	allowPrivate := req.GetBool("allow_private", false)
	allowOutsideCWD := req.GetBool("allow_outside_cwd", false)
	absPath, err := resolveDownloadDestination(path, allowOutsideCWD)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if !overwrite {
		if _, err := os.Stat(absPath); err == nil {
			return mcp.NewToolResultError(fmt.Sprintf("file already exists: %s (use overwrite=true to replace)", absPath)), nil
		}
	}

	timeoutSec := req.GetFloat("timeout_seconds", 300)
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	if timeoutSec > 600 {
		timeoutSec = 600
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid URL: %v", err)), nil
	}
	if err := validateHTTPRequestURL(ctx, httpReq.URL, allowPrivate); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if rawHeaders, ok := req.GetArguments()["headers"]; ok && rawHeaders != nil {
		if hmap, ok := rawHeaders.(map[string]any); ok {
			for k, v := range hmap {
				httpReq.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	timeout := time.Duration(timeoutSec * float64(time.Second))
	client := newGuardedHTTPClient(timeout, true, allowPrivate)

	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: HTTP %s", resp.Status)), nil
	}

	if dir := filepath.Dir(absPath); dir != "" && dir != "." {
		if err := helper.MkdirAllClear(dir, 0o755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create parent dirs: %v", err)), nil
		}
	}

	f, err := os.Create(absPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create file: %v", err)), nil
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		os.Remove(absPath)
		return mcp.NewToolResultError(fmt.Sprintf("failed to write file: %v", err)), nil
	}
	elapsed := time.Since(start)

	return mcp.NewToolResultText(fmt.Sprintf("Downloaded %s to %s\nSize: %s\nElapsed: %s",
		rawURL, absPath, helper.HumanizeBytes(written), elapsed.Round(time.Millisecond))), nil
}

func newGuardedHTTPClient(timeout time.Duration, followRedirects, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host := addr
		if splitHost, _, err := net.SplitHostPort(addr); err == nil {
			host = splitHost
		}
		if err := validateResolvedHTTPHost(ctx, host, allowPrivate); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, addr)
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !followRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if err := validateHTTPRequestURL(context.Background(), req.URL, allowPrivate); err != nil {
			return err
		}
		if len(via) > 0 && !sameHTTPHost(req.URL, via[len(via)-1].URL) {
			stripSensitiveRedirectHeaders(req.Header)
		}
		return nil
	}
	return client
}

func validateHTTPRequestURL(ctx context.Context, target *url.URL, allowPrivate bool) error {
	if target == nil {
		return fmt.Errorf("url is required")
	}
	scheme := strings.ToLower(strings.TrimSpace(target.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q; use http or https", target.Scheme)
	}
	host := strings.TrimSpace(target.Hostname())
	if host == "" {
		return fmt.Errorf("URL host is required")
	}
	if err := validateResolvedHTTPHost(ctx, host, allowPrivate); err != nil {
		return err
	}
	return nil
}

func validateResolvedHTTPHost(ctx context.Context, host string, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("cannot resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("cannot resolve %q", host)
	}
	for _, addr := range ips {
		if isBlockedHTTPIP(addr.IP) {
			return fmt.Errorf("refusing private or loopback network target %q (%s); set allow_private=true to override", host, addr.IP.String())
		}
	}
	return nil
}

func isBlockedHTTPIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		if ipv4[0] == 100 && ipv4[1] >= 64 && ipv4[1] <= 127 {
			return true
		}
		if ipv4[0] == 198 && (ipv4[1] == 18 || ipv4[1] == 19) {
			return true
		}
	}
	return false
}

func sameHTTPHost(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Hostname(), b.Hostname()) && normalizedHTTPPort(a) == normalizedHTTPPort(b)
}

func normalizedHTTPPort(u *url.URL) string {
	if u == nil {
		return ""
	}
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

func stripSensitiveRedirectHeaders(headers http.Header) {
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
		headers.Del(name)
	}
}

func formatHTTPHeaderValue(name string, values []string) string {
	if isSensitiveHTTPHeader(name) {
		return "[redacted]"
	}
	return strings.Join(values, ", ")
}

func isSensitiveHTTPHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Authorization", "Cookie", "Proxy-Authorization", "Set-Cookie":
		return true
	default:
		return false
	}
}

func resolveDownloadDestination(path string, allowOutsideCWD bool) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve download path %q: %w", path, err)
	}
	if allowOutsideCWD {
		return absPath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot get working directory: %w", err)
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return "", fmt.Errorf("refusing download path outside current working directory: %s; set allow_outside_cwd=true to override", absPath)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing download path outside current working directory: %s; set allow_outside_cwd=true to override", absPath)
	}
	return absPath, nil
}
