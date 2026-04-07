package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func registerExecTools(s ToolRegistrar) {
	s.AddTool(mcp.NewTool("get_working_directory",
		mcp.WithDescription(td("get_working_directory")),
	), getWorkingDirectoryHandler)

	s.AddTool(mcp.NewTool("run_command",
		mcp.WithDescription(td("run_command")),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description(pd("run_command", "command")),
		),
		mcp.WithString("cwd",
			mcp.Description(pd("run_command", "cwd")),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description(pd("run_command", "timeout_seconds")),
		),
		mcp.WithArray("env",
			mcp.Description(pd("run_command", "env")),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_output_bytes",
			mcp.Description(pd("run_command", "max_output_bytes")),
		),
		mcp.WithString("stdin",
			mcp.Description(pd("run_command", "stdin")),
		),
		mcp.WithBoolean("allow_nonzero_exit",
			mcp.Description(pd("run_command", "allow_nonzero_exit")),
		),
		mcp.WithBoolean("detach",
			mcp.Description(pd("run_command", "detach")),
		),
		mcp.WithBoolean("raw_output",
			mcp.Description(pd("run_command", "raw_output")),
		),
	), runCommandHandler)

	s.AddTool(mcp.NewTool("open_in_app",
		mcp.WithDescription(td("open_in_app")),
		mcp.WithString("target",
			mcp.Required(),
			mcp.Description(pd("open_in_app", "target")),
		),
		mcp.WithString("app",
			mcp.Description(pd("open_in_app", "app")),
		),
	), openInAppHandler)
}

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

	// Show useful env vars for orientation.
	envKeys := []string{"HOME", "USERPROFILE", "GOPATH", "GOROOT"}
	var envLines []string
	for _, key := range envKeys {
		if val := os.Getenv(key); val != "" {
			envLines = append(envLines, fmt.Sprintf("  %-14s %s", key+":", val))
		}
	}
	// Show full PATH so the caller can reason about the execution environment.
	if pathVal := os.Getenv("PATH"); pathVal != "" {
		envLines = append(envLines, fmt.Sprintf("  %-14s %s", "PATH:", pathVal))
	}
	if len(envLines) > 0 {
		sb.WriteString("Environment:\n")
		for _, l := range envLines {
			sb.WriteString(l + "\n")
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
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

	maxOutputBytes := 0
	if mob := req.GetFloat("max_output_bytes", 0); mob > 0 {
		maxOutputBytes = int(mob)
	}

	allowNonzeroExit := req.GetBool("allow_nonzero_exit", false)
	detach := req.GetBool("detach", false)
	rawOutput := req.GetBool("raw_output", false)
	extraEnv := req.GetStringSlice("env", nil)

	// ── detach mode: start a fully independent background process ──────────────
	if detach {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", command)
		} else {
			cmd = exec.Command("sh", "-c", command)
		}
		if cwd != "" {
			cmd.Dir = cwd
		}
		if len(extraEnv) > 0 {
			cmd.Env = append(os.Environ(), extraEnv...)
		}
		setSysProcDetach(cmd)

		devNull, err := os.Open(os.DevNull)
		if err == nil {
			cmd.Stdin = devNull
			defer devNull.Close()
		}
		// Stdout/Stderr are nil — the child inherits nothing after setsid.

		if err := cmd.Start(); err != nil {
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
		fmt.Fprintf(&sb, "cwd:     %s\n", resolvedCWD)
		fmt.Fprintf(&sb, "\nOutput is not captured. Use list_processes(filter=%q) to check status.", command[:min(30, len(command))])
		return mcp.NewToolResultText(sb.String()), nil
	}

	// ── normal (blocking) mode ─────────────────────────────────────────────────
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	}
	if cwd != "" {
		cmd.Dir = cwd
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if stdinContent := req.GetString("stdin", ""); stdinContent != "" {
		cmd.Stdin = strings.NewReader(stdinContent)
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	startTime := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(startTime)

	output := buf.String()

	if runErr != nil && cmdCtx.Err() == context.DeadlineExceeded {
		return mcp.NewToolResultError(
			fmt.Sprintf("command timed out after %.1fs: %s\n\nPartial output:\n%s", timeoutSec, command, truncateOutput(output, maxOutputBytes)),
		), nil
	}

	var sb strings.Builder
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return mcp.NewToolResultError(fmt.Sprintf("failed to run command: %v", runErr)), nil
		}
	}

	resolvedCWD := cwd
	if resolvedCWD == "" {
		resolvedCWD, _ = os.Getwd()
	}

	if rawOutput {
		body := truncateOutput(output, maxOutputBytes)
		if exitCode != 0 && !allowNonzeroExit {
			return mcp.NewToolResultError(body), nil
		}
		return mcp.NewToolResultText(body), nil
	}

	fmt.Fprintf(&sb, "$ %s\n", command)
	fmt.Fprintf(&sb, "cwd: %s\n", resolvedCWD)
	fmt.Fprintf(&sb, "exit: %d  elapsed: %s\n\n", exitCode, elapsed.Round(time.Millisecond))
	sb.WriteString(truncateOutput(output, maxOutputBytes))

	if exitCode != 0 && !allowNonzeroExit {
		return mcp.NewToolResultError(sb.String()), nil
	}
	return mcp.NewToolResultText(sb.String()), nil
}

// truncateOutput truncates output to maxBytes, appending a notice.
// If maxBytes is 0, output is returned as-is.
func truncateOutput(output string, maxBytes int) string {
	if maxBytes <= 0 || len(output) <= maxBytes {
		return output
	}
	return output[:maxBytes] + fmt.Sprintf(
		"\n\n[Output truncated — showing first %s of %s. Use max_output_bytes to adjust.]",
		humanizeBytes(int64(maxBytes)), humanizeBytes(int64(len(output))),
	)
}

// ── open_in_app ───────────────────────────────────────────────────────────────

func openInAppHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if strings.TrimSpace(target) == "" {
		return mcp.NewToolResultError("target is required"), nil
	}
	app := req.GetString("app", "")

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if app != "" {
			cmd = exec.Command("open", "-a", app, target)
		} else {
			cmd = exec.Command("open", target)
		}
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}

	if err := cmd.Start(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to open %q: %v", target, err)), nil
	}
	// Detach — we don't wait for the application to exit.
	go func() { _ = cmd.Wait() }()

	msg := fmt.Sprintf("Opened %q", target)
	if app != "" {
		msg += fmt.Sprintf(" in %s", app)
	}
	return mcp.NewToolResultText(msg), nil
}
