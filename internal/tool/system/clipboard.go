package system

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const defaultClipboardReadMaxBytes = 256 * 1024

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

func clipboardReadHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := readClipboard()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("clipboard read failed: %v", err)), nil
	}
	if text == "" {
		return mcp.NewToolResultText("(clipboard is empty)"), nil
	}
	maxBytes := int(req.GetFloat("max_bytes", defaultClipboardReadMaxBytes))
	if maxBytes < 0 {
		maxBytes = defaultClipboardReadMaxBytes
	}
	return mcp.NewToolResultText(formatClipboardRead(text, maxBytes)), nil
}

func formatClipboardRead(text string, maxBytes int) string {
	truncated := false
	display := text
	if maxBytes > 0 && len(display) > maxBytes {
		display = display[:maxBytes]
		truncated = true
	}
	lines := strings.Count(display, "\n") + 1
	result := fmt.Sprintf("[Clipboard - %d/%d bytes shown, %d line(s)]\n%s", len(display), len(text), lines, display)
	if truncated {
		result += fmt.Sprintf("\n\n[Clipboard truncated at %d bytes. Increase max_bytes or set max_bytes=0 for unlimited.]", maxBytes)
	}
	return result
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
