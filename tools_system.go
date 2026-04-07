package main

// AI Developer Feedback — written after implementing clipboard_write, http_request,
// list_processes, and get_system_info as an AI using this MCP SDK.
//
// ── What felt great ─────────────────────────────────────────────────────────
//
//  1. Consistency: every tool follows the same pattern:
//       registerXTools(s) → s.AddTool(mcp.NewTool(...), handler)
//       → func handler(ctx, req) (*result, error)
//     After reading one existing file I could implement new tools without
//     referring back to docs — the pattern is self-documenting.
//
//  2. Parameter helpers: req.GetString / GetBool / GetFloat are clean and
//     default-value-aware. No nil-checks, no type assertions for scalars.
//
//  3. Result types: mcp.NewToolResultText / NewToolResultError are simple and
//     unambiguous. Errors-as-values (not Go errors) keeps flow linear.
//
//  4. Description DSL: mcp.WithString("param", mcp.Required(), mcp.Description("..."))
//     reads like natural language and doubles as schema + inline docs.
//
// ── Pain points & suggestions ────────────────────────────────────────────────
//
//  1. No typed getter for object/map params.
//     Getting the "headers" map required reaching into req.GetArguments() directly
//     and doing a manual type-assert:
//       if hmap, ok := rawHeaders.(map[string]any); ok { ... }
//     A helper like req.GetMap("headers") → map[string]any would be cleaner
//     and consistent with the scalar getters.
//
//  2. No streaming / incremental results.
//     All output is buffered and returned at once. For long-running commands
//     (e.g. a slow HTTP download or a large ps output) there's no way to stream
//     partial results to the LLM. A streaming variant would improve latency.
//
//  3. No structured (JSON/table) result type — only text.
//     For tools like list_processes, returning a JSON array would let the LLM
//     reason over the data (e.g. "sum all CPU%") rather than parsing a text table.
//     Even a mcp.NewToolResultJSON(v any) helper would help.
//
//  4. mcp.WithObject schema is not validated.
//     The "headers" parameter accepts any object — there's no way to declare
//     that values must be strings. Optional JSON Schema support would catch
//     caller mistakes early.
//
//  5. Version constant lives in main.go, not exported.
//     When writing this file I wanted to reference the server version for
//     diagnostic output but had to hard-code it. An exported Version const
//     (or moving it to a shared file) would be a small DX improvement.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSystemTools(s *server.MCPServer) {
	// ── clipboard_write ───────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("clipboard_write",
		mcp.WithDescription(td("clipboard_write")),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description(pd("clipboard_write", "text")),
		),
	), clipboardWriteHandler)

	// ── clipboard_read ────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("clipboard_read",
		mcp.WithDescription(td("clipboard_read")),
	), clipboardReadHandler)

	// ── http_request ──────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("http_request",
		mcp.WithDescription(td("http_request")),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description(pd("http_request", "url")),
		),
		mcp.WithString("method",
			mcp.Description(pd("http_request", "method")),
		),
		mcp.WithObject("headers",
			mcp.Description(pd("http_request", "headers")),
		),
		mcp.WithString("body",
			mcp.Description(pd("http_request", "body")),
		),
		mcp.WithString("basic_auth",
			mcp.Description(pd("http_request", "basic_auth")),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description(pd("http_request", "timeout_seconds")),
		),
		mcp.WithBoolean("follow_redirects",
			mcp.Description(pd("http_request", "follow_redirects")),
		),
		mcp.WithBoolean("include_response_headers",
			mcp.Description(pd("http_request", "include_response_headers")),
		),
	), httpRequestHandler)

	// ── list_processes ────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("list_processes",
		mcp.WithDescription(td("list_processes")),
		mcp.WithString("filter",
			mcp.Description(pd("list_processes", "filter")),
		),
		mcp.WithString("sort_by",
			mcp.Description(pd("list_processes", "sort_by")),
		),
		mcp.WithNumber("limit",
			mcp.Description(pd("list_processes", "limit")),
		),
	), listProcessesHandler)

	s.AddTool(mcp.NewTool("get_system_info",
		mcp.WithDescription(td("get_system_info")),
	), getSystemInfoHandler)
}

