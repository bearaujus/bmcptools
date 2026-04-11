package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
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

	followRedirects := req.GetBool("follow_redirects", true)
	includeRespHeaders := req.GetBool("include_response_headers", false)

	bodyStr := req.GetString("body", "")
	var bodyReader io.Reader
	if bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid request: %v", err)), nil
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

	client := &http.Client{
		Timeout: time.Duration(timeoutSec * float64(time.Second)),
	}
	if !followRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading response body: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Status:  %s\n", resp.Status)
	fmt.Fprintf(&sb, "Elapsed: %s\n", elapsed.Round(time.Millisecond))
	fmt.Fprintf(&sb, "Size:    %d bytes\n", len(respBody))

	if includeRespHeaders {
		sb.WriteString("\n\u2500\u2500 Response Headers \u2500\u2500\n")
		keys := make([]string, 0, len(resp.Header))
		for k := range resp.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&sb, "  %s: %s\n", k, strings.Join(resp.Header[k], ", "))
		}
	}

	if len(respBody) > 0 {
		sb.WriteString("\n\u2500\u2500 Body \u2500\u2500\n")
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "json") || json.Valid(respBody) {
			var pretty bytes.Buffer
			if jsonErr := json.Indent(&pretty, respBody, "", "  "); jsonErr == nil {
				sb.WriteString(pretty.String())
			} else {
				sb.Write(respBody)
			}
		} else {
			sb.Write(respBody)
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
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

	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return mcp.NewToolResultError(fmt.Sprintf("file already exists: %s (use overwrite=true to replace)", path)), nil
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

	if rawHeaders, ok := req.GetArguments()["headers"]; ok && rawHeaders != nil {
		if hmap, ok := rawHeaders.(map[string]any); ok {
			for k, v := range hmap {
				httpReq.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	client := &http.Client{
		Timeout: time.Duration(timeoutSec * float64(time.Second)),
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcp.NewToolResultError(fmt.Sprintf("download failed: HTTP %s", resp.Status)), nil
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create parent dirs: %v", err)), nil
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create file: %v", err)), nil
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		os.Remove(path)
		return mcp.NewToolResultError(fmt.Sprintf("failed to write file: %v", err)), nil
	}
	elapsed := time.Since(start)

	return mcp.NewToolResultText(fmt.Sprintf("Downloaded %s to %s\nSize: %s\nElapsed: %s",
		rawURL, path, helper.HumanizeBytes(written), elapsed.Round(time.Millisecond))), nil
}
