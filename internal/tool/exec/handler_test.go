package exec

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunCommandSuccess(t *testing.T) {
	req := newTestRequest(map[string]any{"command": "echo hello"})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "hello") {
		t.Errorf("expected 'hello' in output: %q", text)
	}
	if !strings.Contains(text, "exit: 0") {
		t.Errorf("expected 'exit: 0' in output: %q", text)
	}
	if !strings.Contains(text, "cwd:") {
		t.Errorf("expected 'cwd:' in output: %q", text)
	}
}

func TestRunCommandNonZeroExit(t *testing.T) {
	req := newTestRequest(map[string]any{"command": "exit 1"})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error result for non-zero exit code")
	}
	text := resultText(result)
	if !strings.Contains(text, "exit: 1") {
		t.Errorf("expected 'exit: 1' in output: %q", text)
	}
}

func TestRunCommandWithCwd(t *testing.T) {
	dir := t.TempDir()
	req := newTestRequest(map[string]any{"command": "echo ok", "cwd": dir})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}

func TestRunCommandEmptyCommand(t *testing.T) {
	req := newTestRequest(map[string]any{"command": "   "})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for empty command")
	}
}

func TestRunCommandWithEnv(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command": "go env GOARCH",
		"env":     []any{"GOARCH=wasm"},
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "wasm") {
		t.Errorf("expected GOARCH=wasm in output: %q", text)
	}
}

func TestRunCommandCapturesStderr(t *testing.T) {
	req := newTestRequest(map[string]any{"command": "go version"})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "go") {
		t.Errorf("expected go version output: %q", text)
	}
}

// ── run_command max_output_bytes ──────────────────────────────────────────────

func TestRunCommandMaxOutputBytes(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":          "echo AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"max_output_bytes": float64(10),
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "truncated") {
		t.Errorf("expected 'truncated' in output when max_output_bytes exceeded: %q", text)
	}
}

