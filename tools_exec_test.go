package main

import (
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
	// CWD is now always shown.
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
	// Inject GOARCH=wasm and verify `go env GOARCH` echoes it back.
	// `go env` honours the GOARCH env var, making this cross-platform.
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
	// On both Windows and Unix, writing to stderr via a command.
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
	// Generate output larger than the limit.
	req := newTestRequest(map[string]any{
		"command":          "echo AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"max_output_bytes": float64(10),
	})
	result, err := runCommandHandler(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	// Output should be truncated.
	if !strings.Contains(text, "truncated") {
		t.Errorf("expected 'truncated' in output when max_output_bytes exceeded: %q", text)
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		input    string
		limit    int
		wantFull bool
	}{
		{"hello world", 0, true},     // no limit
		{"hello world", 100, true},   // under limit
		{"hello world", 5, false},    // over limit
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
	// Use a platform-appropriate command to echo stdin back.
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