// ─── clipboard_write ──────────────────────────────────────────────────────────

func clipboardWriteHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text := req.GetString("text", "")
	if err := writeClipboard(text); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("clipboard write failed: %v", err)), nil
	}
	lines := strings.Count(text, "\n") + 1
	return mcp.NewToolResultText(fmt.Sprintf("Copied %d bytes (%d line(s)) to clipboard.", len(text), lines)), nil
}

func writeClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-clipboard)")
		}
	case "windows":
		cmd = exec.Command("powershell", "-Command", "Set-Clipboard -Value $input")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// ─── clipboard_read ───────────────────────────────────────────────────────────

func clipboardReadHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := readClipboard()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("clipboard read failed: %v", err)), nil
	}
	if text == "" {
		return mcp.NewToolResultText("(clipboard is empty)"), nil
	}
	lines := strings.Count(text, "\n") + 1
	return mcp.NewToolResultText(fmt.Sprintf("[Clipboard — %d bytes, %d line(s)]\n%s", len(text), lines, text)), nil
}

func readClipboard() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste", "--no-newline")
		} else {
			return "", fmt.Errorf("no clipboard tool found (install xclip, xsel, or wl-clipboard)")
		}
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// ─── http_request ─────────────────────────────────────────────────────────────

func httpRequestHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	httpReq, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid request: %v", err)), nil
	}

	// Headers from the "headers" object parameter.
	if rawHeaders, ok := req.GetArguments()["headers"]; ok && rawHeaders != nil {
		if hmap, ok := rawHeaders.(map[string]any); ok {
			for k, v := range hmap {
				httpReq.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	// Default Content-Type for requests with a body.
	if bodyStr != "" && httpReq.Header.Get("Content-Type") == "" {
		if json.Valid([]byte(bodyStr)) {
			httpReq.Header.Set("Content-Type", "application/json")
		} else {
			httpReq.Header.Set("Content-Type", "text/plain")
		}
	}

	// Basic auth.
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

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1 MB limit
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading response body: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Status:  %s\n", resp.Status)
	fmt.Fprintf(&sb, "Elapsed: %s\n", elapsed.Round(time.Millisecond))
	fmt.Fprintf(&sb, "Size:    %d bytes\n", len(respBody))

	if includeRespHeaders {
		sb.WriteString("\n── Response Headers ──\n")
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
		sb.WriteString("\n── Body ──\n")
		// Pretty-print JSON if applicable.
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

// ─── list_processes ───────────────────────────────────────────────────────────

type processInfo struct {
	PID     int
	Name    string
	CPU     float64
	Mem     float64
	Command string
}

func listProcessesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := strings.ToLower(strings.TrimSpace(req.GetString("filter", "")))
	sortBy := strings.ToLower(strings.TrimSpace(req.GetString("sort_by", "pid")))
	limit := int(req.GetFloat("limit", 50))
	if limit <= 0 {
		limit = 50
	}

	procs, err := listProcesses()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list processes failed: %v", err)), nil
	}

	// Filter.
	if filter != "" {
		filtered := procs[:0]
		for _, p := range procs {
			if strings.Contains(strings.ToLower(p.Name), filter) ||
				strings.Contains(strings.ToLower(p.Command), filter) {
				filtered = append(filtered, p)
			}
		}
		procs = filtered
	}

	// Sort.
	switch sortBy {
	case "cpu":
		sort.Slice(procs, func(i, j int) bool { return procs[i].CPU > procs[j].CPU })
	case "mem":
		sort.Slice(procs, func(i, j int) bool { return procs[i].Mem > procs[j].Mem })
	default: // pid
		sort.Slice(procs, func(i, j int) bool { return procs[i].PID < procs[j].PID })
	}

	total := len(procs)
	if total > limit {
		procs = procs[:limit]
	}

	if len(procs) == 0 {
		msg := "No processes found"
		if filter != "" {
			msg += fmt.Sprintf(" matching %q", filter)
		}
		return mcp.NewToolResultText(msg + "."), nil
	}

	var sb strings.Builder
	// Calculate dynamic column widths.
	maxNameLen := 4 // min "NAME"
	for _, p := range procs {
		if len(p.Name) > maxNameLen {
			maxNameLen = len(p.Name)
		}
	}
	maxCmdLen := 60
	nameColFmt := fmt.Sprintf("%%-%ds", maxNameLen)
	header := fmt.Sprintf("%-8s "+nameColFmt+" %6s %6s  %s\n", "PID", "NAME", "CPU%", "MEM%", "COMMAND")
	fmt.Fprint(&sb, header)
	fmt.Fprintln(&sb, strings.Repeat("─", 8+1+maxNameLen+1+6+1+6+2+maxCmdLen))
	for _, p := range procs {
		cmd := p.Command
		if len(cmd) > maxCmdLen {
			cmd = cmd[:maxCmdLen-3] + "..."
		}
		fmt.Fprintf(&sb, "%-8d "+nameColFmt+" %6.1f %6.1f  %s\n", p.PID, p.Name, p.CPU, p.Mem, cmd)
	}
	fmt.Fprintf(&sb, "\nShowing %d of %d processes", len(procs), total)
	if filter != "" {
		fmt.Fprintf(&sb, " matching %q", filter)
	}
	sb.WriteByte('.')

	return mcp.NewToolResultText(sb.String()), nil
}

func listProcesses() ([]processInfo, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
		return listProcessesPosix()
	case "windows":
		return listProcessesWindows()
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func listProcessesPosix() ([]processInfo, error) {
	out, err := exec.Command("ps", "axo", "pid,pcpu,pmem,comm,command").Output()
	if err != nil {
		return nil, err
	}
	var procs []processInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		mem, _ := strconv.ParseFloat(fields[2], 64)
		name := filepath.Base(fields[3])
		command := strings.Join(fields[4:], " ")
		procs = append(procs, processInfo{PID: pid, Name: name, CPU: cpu, Mem: mem, Command: command})
	}
	return procs, nil
}

func listProcessesWindows() ([]processInfo, error) {
	out, err := exec.Command("powershell", "-Command",
		"Get-Process | Select-Object Id,Name,CPU,WorkingSet | ConvertTo-Csv -NoTypeInformation").Output()
	if err != nil {
		return nil, err
	}
	var procs []processInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // skip CSV header
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			continue
		}
		stripQ := func(s string) string { return strings.Trim(strings.TrimSpace(s), `"`) }
		pid, _ := strconv.Atoi(stripQ(parts[0]))
		name := stripQ(parts[1])
		cpu, _ := strconv.ParseFloat(stripQ(parts[2]), 64)
		memBytes, _ := strconv.ParseFloat(stripQ(parts[3]), 64)
		procs = append(procs, processInfo{PID: pid, Name: name, CPU: cpu, Mem: memBytes / 1024 / 1024, Command: name})
	}
	return procs, nil
}

