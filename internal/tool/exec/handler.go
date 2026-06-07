package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

const (
	defaultMaxCommandOutputBytes = 256 * 1024
	defaultEnvListLimit          = 100
	defaultEnvValueMaxBytes      = 4096
)

func getWorkingDirectoryHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot get working directory: %v", err)), nil
	}
	hostname, _ := os.Hostname()
	var sb strings.Builder
	fmt.Fprintf(&sb, "Working Directory: %s\n", cwd)
	fmt.Fprintf(&sb, "OS:                %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if hostname != "" {
		fmt.Fprintf(&sb, "Hostname:          %s\n", hostname)
	}

	envKeys := []string{"HOME", "USERPROFILE", "GOPATH", "GOROOT"}
	var envLines []string
	for _, key := range envKeys {
		if val := os.Getenv(key); val != "" {
			envLines = append(envLines, fmt.Sprintf("  %-14s %s", key+":", val))
		}
	}
	if pathVal := os.Getenv("PATH"); pathVal != "" {
		envLines = append(envLines, fmt.Sprintf("  %-14s %s", "PATH:", summarizePathEnv(pathVal)))
	}
	if len(envLines) > 0 {
		sb.WriteString("Environment:\n")
		for _, l := range envLines {
			sb.WriteString(l + "\n")
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func summarizePathEnv(pathVal string) string {
	parts := strings.Split(pathVal, string(os.PathListSeparator))
	return fmt.Sprintf("%d entries; use get_env key=PATH for full value", len(parts))
}

func runCommandHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	command := req.GetString("command", "")
	if strings.TrimSpace(command) == "" {
		return mcp.NewToolResultError("command is required"), nil
	}

	timeoutSec := req.GetFloat("timeout_seconds", 60)
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	if timeoutSec > 600 {
		timeoutSec = 600
	}

	cwd := req.GetString("cwd", "")
	if err := validateCommandCWD(cwd); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	maxOutputBytes := defaultMaxCommandOutputBytes
	if _, explicit := req.GetArguments()["max_output_bytes"]; explicit {
		maxOutputBytes = 0
		if mob := req.GetFloat("max_output_bytes", 0); mob > 0 {
			maxOutputBytes = int(mob)
		}
	}

	allowNonzeroExit := req.GetBool("allow_nonzero_exit", false)
	detachFlag := req.GetBool("detach", false)
	rawOutput := req.GetBool("raw_output", false)
	extraEnv := req.GetStringSlice("env", nil)
	shellName := req.GetString("shell", "")

	if detachFlag {
		cmd, shellLabel, shellErr := newShellCommand(nil, shellName, command)
		if shellErr != nil {
			return mcp.NewToolResultError(shellErr.Error()), nil
		}
		if cwd != "" {
			cmd.Dir = cwd
		}
		cmd.Env = mergeCommandEnv(extraEnv)
		setSysProcDetach(cmd)

		devNull, err := os.Open(os.DevNull)
		if err == nil {
			cmd.Stdin = devNull
			defer devNull.Close()
		}

		if err := cmd.Start(); err != nil {
			if isCommandNotFoundError(err) {
				return mcp.NewToolResultError(formatMissingShellError(shellLabel)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("failed to start detached process: %v", err)), nil
		}
		pid := cmd.Process.Pid
		go func() { _ = cmd.Wait() }()

		resolvedCWD := cwd
		if resolvedCWD == "" {
			resolvedCWD, _ = os.Getwd()
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Detached process started.\n")
		fmt.Fprintf(&sb, "PID:     %d\n", pid)
		fmt.Fprintf(&sb, "Command: %s\n", command)
		fmt.Fprintf(&sb, "Shell:   %s\n", shellLabel)
		fmt.Fprintf(&sb, "cwd:     %s\n", resolvedCWD)
		sb.WriteString("Note:    Detached processes inherit the current environment and are not auto-cleaned.\n")
		fmt.Fprintf(&sb, "\nOutput is not captured. Use list_processes(filter=%q) to check status.", summarizeProcessFilter(command))
		return mcp.NewToolResultText(sb.String()), nil
	}

	timeout := time.Duration(timeoutSec * float64(time.Second))
	if timeout <= 0 {
		timeout = time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, shellLabel, shellErr := newShellCommand(cmdCtx, shellName, command)
	if shellErr != nil {
		return mcp.NewToolResultError(shellErr.Error()), nil
	}
	configureTimeoutCommand(cmd)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = mergeCommandEnv(extraEnv)
	if stdinContent := req.GetString("stdin", ""); stdinContent != "" {
		cmd.Stdin = strings.NewReader(stdinContent)
	}

	capture := &outputCapture{limit: maxOutputBytes}
	cmd.Stdout = capture
	cmd.Stderr = capture

	startTime := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(startTime)

	resolvedCWD := cwd
	if resolvedCWD == "" {
		resolvedCWD, _ = os.Getwd()
	}

	if runErr != nil && cmdCtx.Err() == context.DeadlineExceeded {
		if rawOutput {
			return mcp.NewToolResultText(capture.String()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(
			"timed out after %.1fs (killed) | %s | %s\n\n%s",
			timeoutSec,
			shellLabel,
			resolvedCWD,
			capture.String(),
		)), nil
	}

	var sb strings.Builder
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if isCommandNotFoundError(runErr) {
			return mcp.NewToolResultError(formatMissingShellError(shellLabel)), nil
		} else {
			return mcp.NewToolResultError(fmt.Sprintf("failed to run command: %v", runErr)), nil
		}
	}

	if rawOutput {
		body := capture.String()
		if exitCode != 0 && !allowNonzeroExit {
			return mcp.NewToolResultError(body), nil
		}
		return mcp.NewToolResultText(body), nil
	}

	fmt.Fprintf(&sb, "exit %d | %s | %s | %s\n\n", exitCode, elapsed.Round(time.Millisecond), shellLabel, resolvedCWD)
	sb.WriteString(capture.String())

	if exitCode != 0 && !allowNonzeroExit {
		return mcp.NewToolResultError(sb.String()), nil
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func newShellCommand(ctx context.Context, shell, command string) (*exec.Cmd, string, error) {
	exe, args, label, err := shellCommandParts(shell, command)
	if err != nil {
		return nil, "", err
	}
	if ctx != nil {
		return exec.CommandContext(ctx, exe, args...), label, nil
	}
	return exec.Command(exe, args...), label, nil
}

func shellCommandParts(shell, command string) (string, []string, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(shell))
	if normalized == "" || normalized == "default" {
		if runtime.GOOS == "windows" {
			normalized = "cmd"
		} else {
			normalized = "sh"
		}
	}

	switch normalized {
	case "cmd", "cmd.exe":
		return "cmd", []string{"/C", command}, "cmd", nil
	case "powershell", "powershell.exe":
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", command}, "powershell", nil
	case "pwsh", "pwsh.exe":
		return "pwsh", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}, "pwsh", nil
	case "bash":
		return "bash", []string{"-c", command}, "bash", nil
	case "sh":
		return "sh", []string{"-c", command}, "sh", nil
	default:
		return "", nil, "", fmt.Errorf("unsupported shell %q; use default, sh, bash, cmd, powershell, or pwsh", shell)
	}
}

type outputCapture struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	total     int
	truncated bool
}

func (c *outputCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.total += len(p)
	if c.limit <= 0 {
		_, err := c.buf.Write(p)
		return len(p), err
	}
	remaining := c.limit - c.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			cut := clampCommandRuneBoundary(p, remaining)
			_, _ = c.buf.Write(p[:cut])
			c.truncated = true
			return len(p), nil
		}
		_, _ = c.buf.Write(p)
		return len(p), nil
	}
	c.truncated = true
	return len(p), nil
}

func (c *outputCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	output := strings.ToValidUTF8(c.buf.String(), "\uFFFD")
	if !c.truncated {
		return output
	}
	return output + fmt.Sprintf(
		"\n\n[Output truncated — showing first %s of %s. Use max_output_bytes to adjust; set max_output_bytes=0 for unlimited.]",
		helper.HumanizeBytes(int64(c.limit)), helper.HumanizeBytes(int64(c.total)),
	)
}

func clampCommandRuneBoundary(p []byte, maxBytes int) int {
	if maxBytes <= 0 {
		return 0
	}
	if len(p) <= maxBytes {
		return len(p)
	}
	n := maxBytes
	for n > 0 && !utf8.RuneStart(p[n]) {
		n--
	}
	if n == 0 {
		return maxBytes
	}
	return n
}

func validateCommandCWD(cwd string) error {
	if cwd == "" {
		return nil
	}
	info, err := os.Stat(cwd)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cwd %q does not exist", cwd)
		}
		return fmt.Errorf("cannot access cwd %q: %w", cwd, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("cwd %q is not a directory", cwd)
	}
	return nil
}

