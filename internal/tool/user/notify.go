package user

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// sendNotificationFn is called whenever an OS notification needs to be sent.
// Tests override this to a no-op to prevent real notifications from appearing.
var sendNotificationFn = sendNotification

func notifyUserHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	message := req.GetString("message", "")
	if strings.TrimSpace(message) == "" {
		return mcp.NewToolResultError("message is required"), nil
	}
	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}
	level := req.GetString("level", "info")
	if level != "warning" && level != "error" {
		level = "info"
	}
	durationSec := req.GetFloat("duration_seconds", 5)
	if durationSec <= 0 {
		durationSec = 5
	}
	if durationSec > 60 {
		durationSec = 60
	}

	go sendNotificationFn(message, title, level, int(durationSec))

	return mcp.NewToolResultText(fmt.Sprintf(
		"[Notification sent] title=%q level=%s message_bytes=%d",
		title, level, len(message),
	)), nil
}

func sendNotification(message, title, level string, durationSec int) {
	// Always echo to stderr so there is a log trail even when the OS notification
	// is silently skipped (CI, headless, unsupported platform, etc.).
	fmt.Fprintf(os.Stderr, "\n[AI NOTIFY][%s] %s: %s\n", strings.ToUpper(level), title, message)
	var delivered bool
	switch runtime.GOOS {
	case "windows":
		sendNotificationWindows(message, title, level, durationSec)
		delivered = true
	case "darwin":
		subtitle := map[string]string{
			"warning": "⚠ WARNING",
			"error":   "🔴 ERROR",
		}[level]
		var script string
		if subtitle != "" {
			script = fmt.Sprintf(
				`display notification %s with title %s subtitle %s sound name "default"`,
				macASQuote(message), macASQuote(title), macASQuote(subtitle),
			)
		} else {
			script = fmt.Sprintf(
				`display notification %s with title %s sound name "default"`,
				macASQuote(message), macASQuote(title),
			)
		}
		if exec.Command("osascript", "-e", script).Run() == nil {
			delivered = true
			break
		}
		args := []string{"-message", message, "-title", title, "-sound", "default"}
		if subtitle != "" {
			args = append(args, "-subtitle", subtitle)
		}
		delivered = exec.Command("terminal-notifier", args...).Run() == nil
	default:
		urgency := "low"
		switch level {
		case "warning":
			urgency = "normal"
		case "error":
			urgency = "critical"
		}
		expireMs := durationSec * 1000
		delivered = exec.Command("notify-send",
			fmt.Sprintf("--expire-time=%d", expireMs),
			"--urgency="+urgency, title, message,
		).Run() == nil
	}
	if !delivered {
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", strings.ToUpper(level), title, message)
	}
}

func macASQuote(s string) string {
	s = strings.ReplaceAll(s, `"`, `" & quote & "`)
	s = strings.ReplaceAll(s, "\r\n", `" & return & "`)
	s = strings.ReplaceAll(s, "\n", `" & return & "`)
	s = strings.ReplaceAll(s, "\r", `" & return & "`)
	return `"` + s + `"`
}