// ─── get_system_info ──────────────────────────────────────────────────────────

func getSystemInfoHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var sb strings.Builder

	// OS & arch
	hostname, _ := os.Hostname()
	fmt.Fprintf(&sb, "OS:       %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if hostname != "" {
		fmt.Fprintf(&sb, "Hostname: %s\n", hostname)
	}
	fmt.Fprintf(&sb, "CPUs:     %d logical core(s)\n", runtime.NumCPU())

	// CPU model
	if model := cpuModel(); model != "" {
		fmt.Fprintf(&sb, "CPU:      %s\n", model)
	}

	// Memory
	if mem, err := memoryInfo(); err == nil {
		fmt.Fprintf(&sb, "\n── Memory ──\n%s", mem)
	}

	// Disk
	if disk, err := diskInfo(); err == nil {
		fmt.Fprintf(&sb, "\n── Disk ──\n%s", disk)
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func cpuModel() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "linux":
		out, err := exec.Command("sh", "-c", `grep -m1 "model name" /proc/cpuinfo | cut -d: -f2`).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func memoryInfo() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return "", err
		}
		totalBytes, _ := strconv.ParseInt(strings.TrimSpace(string(totalOut)), 10, 64)

		vmOut, err := exec.Command("vm_stat").Output()
		if err != nil {
			return "", err
		}
		pageSize := int64(16384) // default Apple Silicon page size
		vals := map[string]int64{}
		for _, line := range strings.Split(string(vmOut), "\n") {
			for _, key := range []string{"Pages free", "Pages inactive", "Pages speculative"} {
				if strings.HasPrefix(line, key) {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						v, _ := strconv.ParseInt(strings.TrimRight(parts[len(parts)-1], "."), 10, 64)
						vals[key] = v
					}
				}
			}
		}
		freePages := vals["Pages free"] + vals["Pages inactive"] + vals["Pages speculative"]
		freeBytes := freePages * pageSize
		usedBytes := totalBytes - freeBytes

		return fmt.Sprintf("  Total: %s\n  Used:  %s  (%.1f%%)\n  Free:  %s\n",
			formatBytes(totalBytes),
			formatBytes(usedBytes), float64(usedBytes)/float64(totalBytes)*100,
			formatBytes(freeBytes),
		), nil

	case "linux":
		out, err := exec.Command("free", "-b").Output()
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Mem:") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					total, _ := strconv.ParseInt(fields[1], 10, 64)
					used, _ := strconv.ParseInt(fields[2], 10, 64)
					free, _ := strconv.ParseInt(fields[3], 10, 64)
					return fmt.Sprintf("  Total: %s\n  Used:  %s  (%.1f%%)\n  Free:  %s\n",
						formatBytes(total),
						formatBytes(used), float64(used)/float64(total)*100,
						formatBytes(free),
					), nil
				}
			}
		}
	case "windows":
		out, err := exec.Command("powershell", "-Command",
			"$m = Get-CimInstance Win32_OperatingSystem; "+
				"Write-Output (\"Total:\"+$m.TotalVisibleMemorySize+\" Free:\"+$m.FreePhysicalMemory)").Output()
		if err != nil {
			return "", err
		}
		return "  " + strings.TrimSpace(string(out)) + " KB\n", nil
	}
	return "", fmt.Errorf("unsupported OS")
}