func summarizeProcessFilter(command string) string {
	command = strings.TrimSpace(command)
	runes := []rune(command)
	if len(runes) <= 30 {
		return command
	}
	return string(runes[:30])
}

func openInAppHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if strings.TrimSpace(target) == "" {
		return mcp.NewToolResultError("target is required"), nil
	}
	if err := validateOpenTarget(target); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	app := req.GetString("app", "")

	if err := openInAppFn(target, app); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open %q: %v", target, err)), nil
	}

	msg := fmt.Sprintf("Opened %q", target)
	if app != "" {
		msg += fmt.Sprintf(" in %s", app)
	}
	return mcp.NewToolResultText(msg), nil
}

// openInAppFn opens target in the default (or specified) system application.
// Tests replace this with a no-op to avoid spawning real OS windows/dialogs.
var openInAppFn = func(target, app string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if app != "" {
			cmd = exec.Command("open", "-a", app, target)
		} else {
			cmd = exec.Command("open", target)
		}
	case "windows":
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			cmd = exec.Command("explorer.exe", target)
		} else {
			cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target)
		}
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func getEnvHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	key := strings.TrimSpace(req.GetString("key", ""))
	filter := strings.TrimSpace(req.GetString("filter", ""))
	includeValuesDefault := key != "" || filter != ""
	includeValues := req.GetBool("include_values", includeValuesDefault)
	redactSecrets := req.GetBool("redact_secrets", true)
	limit := int(req.GetFloat("limit", defaultEnvListLimit))
	if limit < 0 {
		limit = defaultEnvListLimit
	}
	valueMaxBytes := int(req.GetFloat("value_max_bytes", defaultEnvValueMaxBytes))
	if valueMaxBytes < 0 {
		valueMaxBytes = defaultEnvValueMaxBytes
	}

	if _, ok := args["include_values"]; !ok {
		includeValues = includeValuesDefault
	}

	if key != "" {
		val, ok := os.LookupEnv(key)
		if !ok {
			return mcp.NewToolResultText(fmt.Sprintf("%s is not set", key)), nil
		}
		if !includeValues {
			return mcp.NewToolResultText(fmt.Sprintf("%s is set (value omitted; set include_values=true to show it)", key)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s=%s", key, formatEnvValue(key, val, valueMaxBytes, redactSecrets))), nil
	}

	envVars := collectEnvVars()

	if filter != "" {
		filterLower := strings.ToLower(filter)
		var matched []envVar
		for _, e := range envVars {
			if strings.Contains(strings.ToLower(e.Name), filterLower) {
				matched = append(matched, e)
			}
		}
		if len(matched) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No environment variables matching %q", filter)), nil
		}
		return mcp.NewToolResultText(formatEnvList(matched, includeValues, redactSecrets, valueMaxBytes, limit,
			fmt.Sprintf("Environment variables matching %q", filter))), nil
	}

	return mcp.NewToolResultText(formatEnvList(envVars, includeValues, redactSecrets, valueMaxBytes, limit,
		"Environment variables")), nil
}