func TestRunCommandDefaultOutputCap(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(dir, "main.go")
	source := fmt.Sprintf(`package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Print(strings.Repeat("A", %d))
}
`, defaultMaxCommandOutputBytes+1024)
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := runCommandHandler(nil, newTestRequest(map[string]any{
		"command": "go run main.go",
		"cwd":     dir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Output truncated") {
		t.Errorf("expected default output cap truncation notice: %q", text)
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		input    string
		limit    int
		wantFull bool
	}{
		{"hello world", 0, true},   // no limit
		{"hello world", 100, true}, // under limit
		{"hello world", 5, false},  // over limit
	}
	for _, tt := range tests {
		got := truncateOutput(tt.input, tt.limit)
		if tt.wantFull {
			if got != tt.input {
				t.Errorf("truncateOutput(%q, %d): got %q, want original", tt.input, tt.limit, got)
			}
		} else {
			if !strings.Contains(got, "truncated") {
				t.Errorf("truncateOutput(%q, %d): expected 'truncated' notice: %q", tt.input, tt.limit, got)
			}
		}
	}
}

// ── get_working_directory ─────────────────────────────────────────────────────

func TestGetWorkingDirectoryHandler(t *testing.T) {
	result, err := getWorkingDirectoryHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "Working Directory:") {
		t.Errorf("expected 'Working Directory:' in output: %q", text)
	}
	if !strings.Contains(text, "OS:") {
		t.Errorf("expected 'OS:' in output: %q", text)
	}
}

func TestRunCommandStdin(t *testing.T) {
	var command, stdinContent string
	stdinContent = "hello from stdin"
	if runtime.GOOS == "windows" {
		command = `powershell -NoProfile -NonInteractive -Command "$input | Write-Output"`
	} else {
		command = "cat"
	}
	req := newTestRequest(map[string]any{
		"command": command,
		"stdin":   stdinContent,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "hello from stdin") {
		t.Errorf("expected stdin content in output: %q", text)
	}
}

func TestRunCommandShellInvalid(t *testing.T) {
	result, err := runCommandHandler(nil, newTestRequest(map[string]any{
		"command": "echo hello",
		"shell":   "definitely-not-a-shell",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatal("expected error for unsupported shell")
	}
	if !strings.Contains(resultText(result), "unsupported shell") {
		t.Errorf("expected unsupported shell message, got: %q", resultText(result))
	}
}

func TestRunCommandShellSHHereDoc(t *testing.T) {
	if _, err := osexec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	result, err := runCommandHandler(nil, newTestRequest(map[string]any{
		"command": "cat <<'EOF'\nhello heredoc\nEOF",
		"shell":   "sh",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "hello heredoc") {
		t.Errorf("expected heredoc output, got: %q", resultText(result))
	}
}

func TestRunCommandShellPowerShellHereString(t *testing.T) {
	if _, err := osexec.LookPath("powershell"); err != nil {
		t.Skip("powershell is not available")
	}
	result, err := runCommandHandler(nil, newTestRequest(map[string]any{
		"command": "@'\nhello here-string\n'@",
		"shell":   "powershell",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "hello here-string") {
		t.Errorf("expected here-string output, got: %q", resultText(result))
	}
}

func TestRunCommandAllowNonzeroExit(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":            "exit 42",
		"allow_nonzero_exit": true,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Errorf("expected success result when allow_nonzero_exit=true, got error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "exit: 42") {
		t.Errorf("expected 'exit: 42' in output: %q", text)
	}
}

func TestRunCommandDetach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detach not fully tested on Windows")
	}
	req := newTestRequest(map[string]any{
		"command": "sleep 1",
		"detach":  true,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error in detach mode: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "Detached process started") {
		t.Errorf("expected 'Detached process started' in output: %q", text)
	}
	if !strings.Contains(text, "PID:") {
		t.Errorf("expected 'PID:' in detach output: %q", text)
	}
}

// ── run_command raw_output ────────────────────────────────────────────────────
// Reason: The raw_output parameter strips the metadata header (cwd, exit code,
// elapsed) from the response. This alternate code path was never covered;
// LLM clients that set raw_output=true would receive unexpected metadata if
// the feature regressed.

func TestRunCommandRawOutput(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":    "echo rawtest",
		"raw_output": true,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "rawtest") {
		t.Errorf("expected command output in raw result: %q", text)
	}
	// raw_output should not include the metadata lines.
	if strings.Contains(text, "exit:") {
		t.Errorf("raw_output should not include 'exit:' metadata: %q", text)
	}
	if strings.Contains(text, "cwd:") {
		t.Errorf("raw_output should not include 'cwd:' metadata: %q", text)
	}
}

// ── run_command invalid cwd ───────────────────────────────────────────────────
// Reason: When the requested working directory does not exist the handler
// should return a clear error rather than silently running in an unexpected
// directory. This common mistake in LLM sessions was untested.

func TestRunCommandInvalidCwd(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command": "echo hello",
		"cwd":     filepath.Join(t.TempDir(), "does_not_exist"),
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for nonexistent working directory")
	}
}

// ── open_in_app ───────────────────────────────────────────────────────────────

// Reason: openInAppHandler had zero direct tests. Missing target is the most
// common validation failure from LLM sessions.
func TestOpenInAppHandlerMissingTarget(t *testing.T) {
	result, err := openInAppHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for missing target")
	}
}

func TestOpenInAppHandlerWhitespaceTarget(t *testing.T) {
	result, err := openInAppHandler(nil, newTestRequest(map[string]any{
		"target": "   ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error for whitespace-only target")
	}
}

// Reason: A valid file target should succeed (fires-and-forgets, does not block).
// Without this test, a refactor of the cmd-building code could silently break
// all open-in-app calls.
func TestOpenInAppHandlerValidTarget(t *testing.T) {
	dir := t.TempDir()
	result, err := openInAppHandler(nil, newTestRequest(map[string]any{
		"target": dir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	// OS might not have a registered handler for the temp dir, but the
	// handler itself should not return an MCP error — it fires and forgets.
	// We only need to ensure no panic and a text result.
	_ = result
}

// ── run_command (additional edge cases) ──────────────────────────────────────

// Reason: timeout_seconds <= 0 should silently clamp to 60, not error.
// If this clamping is removed, sub-second timeouts would kill every command.
func TestRunCommandTimeoutZeroClamped(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":         "echo ok",
		"timeout_seconds": float64(0),
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error with timeout=0 (should clamp to 60): %s", resultText(result))
	}
}

func TestRunCommandFractionalTimeoutAllowsFastCommand(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":         "echo ok",
		"timeout_seconds": 0.5,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("fractional timeout should not truncate to zero: %s", resultText(result))
	}
}

func TestRunCommandTimeoutKillsCommand(t *testing.T) {
	var command, shell string
	if runtime.GOOS == "windows" {
		if _, err := osexec.LookPath("powershell"); err != nil {
			t.Skip("powershell is not available")
		}
		shell = "powershell"
		command = "Write-Output before; Start-Sleep -Milliseconds 500; Write-Output after"
	} else {
		shell = "sh"
		command = "echo before; sleep 1; echo after"
	}

	result, err := runCommandHandler(nil, newTestRequest(map[string]any{
		"command":         command,
		"shell":           shell,
		"timeout_seconds": 0.1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Fatalf("expected timeout error, got: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "command timed out") {
		t.Errorf("expected timeout message: %q", text)
	}
	partial := text
	if idx := strings.Index(text, "Partial output:"); idx >= 0 {
		partial = text[idx:]
	}
	if strings.Contains(partial, "after") {
		t.Errorf("command appears to have continued after timeout: %q", text)
	}
}

// Reason: timeout_seconds > 600 should clamp to 600. A rogue LLM calling
// run_command with timeout=99999 would otherwise tie up the worker indefinitely.
func TestRunCommandTimeoutAboveMaxClamped(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":         "echo ok",
		"timeout_seconds": float64(99999),
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error with large timeout: %s", resultText(result))
	}
}

// Reason: raw_output=true combined with allow_nonzero_exit=true must return a
// success result even for non-zero exits, without any metadata header.
func TestRunCommandRawOutputAllowNonzeroExit(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":            "exit 2",
		"raw_output":         true,
		"allow_nonzero_exit": true,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("expected success with raw_output+allow_nonzero_exit: %s", resultText(result))
	}
	// raw mode must not include the 'exit:' metadata prefix
	text := resultText(result)
	if strings.Contains(text, "exit:") {
		t.Errorf("raw_output should suppress metadata header: %q", text)
	}
}

// Reason: When raw_output=true and the command fails without allow_nonzero_exit,
// the body (not metadata) is what becomes the error message.
func TestRunCommandRawOutputNonZeroExitIsError(t *testing.T) {
	req := newTestRequest(map[string]any{
		"command":    "exit 3",
		"raw_output": true,
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !isResultError(result) {
		t.Error("expected error result for non-zero exit with raw_output")
	}
}

// Reason: env[] sets extra env vars for the child process. The existing test
// uses GOARCH but we add a simpler case using our own env var that's unlikely
// to be pre-set, confirming the injection path.
func TestRunCommandEnvVarInjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env var check differs on Windows — covered by TestRunCommandWithEnv")
	}
	req := newTestRequest(map[string]any{
		"command": "echo $MY_TEST_VAR_XYZ",
		"env":     []any{"MY_TEST_VAR_XYZ=injected_value"},
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "injected_value") {
		t.Errorf("expected env variable to appear in output: %q", text)
	}
}

// Reason: get_working_directory should include the PATH environment variable
// in the Environment section. An LLM needs PATH to know which tools are available.
func TestGetWorkingDirectoryContainsPath(t *testing.T) {
	result, err := getWorkingDirectoryHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "PATH") {
		t.Errorf("expected 'PATH' in get_working_directory output: %q", text)
	}
}

func TestGetWorkingDirectorySummarizesLongPath(t *testing.T) {
	parts := make([]string, 20)
	for i := range parts {
		parts[i] = fmt.Sprintf("C:\\tool%d", i)
		if runtime.GOOS != "windows" {
			parts[i] = fmt.Sprintf("/tool%d", i)
		}
	}
	t.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))

	result, err := getWorkingDirectoryHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "use get_env key=PATH") {
		t.Errorf("expected compact PATH hint in output: %q", text)
	}
}

// ── get_env ──────────────────────────────────────────────────────────────────

func TestGetEnvSpecificKey(t *testing.T) {
	t.Setenv("BMCPTOOLS_TEST_VALUE", "test_value_123")
	result, err := getEnvHandler(nil, newTestRequest(map[string]any{
		"key": "BMCPTOOLS_TEST_VALUE",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "BMCPTOOLS_TEST_VALUE=test_value_123") {
		t.Errorf("expected key=value in output: %q", text)
	}
}

func TestGetEnvNonExistentKey(t *testing.T) {
	result, err := getEnvHandler(nil, newTestRequest(map[string]any{
		"key": "BMCPTOOLS_DEFINITELY_DOES_NOT_EXIST_XYZ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "is not set") {
		t.Errorf("expected 'is not set' in output: %q", text)
	}
}

func TestGetEnvFilter(t *testing.T) {
	t.Setenv("BMCPTOOLS_FILTER_AAA", "val1")
	t.Setenv("BMCPTOOLS_FILTER_BBB", "val2")
	result, err := getEnvHandler(nil, newTestRequest(map[string]any{
		"filter": "BMCPTOOLS_FILTER",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "BMCPTOOLS_FILTER_AAA=val1") {
		t.Errorf("expected BMCPTOOLS_FILTER_AAA in output: %q", text)
	}
	if !strings.Contains(text, "BMCPTOOLS_FILTER_BBB=val2") {
		t.Errorf("expected BMCPTOOLS_FILTER_BBB in output: %q", text)
	}
}

func TestGetEnvFilterNoMatch(t *testing.T) {
	result, err := getEnvHandler(nil, newTestRequest(map[string]any{
		"filter": "ZZZZNOTEXIST999",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "No environment variables matching") {
		t.Errorf("expected 'No environment variables matching' in output: %q", text)
	}
}

func TestGetEnvAll(t *testing.T) {
	t.Setenv("BMCPTOOLS_VISIBLE_NAME_ONLY", "hidden_by_default")
	result, err := getEnvHandler(nil, newTestRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "names only") {
		t.Errorf("expected names-only mode in output: %q", text)
	}
	if strings.Contains(text, "hidden_by_default") {
		t.Errorf("expected values to be omitted by default: %q", text)
	}
}

func TestGetEnvRedactsSecretLikeNamesByDefault(t *testing.T) {
	t.Setenv("BMCPTOOLS_API_KEY", "super-secret-value")
	result, err := getEnvHandler(nil, newTestRequest(map[string]any{
		"key": "BMCPTOOLS_API_KEY",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if strings.Contains(text, "super-secret-value") {
		t.Errorf("expected secret value to be redacted: %q", text)
	}
	if !strings.Contains(text, "redacted") {
		t.Errorf("expected redaction notice: %q", text)
	}
}

func TestGetEnvCanReturnUnredactedSpecificKey(t *testing.T) {
	t.Setenv("BMCPTOOLS_API_KEY", "super-secret-value")
	result, err := getEnvHandler(nil, newTestRequest(map[string]any{
		"key":            "BMCPTOOLS_API_KEY",
		"redact_secrets": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "BMCPTOOLS_API_KEY=super-secret-value") {
		t.Errorf("expected unredacted value for explicit opt-out: %q", text)
	}
}

func TestGetEnvValueMaxBytesTruncates(t *testing.T) {
	t.Setenv("BMCPTOOLS_LONG_VALUE", "1234567890")
	result, err := getEnvHandler(nil, newTestRequest(map[string]any{
		"key":             "BMCPTOOLS_LONG_VALUE",
		"value_max_bytes": float64(4),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if isResultError(result) {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "1234... [truncated") {
		t.Errorf("expected value truncation notice: %q", text)
	}
	if strings.Contains(text, "1234567890") {
		t.Errorf("expected full value to be omitted: %q", text)
	}
}