func diskInfo() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin", "linux":
		cmd = exec.Command("df", "-h", "-P")
	case "windows":
		cmd = exec.Command("powershell", "-Command",
			"Get-PSDrive -PSProvider FileSystem | "+
				"Select-Object Name,@{N='Used(GB)';E={[math]::Round($_.Used/1GB,1)}},@{N='Free(GB)';E={[math]::Round($_.Free/1GB,1)}} | "+
				"Format-Table -AutoSize | Out-String")
	default:
		return "", fmt.Errorf("unsupported OS")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Keep only the first 10 lines (skip network mounts etc.)
	lines := strings.Split(string(out), "\n")
	// For POSIX df, filter to real filesystems (skip tmpfs, devfs, etc.)
	var kept []string
	if runtime.GOOS != "windows" {
		for _, line := range lines {
			if line == "" {
				continue
			}
			// Always keep header
			if strings.HasPrefix(line, "Filesystem") {
				kept = append(kept, "  "+line)
				continue
			}
			// Skip virtual/pseudo filesystems
			fs := strings.Fields(line)
			if len(fs) == 0 {
				continue
			}
			skip := false
			for _, prefix := range []string{"devfs", "tmpfs", "map ", "none", "udev"} {
				if strings.HasPrefix(fs[0], prefix) {
					skip = true
					break
				}
			}
			if !skip {
				kept = append(kept, "  "+line)
			}
		}
	} else {
		for _, line := range lines {
			kept = append(kept, "  "+line)
		}
	}
	return strings.Join(kept, "\n") + "\n", nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
