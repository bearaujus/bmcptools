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
	"github.com/mark3labs/mcp-go/server"
)

func registerExecTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("get_working_directory",
		mcp.WithDescription(
			"Return the server's current working directory along with basic environment info "+
				"(OS, hostname). Use this as your first call when you need to understand "+
				"where you are in the filesystem before navigating or reading files.",
		),
	), getWorkingDirectoryHandler)

	s.AddTool(mcp.NewTool("run_command",
		mcp.WithDescription(
			"Execute a shell command and return its combined stdout+stderr output. "+
				"Useful for running builds, tests, git operations, linters, and other CLI tools. "+
				"Commands run with a configurable timeout (default 60 s, max 600 s). "+
				"The working directory defaults to the current process directory but can be overridden.",
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("The command to run (passed to the OS shell: cmd /C on Windows, sh -c elsewhere)"),
		),
		mcp.WithString("cwd",
			mcp.Description("Working directory for the command. Defaults to the server's working directory."),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Maximum seconds to wait before killing the command. Default: 60, max: 600."),
		),
		mcp.WithArray("env",
			mcp.Description(
				`Additional environment variables in ["KEY=value", ...] format. `+
					`Merged on top of the current process environment. `+
					`Example: ["GOFLAGS=-mod=vendor", "CGO_ENABLED=0"]`,
			),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_output_bytes",
			mcp.Description(
				"Maximum bytes of command output to return. "+
					"Useful when commands produce large output that could overwhelm context. "+
					"Output is truncated from the end when the limit is exceeded. Default: no limit.",
			),
		),
		mcp.WithString("stdin",
			mcp.Description("Content to pass to the command's standard input. Useful for commands that read from stdin (e.g. piping a script or data)."),
		),
	), runCommandHandler)
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
	// Show a condensed PATH (first 3 entries).
	if pathVal := os.Getenv("PATH"); pathVal != "" {
		sep := string(os.PathListSeparator)
		parts := strings.SplitN(pathVal, sep, 5)
		display := strings.Join(parts[:min(3, len(parts))], sep)
		if len(parts) > 3 {
			display += fmt.Sprintf("%s... (+%d more)", sep, len(parts)-3)
		}
		envLines = append(envLines, fmt.Sprintf("  %-14s %s", "PATH:", display))
	}
	if len(envLines) > 0 {
		sb.WriteString("Environment:\n")
		for _, l := range envLines {
			sb.WriteString(l + "\n")
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func runCommandHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	if cwd != "" {
		cmd.Dir = cwd
	}

	if extraEnv := req.GetStringSlice("env", nil); len(extraEnv) > 0 {
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

	if runErr != nil && ctx.Err() == context.DeadlineExceeded {
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

	fmt.Fprintf(&sb, "$ %s\n", command)
	fmt.Fprintf(&sb, "cwd: %s\n", resolvedCWD)
	fmt.Fprintf(&sb, "exit: %d  elapsed: %s\n\n", exitCode, elapsed.Round(time.Millisecond))
	sb.WriteString(truncateOutput(output, maxOutputBytes))

	if exitCode != 0 {
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