type envVar struct {
	Name  string
	Value string
}

func collectEnvVars() []envVar {
	raw := os.Environ()
	var vars []envVar
	for _, e := range raw {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			vars = append(vars, envVar{Name: e})
			continue
		}
		vars = append(vars, envVar{Name: e[:idx], Value: e[idx+1:]})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Name < vars[j].Name })
	return vars
}

func formatEnvList(vars []envVar, includeValues, redactSecrets bool, valueMaxBytes, limit int, title string) string {
	total := len(vars)
	if limit > 0 && total > limit {
		vars = vars[:limit]
	}

	var sb strings.Builder
	if includeValues {
		fmt.Fprintf(&sb, "%s (values shown; secret-like names redacted by default):\n", title)
	} else {
		fmt.Fprintf(&sb, "%s (names only; set include_values=true to include values):\n", title)
	}
	for _, e := range vars {
		if includeValues {
			fmt.Fprintf(&sb, "%s=%s\n", e.Name, formatEnvValue(e.Name, e.Value, valueMaxBytes, redactSecrets))
		} else {
			sb.WriteString(e.Name)
			sb.WriteByte('\n')
		}
	}
	fmt.Fprintf(&sb, "\nShowing %d of %d environment variable(s).", len(vars), total)
	if total > len(vars) {
		sb.WriteString(" Increase limit or set limit=0 for all matches.")
	}
	if !includeValues {
		sb.WriteString(" Prefer key for exact values and filter for targeted searches.")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatEnvValue(name, value string, maxBytes int, redactSecrets bool) string {
	if redactSecrets && isSecretEnvName(name) {
		return "[redacted; set redact_secrets=false with a specific key if you need this value]"
	}
	if maxBytes > 0 && len(value) > maxBytes {
		return fmt.Sprintf("%s... [truncated at %d/%d bytes; set value_max_bytes=0 for full value]", truncateStringByBytes(value, maxBytes), maxBytes, len(value))
	}
	return value
}

func isSecretEnvName(name string) bool {
	segments := splitEnvNameSegments(name)
	if len(segments) == 0 {
		return false
	}
	secretMarkers := map[string]struct{}{
		"KEY": {}, "TOKEN": {}, "SECRET": {}, "PASSWORD": {}, "PASS": {}, "PASSWD": {},
		"PRIVATE": {}, "CREDENTIAL": {}, "CREDENTIALS": {}, "AUTH": {}, "APIKEY": {},
		"DSN": {}, "JWT": {}, "COOKIE": {}, "SESSION": {}, "OTP": {}, "PAT": {},
		"SALT": {}, "SEED": {}, "MNEMONIC": {},
	}
	safeSuffixes := map[string]struct{}{
		"AUDIENCE": {}, "TYPE": {}, "TYPES": {}, "KIND": {}, "NAME": {}, "NAMES": {},
		"FORMAT": {}, "PREFIX": {}, "SUFFIX": {}, "PATH": {}, "FILE": {}, "FILES": {},
		"LENGTH": {}, "LEN": {}, "TTL": {}, "AGE": {}, "ENABLED": {}, "DISABLED": {},
	}
	for i, segment := range segments {
		if _, ok := secretMarkers[segment]; !ok {
			continue
		}
		if i == len(segments)-1 {
			return true
		}
		if _, safe := safeSuffixes[segments[i+1]]; safe {
			continue
		}
		return true
	}
	if containsAnyEnvSegment(segments, "URL", "URI", "DSN") {
		return true
	}
	collapsed := strings.Join(segments, "")
	switch collapsed {
	case "TOKEN", "SECRET", "PASSWORD", "PASSWD", "APIKEY", "AUTHTOKEN", "ACCESSTOKEN",
		"REFRESHTOKEN", "SESSIONTOKEN", "PRIVATEKEY", "SECRETKEY", "CLIENTSECRET",
		"AWSSECRETACCESSKEY", "DATABASEURL":
		return true
	}
	return false
}

func containsAnyEnvSegment(segments []string, candidates ...string) bool {
	for _, segment := range segments {
		for _, candidate := range candidates {
			if segment == candidate {
				return true
			}
		}
	}
	return false
}

func splitEnvNameSegments(name string) []string {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if upper == "" {
		return nil
	}
	return strings.FieldsFunc(upper, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
}

func mergeCommandEnv(extraEnv []string) []string {
	env := append([]string(nil), os.Environ()...)
	if len(extraEnv) == 0 {
		return env
	}
	return append(env, extraEnv...)
}

func isCommandNotFoundError(err error) bool {
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var execErr *exec.Error
	return errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound)
}

func formatMissingShellError(shellLabel string) string {
	return fmt.Sprintf("shell %q is not installed or not on PATH; use shell=default or choose one of default, sh, bash, cmd, powershell, or pwsh", shellLabel)
}

func validateOpenTarget(target string) error {
	if strings.ContainsRune(target, '\x00') {
		return fmt.Errorf("target contains an invalid NUL byte")
	}
	if runtime.GOOS == "windows" && looksLikeWindowsDrivePath(target) {
		return nil
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" {
		return nil
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "file":
		return nil
	default:
		return fmt.Errorf("unsupported URL scheme %q; use http, https, file, or a local path", parsed.Scheme)
	}
}

func looksLikeWindowsDrivePath(target string) bool {
	if len(target) < 2 {
		return false
	}
	ch := target[0]
	return ((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) && target[1] == ':'
}

func truncateStringByBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	n := maxBytes
	for n > 0 && !utf8.ValidString(value[:n]) {
		n--
	}
	if n <= 0 {
		return ""
	}
	return value[:n]
}
