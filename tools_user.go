package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// askUserMu ensures at most one ask_user dialog is visible at a time.
// Concurrent calls queue and are served in arrival order.
var askUserMu sync.Mutex

func registerUserTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("notify_user",
			mcp.WithDescription(
				"Send a non-blocking notification/message to the user. "+
					"Unlike ask_user, this does NOT wait for a response — it fires and returns immediately. "+
					"Ideal for progress updates during long-running tasks: "+
					"\"Starting code analysis...\", \"Tests passed!\", \"Build complete.\". "+
					"On Windows a themed WPF toast appears in the bottom-right corner; "+
					"on macOS a system notification; on Linux notify-send is used. "+
					"Always falls back to stderr if the GUI is unavailable.",
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("The message to display to the user"),
			),
			mcp.WithString("title",
				mcp.Description(`Notification title. Default: "AI Assistant"`),
			),
			mcp.WithString("level",
				mcp.Description(`Notification severity: "info" (default), "warning", or "error". Affects the accent colour and urgency.`),
			),
			mcp.WithNumber("duration_seconds",
				mcp.Description("How long the toast stays visible before fading out (Windows/Linux only). Default: 5 s."),
			),
		),
		notifyUserHandler,
	)

	s.AddTool(
		mcp.NewTool("ask_user",
			mcp.WithDescription(
				"Ask the user a question and wait for their answer. "+
					"Use this whenever you are unsure about something, need clarification, "+
					"or require information that only the user can provide. "+
					"A resizable dialog box appears on the user's screen (falls back to console if no GUI is available). "+
					"The input box is multi-line: Enter sends the answer, Shift+Enter inserts a new line. "+
					"Provide 'choices' for a multiple-choice picker — the user selects one option rather than typing freely.\n\n"+
					"At most one ask_user dialog is visible at a time; concurrent calls queue automatically "+
					"and each dialog closes on its own timeout so the next one can appear.\n\n"+
					"Set notify=true (default) to also flash the taskbar and send a toast notification "+
					"so the user is alerted even when the dialog opens behind other windows.\n\n"+
					"IMPORTANT: MCP clients enforce their own request timeout (often 30-120 s). "+
					"Always set timeout_seconds explicitly — if the client's timeout fires first you will "+
					"get a transport error instead of the user's answer. A safe default is 300 s.",
			),
			mcp.WithString("question",
				mcp.Required(),
				mcp.Description("The question to present to the user"),
			),
			mcp.WithString("title",
				mcp.Description(`Title shown in the dialog window. Default: "AI Assistant"`),
			),
			mcp.WithArray("choices",
				mcp.Description(
					"Optional list of choices to present. "+
						"When provided, a searchable picker dialog is shown instead of a free-form input box. "+
						"The user selects one option; their selection is returned as-is.",
				),
				mcp.Items(map[string]any{"type": "string"}),
			),
			mcp.WithNumber("timeout_seconds",
				mcp.Description("Seconds to wait before auto-closing. Default: 300 s. Max: 3600 s. Keep below your MCP client's own request timeout."),
			),
			mcp.WithBoolean("notify",
				mcp.Description("Flash the taskbar and send a companion toast notification when the dialog opens. Default: true. Set false to suppress."),
			),
		),
		askUserHandler,
	)
}

func askUserHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	question := req.GetString("question", "")
	if strings.TrimSpace(question) == "" {
		return mcp.NewToolResultError("question is required"), nil
	}

	title := req.GetString("title", "AI Assistant")
	if title == "" {
		title = "AI Assistant"
	}

	choices := req.GetStringSlice("choices", nil)

	timeoutSec := req.GetFloat("timeout_seconds", 300)
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	timeout := time.Duration(timeoutSec) * time.Second

	notify := req.GetBool("notify", true)

	// Serialize: block until any previous dialog is dismissed or times out.
	askUserMu.Lock()
	defer askUserMu.Unlock()

	// Now that we hold the lock (i.e. the dialog is about to appear), alert the user.
	if notify {
		msg := question
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		go sendNotification(msg, title, "info", 10)
	}

	var answer string
	var err error
	if len(choices) > 0 {
		answer, err = promptUserChoice(question, title, choices, timeout)
	} else {
		answer, err = promptUser(question, title, timeout)
	}
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get user input: %v", err)), nil
	}

	if strings.TrimSpace(answer) == "" {
		return mcp.NewToolResultText("[User did not provide an answer or dismissed the dialog]"), nil
	}

	return mcp.NewToolResultText(answer), nil
}

// ── notify_user ───────────────────────────────────────────────────────────────

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

	go sendNotification(message, title, level, int(durationSec))
	fmt.Fprintf(os.Stderr, "\n[AI NOTIFY][%s] %s: %s\n", strings.ToUpper(level), title, message)

	return mcp.NewToolResultText(fmt.Sprintf("[Notification sent] %s", message)), nil
}

// sendNotification dispatches to the OS-appropriate non-blocking notification.
func sendNotification(message, title, level string, durationSec int) {
	switch runtime.GOOS {
	case "windows":
		sendNotificationWindows(message, title, level, durationSec)
	case "darwin":
		safeM := strings.ReplaceAll(message, `"`, `\"`)
		safeT := strings.ReplaceAll(title, `"`, `\"`)
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, safeM, safeT)
		_ = exec.Command("osascript", "-e", script).Run()
	default:
		urgency := "low"
		switch level {
		case "warning":
			urgency = "normal"
		case "error":
			urgency = "critical"
		}
		expireMs := durationSec * 1000
		_ = exec.Command("notify-send",
			fmt.Sprintf("--expire-time=%d", expireMs),
			"--urgency="+urgency, title, message,
		).Run()
	}
}

// ── free-form prompt ──────────────────────────────────────────────────────────

func promptUser(question, title string, timeout time.Duration) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return promptWindows(question, title, timeout)
	case "darwin":
		return promptMac(question, title, timeout)
	default:
		return promptLinux(question, title, timeout)
	}
}

func promptWindows(question, title string, timeout time.Duration) (string, error) {
	script := buildWPFInputScript(
		sanitizePSHereString(question),
		sanitizePSHereString(title),
		int(timeout.Seconds()),
	)
	result, err := runPSTempFile(script, timeout)
	if err != nil {
		return promptConsole(question)
	}
	return result, nil
}

func promptMac(question, title string, timeout time.Duration) (string, error) {
	safeQ := strings.ReplaceAll(question, `"`, `\"`)
	safeT := strings.ReplaceAll(title, `"`, `\"`)

	script := fmt.Sprintf(
		`text returned of (display dialog "%s" with title "%s" default answer "")`,
		safeQ, safeT,
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return promptConsole(question)
	}

	return strings.TrimRight(string(out), "\r\n"), nil
}

func promptLinux(question, title string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "zenity", "--entry", "--title="+title, "--text="+question)
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	cmd = exec.CommandContext(ctx, "kdialog", "--inputbox", question, "--title", title)
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	cmd = exec.CommandContext(ctx, "bash", "-c",
		fmt.Sprintf(`whiptail --inputbox %q 8 72 2>&1 >/dev/tty`, question))
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	return promptConsole(question)
}

// ── multiple-choice prompt ────────────────────────────────────────────────────

func promptUserChoice(question, title string, choices []string, timeout time.Duration) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return promptChoiceWindows(question, title, choices, timeout)
	case "darwin":
		return promptChoiceMac(question, title, choices, timeout)
	default:
		return promptChoiceLinux(question, title, choices, timeout)
	}
}

func promptChoiceWindows(question, title string, choices []string, timeout time.Duration) (string, error) {
	choicesJSON, err := json.Marshal(choices)
	if err != nil {
		return promptChoiceConsole(question, title, choices)
	}
	script := buildWPFChoiceScript(
		sanitizePSHereString(question),
		sanitizePSHereString(title),
		string(choicesJSON),
		int(timeout.Seconds()),
	)
	result, err := runPSTempFile(script, timeout)
	if err != nil {
		return promptChoiceConsole(question, title, choices)
	}
	return result, nil
}

func promptChoiceMac(question, title string, choices []string, timeout time.Duration) (string, error) {
	safeQ := strings.ReplaceAll(question, `"`, `\"`)
	safeT := strings.ReplaceAll(title, `"`, `\"`)

	quotedChoices := make([]string, len(choices))
	for i, c := range choices {
		quotedChoices[i] = fmt.Sprintf(`"%s"`, strings.ReplaceAll(c, `"`, `\"`))
	}
	listLiteral := "{" + strings.Join(quotedChoices, ", ") + "}"

	script := fmt.Sprintf(
		`set chosen to choose from list %s with title "%s" with prompt "%s" OK button name "Select" cancel button name "Cancel"`+
			`\nif chosen is false then\n  return ""\nelse\n  return item 1 of chosen\nend if`,
		listLiteral, safeT, safeQ,
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return promptChoiceConsole(question, title, choices)
	}

	return strings.TrimRight(string(out), "\r\n"), nil
}

func promptChoiceLinux(question, title string, choices []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"--list", "--title=" + title, "--text=" + question, "--column=Options", "--hide-header"}
	args = append(args, choices...)
	cmd := exec.CommandContext(ctx, "zenity", args...)
	if out, err := cmd.Output(); err == nil {
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	args = []string{"--menu", question, "--title", title}
	for i, c := range choices {
		args = append(args, fmt.Sprintf("%d", i+1), c)
	}
	cmd = exec.CommandContext(ctx, "kdialog", args...)
	if out, err := cmd.Output(); err == nil {
		result := strings.TrimRight(string(out), "\r\n")
		for i := range choices {
			if result == fmt.Sprintf("%d", i+1) {
				return choices[i], nil
			}
		}
		return result, nil
	}

	return promptChoiceConsole(question, title, choices)
}

func promptChoiceConsole(question, title string, choices []string) (string, error) {
	printConsolePromptHeader(title)
	fmt.Fprintf(os.Stderr, "%s\n\nChoices:\n", question)
	for i, c := range choices {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, c)
	}
	fmt.Fprintf(os.Stderr, "\nEnter number or text: ")

	var ttyPath string
	if runtime.GOOS == "windows" {
		ttyPath = "CONIN$"
	} else {
		ttyPath = "/dev/tty"
	}

	tty, err := os.Open(ttyPath)
	if err != nil {
		return "", fmt.Errorf("cannot open console (%s): %w", ttyPath, err)
	}
	defer tty.Close()

	scanner := bufio.NewScanner(tty)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		for i, c := range choices {
			if input == fmt.Sprintf("%d", i+1) {
				return c, nil
			}
		}
		return input, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func printConsolePromptHeader(title string) {
	const (
		borderCols = 50
		label      = "  AI ASSISTANT QUESTION"
	)
	border := strings.Repeat("═", borderCols)
	padding := strings.Repeat(" ", borderCols-len(label))
	fmt.Fprintf(os.Stderr, "\n╔%s╗\n║%s%s║\n╚%s╝\n", border, label, padding, border)
	if title != "" && title != "AI Assistant" {
		fmt.Fprintf(os.Stderr, "[%s]\n", title)
	}
}

func promptConsole(question string) (string, error) {
	printConsolePromptHeader("AI Assistant")
	fmt.Fprintf(os.Stderr, "%s\n\nYour answer: ", question)

	var ttyPath string
	if runtime.GOOS == "windows" {
		ttyPath = "CONIN$"
	} else {
		ttyPath = "/dev/tty"
	}

	tty, err := os.Open(ttyPath)
	if err != nil {
		return "", fmt.Errorf("cannot open console (%s): %w", ttyPath, err)
	}
	defer tty.Close()

	scanner := bufio.NewScanner(tty)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

// ── Windows WPF notification / dialog helpers ─────────────────────────────────

// wpfNotifyScriptTmpl is a PowerShell WPF script that renders a themed toast-style
// popup in the bottom-right corner.  Placeholders: {{ACCENT}}, {{TITLE}}, {{MESSAGE}}.
const wpfNotifyScriptTmpl = `
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase
$isDark = $false
try {
    $val = Get-ItemPropertyValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' -Name 'AppsUseLightTheme' -ErrorAction Stop
    $isDark = ($val -eq 0)
} catch {}
$bg      = if ($isDark) { '#2D2D2D' } else { '#FFFFFF' }
$fg      = if ($isDark) { '#E0E0E0' } else { '#1A1A1A' }
$titleFg = if ($isDark) { '#FFFFFF' } else { '#000000' }
$accent  = '{{ACCENT}}'
[xml]$xaml = @"
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        AllowsTransparency="True" WindowStyle="None"
        SizeToContent="WidthAndHeight" Topmost="True"
        Background="Transparent" ShowInTaskbar="False" Opacity="0">
  <Border Background="$bg" CornerRadius="8"
          BorderBrush="$accent" BorderThickness="0,0,0,3">
    <Grid Width="320">
      <Grid.ColumnDefinitions>
        <ColumnDefinition Width="4"/>
        <ColumnDefinition Width="*"/>
      </Grid.ColumnDefinitions>
      <Border Grid.Column="0" Background="$accent" CornerRadius="8,0,0,8"/>
      <StackPanel Grid.Column="1" Margin="14,12,14,12">
        <TextBlock Name="TitleBlock" FontSize="13" FontWeight="SemiBold"
                   FontFamily="Segoe UI" Foreground="$titleFg" Margin="0,0,0,3"/>
        <TextBlock Name="MsgBlock" FontSize="12" FontFamily="Segoe UI"
                   Foreground="$fg" TextWrapping="Wrap"/>
      </StackPanel>
    </Grid>
  </Border>
</Window>
"@
$reader = New-Object System.Xml.XmlNodeReader $xaml
$window = [System.Windows.Markup.XamlReader]::Load($reader)
$titleVal = @'
{{TITLE}}
'@
$window.FindName('TitleBlock').Text = $titleVal.Trim()
$msgVal = @'
{{MESSAGE}}
'@
$window.FindName('MsgBlock').Text = $msgVal.Trim()
$window.Add_MouseLeftButtonDown({ $window.Close() })
$script:toastTimer = $null
$window.Add_ContentRendered({
    $screen = [System.Windows.SystemParameters]::WorkArea
    $window.Left = $screen.Right  - $window.ActualWidth  - 20
    $window.Top  = $screen.Bottom - $window.ActualHeight - 20
    $fadeIn = [System.Windows.Media.Animation.DoubleAnimation]::new(0, 1,
        [System.Windows.Duration]::new([TimeSpan]::FromMilliseconds(250)))
    $window.BeginAnimation([System.Windows.UIElement]::OpacityProperty, $fadeIn)
    $script:toastTimer = [System.Windows.Threading.DispatcherTimer]::new()
    $script:toastTimer.Interval = [TimeSpan]::FromSeconds({{DURATION_SEC}})
    $script:toastTimer.Add_Tick({
        $script:toastTimer.Stop()
        $fadeOut = [System.Windows.Media.Animation.DoubleAnimation]::new(1, 0,
            [System.Windows.Duration]::new([TimeSpan]::FromMilliseconds(350)))
        $fadeOut.Add_Completed({ $window.Close() })
        $window.BeginAnimation([System.Windows.UIElement]::OpacityProperty, $fadeOut)
    })
    $script:toastTimer.Start()
})
$window.ShowDialog() | Out-Null
`

// wpfInputScriptTmpl is a PowerShell WPF free-form input dialog.
// Features: mandatory input (Send disabled until text entered), multi-line TextBox
// (Enter = send, Shift+Enter = new line), resizable window, scrollable question,
// countdown timer, FlashWindowEx taskbar alert, and bring-to-front on open.
// Placeholders: {{TITLE}}, {{QUESTION}}, {{TIMEOUT_SEC}}.
const wpfInputScriptTmpl = `
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase
try {
    Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public class WinFlash {
    [DllImport("user32.dll")]
    public static extern bool FlashWindowEx(ref FLASHWINFO pwfi);
    [StructLayout(LayoutKind.Sequential)]
    public struct FLASHWINFO {
        public uint cbSize;
        public IntPtr hwnd;
        public uint dwFlags;
        public uint uCount;
        public uint dwTimeout;
    }
    public static void Flash(IntPtr hwnd) {
        FLASHWINFO fw = new FLASHWINFO();
        fw.cbSize = (uint)System.Runtime.InteropServices.Marshal.SizeOf(fw);
        fw.hwnd = hwnd;
        fw.dwFlags = 3;
        fw.uCount = 5;
        fw.dwTimeout = 0;
        FlashWindowEx(ref fw);
    }
}
"@ -ErrorAction Stop
} catch {}
$isDark = $false
try {
    $val = Get-ItemPropertyValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' -Name 'AppsUseLightTheme' -ErrorAction Stop
    $isDark = ($val -eq 0)
} catch {}
$bg      = if ($isDark) { '#1E1E1E' } else { '#F9F9F9' }
$fg      = if ($isDark) { '#E8E8E8' } else { '#1A1A1A' }
$inputBg = if ($isDark) { '#2D2D2D' } else { '#FFFFFF' }
$border  = if ($isDark) { '#444444' } else { '#CCCCCC' }
$subFg   = if ($isDark) { '#888888' } else { '#999999' }
[xml]$xaml = @"
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        MinWidth="420" Width="520" MinHeight="260" Height="380"
        WindowStartupLocation="CenterScreen"
        ResizeMode="CanResizeWithGrip" Background="$bg"
        ShowInTaskbar="True">
  <Grid Margin="24,20,24,20">
    <Grid.RowDefinitions>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="12"/>
      <RowDefinition Height="*"/>
      <RowDefinition Height="10"/>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="6"/>
      <RowDefinition Height="Auto"/>
    </Grid.RowDefinitions>
    <ScrollViewer Grid.Row="0" MaxHeight="140" VerticalScrollBarVisibility="Auto"
                  HorizontalScrollBarVisibility="Disabled">
      <TextBlock Name="QuestionBlock" Foreground="$fg"
                 TextWrapping="Wrap" FontSize="13" FontFamily="Segoe UI"/>
    </ScrollViewer>
    <Grid Grid.Row="2">
      <TextBox Name="InputBox"
               Background="$inputBg" Foreground="$fg"
               BorderBrush="$border" BorderThickness="1"
               Padding="10,8" FontSize="13" FontFamily="Segoe UI"
               CaretBrush="$fg" AcceptsReturn="True"
               TextWrapping="Wrap" VerticalScrollBarVisibility="Auto"
               VerticalAlignment="Stretch"/>
      <TextBlock Name="PlaceholderBlock"
                 Text="Type your answer here...  (Shift+Enter for new line)"
                 Foreground="$subFg" FontSize="13" FontFamily="Segoe UI"
                 Padding="12,10,0,0" IsHitTestVisible="False"
                 VerticalAlignment="Top" HorizontalAlignment="Left"/>
    </Grid>
    <TextBlock Name="CountdownBlock" Grid.Row="4"
               Foreground="$subFg" FontSize="11" FontFamily="Segoe UI"
               HorizontalAlignment="Left"/>
    <StackPanel Grid.Row="6" Orientation="Horizontal" HorizontalAlignment="Right">
      <Button Name="CancelBtn" Content="Cancel" Width="90" Height="32" Margin="0,0,8,0"
              Background="$border" Foreground="$fg" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" Cursor="Hand"/>
      <Button Name="OkBtn" Content="Send" Width="90" Height="32"
              Background="#0078D4" Foreground="#FFFFFF" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" IsEnabled="False" Cursor="Hand"/>
    </StackPanel>
  </Grid>
</Window>
"@
$reader = New-Object System.Xml.XmlNodeReader $xaml
$window = [System.Windows.Markup.XamlReader]::Load($reader)
$titleVal = @'
{{TITLE}}
'@
$window.Title = $titleVal.Trim()
$questionVal = @'
{{QUESTION}}
'@
$window.FindName('QuestionBlock').Text = $questionVal.Trim()
$inputBox       = $window.FindName('InputBox')
$okBtn          = $window.FindName('OkBtn')
$cancelBtn      = $window.FindName('CancelBtn')
$placeholder    = $window.FindName('PlaceholderBlock')
$countdownBlock = $window.FindName('CountdownBlock')
$script:dialogResult = [string]::Empty

$inputBox.Add_TextChanged({
    $hasText = $inputBox.Text.Length -gt 0
    $placeholder.Visibility = if ($hasText) { 'Hidden' } else { 'Visible' }
    $okBtn.IsEnabled = $hasText
})

$script:doSubmit = {
    if ($inputBox.Text.Trim().Length -gt 0) {
        $script:dialogResult = $inputBox.Text
        $window.DialogResult = $true
    }
}

$okBtn.Add_Click({ & $script:doSubmit })
$cancelBtn.Add_Click({ $window.DialogResult = $false })

$inputBox.Add_PreviewKeyDown({
    param($s, $e)
    $shiftHeld = [System.Windows.Input.Keyboard]::IsKeyDown([System.Windows.Input.Key]::LeftShift) -or
                 [System.Windows.Input.Keyboard]::IsKeyDown([System.Windows.Input.Key]::RightShift)
    if ($e.Key -eq [System.Windows.Input.Key]::Return -and -not $shiftHeld) {
        $e.Handled = $true
        & $script:doSubmit
    }
    if ($e.Key -eq [System.Windows.Input.Key]::Escape) {
        $e.Handled = $true
        $window.DialogResult = $false
    }
})

$script:remainingSec = {{TIMEOUT_SEC}}
$script:countdownTimer = [System.Windows.Threading.DispatcherTimer]::new()
$script:countdownTimer.Interval = [TimeSpan]::FromSeconds(1)
$script:countdownTimer.Add_Tick({
    $script:remainingSec--
    if ($script:remainingSec -le 0) {
        $script:countdownTimer.Stop()
        $window.Close()
    } else {
        $mins = [int]($script:remainingSec / 60)
        $secs = $script:remainingSec % 60
        $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    }
})

$window.Add_ContentRendered({
    try {
        $hwnd = (New-Object System.Windows.Interop.WindowInteropHelper($window)).Handle
        [WinFlash]::Flash($hwnd)
    } catch {}
    $window.Topmost = $true
    $window.Activate()
    $window.Topmost = $false
    $inputBox.Focus() | Out-Null
    $mins = [int]($script:remainingSec / 60)
    $secs = $script:remainingSec % 60
    $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    $script:countdownTimer.Start()
})
$window.ShowDialog() | Out-Null
Write-Output $script:dialogResult
`

// wpfChoiceScriptTmpl is a PowerShell WPF list-picker dialog with live search,
// countdown timer, FlashWindowEx taskbar alert, and bring-to-front on open.
// Placeholders: {{TITLE}}, {{QUESTION}}, {{CHOICES_JSON}}, {{TIMEOUT_SEC}}.
const wpfChoiceScriptTmpl = `
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase
try {
    Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public class WinFlash {
    [DllImport("user32.dll")]
    public static extern bool FlashWindowEx(ref FLASHWINFO pwfi);
    [StructLayout(LayoutKind.Sequential)]
    public struct FLASHWINFO {
        public uint cbSize;
        public IntPtr hwnd;
        public uint dwFlags;
        public uint uCount;
        public uint dwTimeout;
    }
    public static void Flash(IntPtr hwnd) {
        FLASHWINFO fw = new FLASHWINFO();
        fw.cbSize = (uint)System.Runtime.InteropServices.Marshal.SizeOf(fw);
        fw.hwnd = hwnd;
        fw.dwFlags = 3;
        fw.uCount = 5;
        fw.dwTimeout = 0;
        FlashWindowEx(ref fw);
    }
}
"@ -ErrorAction Stop
} catch {}
$isDark = $false
try {
    $val = Get-ItemPropertyValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' -Name 'AppsUseLightTheme' -ErrorAction Stop
    $isDark = ($val -eq 0)
} catch {}
$bg      = if ($isDark) { '#1E1E1E' } else { '#F9F9F9' }
$fg      = if ($isDark) { '#E8E8E8' } else { '#1A1A1A' }
$inputBg = if ($isDark) { '#2D2D2D' } else { '#FFFFFF' }
$border  = if ($isDark) { '#444444' } else { '#CCCCCC' }
$subFg   = if ($isDark) { '#888888' } else { '#999999' }
[xml]$xaml = @"
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        Width="460" Height="460"
        MinWidth="380" MinHeight="340"
        WindowStartupLocation="CenterScreen"
        ResizeMode="CanResizeWithGrip" Background="$bg"
        ShowInTaskbar="True">
  <Grid Margin="24,20,24,20">
    <Grid.RowDefinitions>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="10"/>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="8"/>
      <RowDefinition Height="*"/>
      <RowDefinition Height="10"/>
      <RowDefinition Height="Auto"/>
      <RowDefinition Height="6"/>
      <RowDefinition Height="Auto"/>
    </Grid.RowDefinitions>
    <ScrollViewer Grid.Row="0" MaxHeight="120" VerticalScrollBarVisibility="Auto"
                  HorizontalScrollBarVisibility="Disabled">
      <TextBlock Name="QuestionBlock" Foreground="$fg"
                 TextWrapping="Wrap" FontSize="13" FontFamily="Segoe UI"/>
    </ScrollViewer>
    <TextBox Name="SearchBox" Grid.Row="2"
             Background="$inputBg" Foreground="$fg"
             BorderBrush="$border" BorderThickness="1"
             Padding="8,6" FontSize="12" FontFamily="Segoe UI"
             CaretBrush="$fg"/>
    <ListBox Name="ChoiceList" Grid.Row="4"
             Background="$inputBg" Foreground="$fg"
             BorderBrush="$border" BorderThickness="1"
             FontSize="13" FontFamily="Segoe UI"
             ScrollViewer.VerticalScrollBarVisibility="Auto">
      <ListBox.ItemContainerStyle>
        <Style TargetType="ListBoxItem">
          <Setter Property="Padding" Value="10,7"/>
          <Setter Property="Foreground" Value="$fg"/>
          <Style.Triggers>
            <Trigger Property="IsSelected" Value="True">
              <Setter Property="Background" Value="#0078D4"/>
              <Setter Property="Foreground" Value="#FFFFFF"/>
            </Trigger>
          </Style.Triggers>
        </Style>
      </ListBox.ItemContainerStyle>
    </ListBox>
    <TextBlock Name="CountdownBlock" Grid.Row="6"
               Foreground="$subFg" FontSize="11" FontFamily="Segoe UI"
               HorizontalAlignment="Left"/>
    <StackPanel Grid.Row="8" Orientation="Horizontal" HorizontalAlignment="Right">
      <Button Name="CancelBtn" Content="Cancel" Width="90" Height="32" Margin="0,0,8,0"
              Background="$border" Foreground="$fg" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" Cursor="Hand"/>
      <Button Name="SelectBtn" Content="Select" Width="90" Height="32"
              Background="#0078D4" Foreground="#FFFFFF" BorderThickness="0"
              FontSize="13" FontFamily="Segoe UI" Cursor="Hand"/>
    </StackPanel>
  </Grid>
</Window>
"@
$reader = New-Object System.Xml.XmlNodeReader $xaml
$window = [System.Windows.Markup.XamlReader]::Load($reader)
$titleVal = @'
{{TITLE}}
'@
$window.Title = $titleVal.Trim()
$questionVal = @'
{{QUESTION}}
'@
$window.FindName('QuestionBlock').Text = $questionVal.Trim()
$choicesJson = @'
{{CHOICES_JSON}}
'@
$listBox        = $window.FindName('ChoiceList')
$searchBox      = $window.FindName('SearchBox')
$selectBtn      = $window.FindName('SelectBtn')
$cancelBtn      = $window.FindName('CancelBtn')
$countdownBlock = $window.FindName('CountdownBlock')
$choicesArr = $choicesJson | ConvertFrom-Json
foreach ($c in $choicesArr) { $listBox.Items.Add($c) | Out-Null }
$script:dialogResult = [string]::Empty

$searchBox.Add_TextChanged({
    $filter = $searchBox.Text
    $listBox.Items.Clear()
    foreach ($c in $choicesArr) {
        if ($filter -eq '' -or $c -like "*$filter*") {
            $listBox.Items.Add($c) | Out-Null
        }
    }
    if ($listBox.Items.Count -gt 0) { $listBox.SelectedIndex = 0 }
})

$script:doSelect = {
    if ($null -ne $listBox.SelectedItem) {
        $script:dialogResult = $listBox.SelectedItem
        $window.DialogResult = $true
    }
}

$selectBtn.Add_Click({ & $script:doSelect })
$listBox.Add_MouseDoubleClick({ & $script:doSelect })
$cancelBtn.Add_Click({ $window.DialogResult = $false })

$window.Add_KeyDown({
    param($s, $e)
    if ($e.Key -eq [System.Windows.Input.Key]::Return) { & $script:doSelect }
    if ($e.Key -eq [System.Windows.Input.Key]::Escape)  { $window.DialogResult = $false }
    if ($e.Key -eq [System.Windows.Input.Key]::Down)    { $listBox.Focus() | Out-Null }
})

$script:remainingSec = {{TIMEOUT_SEC}}
$script:countdownTimer = [System.Windows.Threading.DispatcherTimer]::new()
$script:countdownTimer.Interval = [TimeSpan]::FromSeconds(1)
$script:countdownTimer.Add_Tick({
    $script:remainingSec--
    if ($script:remainingSec -le 0) {
        $script:countdownTimer.Stop()
        $window.Close()
    } else {
        $mins = [int]($script:remainingSec / 60)
        $secs = $script:remainingSec % 60
        $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    }
})

$window.Add_ContentRendered({
    try {
        $hwnd = (New-Object System.Windows.Interop.WindowInteropHelper($window)).Handle
        [WinFlash]::Flash($hwnd)
    } catch {}
    $window.Topmost = $true
    $window.Activate()
    $window.Topmost = $false
    $searchBox.Focus() | Out-Null
    $mins = [int]($script:remainingSec / 60)
    $secs = $script:remainingSec % 60
    $countdownBlock.Text = "${mins}:$($secs.ToString('D2')) to respond"
    $script:countdownTimer.Start()
})
$window.ShowDialog() | Out-Null
Write-Output $script:dialogResult
`

// sanitizePSHereString prevents a value from accidentally closing a PS @'...'@ here-string.
func sanitizePSHereString(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "'@" || trimmed == `"@` {
			lines[i] = line + " "
		}
	}
	return strings.Join(lines, "\n")
}

// runPSTempFile writes a PowerShell script to a temp .ps1 file, executes it, and returns stdout.
func runPSTempFile(script string, timeout time.Duration) (string, error) {
	f, err := os.CreateTemp("", "bmcp-*.ps1")
	if err != nil {
		return "", fmt.Errorf("create temp ps1: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		return "", fmt.Errorf("write temp ps1: %w", err)
	}
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass", "-File", name,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// runPSScriptBg writes a PowerShell script to a temp file and runs it synchronously
// (intended to be called from a goroutine for fire-and-forget behaviour).
func runPSScriptBg(script string) {
	f, err := os.CreateTemp("", "bmcp-*.ps1")
	if err != nil {
		return
	}
	name := f.Name()
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		os.Remove(name)
		return
	}
	f.Close()
	cmd := exec.Command("powershell",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass", "-File", name,
	)
	_ = cmd.Run()
	os.Remove(name)
}

// sendNotificationWindows shows a WPF toast-style popup anchored to the bottom-right
// corner of the screen.  It fades in, stays for durationSec seconds, then fades out.
func sendNotificationWindows(message, title, level string, durationSec int) {
	accents := map[string]string{
		"info":    "#0078D4",
		"warning": "#F7A600",
		"error":   "#D13438",
	}
	accent, ok := accents[level]
	if !ok {
		accent = "#0078D4"
	}
	script := wpfNotifyScriptTmpl
	script = strings.ReplaceAll(script, "{{ACCENT}}", accent)
	script = strings.ReplaceAll(script, "{{TITLE}}", sanitizePSHereString(title))
	script = strings.ReplaceAll(script, "{{MESSAGE}}", sanitizePSHereString(message))
	script = strings.ReplaceAll(script, "{{DURATION_SEC}}", fmt.Sprintf("%d", durationSec))
	runPSScriptBg(script)
}

// buildWPFInputScript returns a fully substituted PowerShell WPF free-form input script.
func buildWPFInputScript(safeQuestion, safeTitle string, timeoutSec int) string {
	s := wpfInputScriptTmpl
	s = strings.ReplaceAll(s, "{{TITLE}}", safeTitle)
	s = strings.ReplaceAll(s, "{{QUESTION}}", safeQuestion)
	s = strings.ReplaceAll(s, "{{TIMEOUT_SEC}}", fmt.Sprintf("%d", timeoutSec))
	return s
}

// buildWPFChoiceScript returns a fully substituted PowerShell WPF list-picker script.
func buildWPFChoiceScript(safeQuestion, safeTitle, choicesJSON string, timeoutSec int) string {
	s := wpfChoiceScriptTmpl
	s = strings.ReplaceAll(s, "{{TITLE}}", safeTitle)
	s = strings.ReplaceAll(s, "{{QUESTION}}", safeQuestion)
	s = strings.ReplaceAll(s, "{{CHOICES_JSON}}", choicesJSON)
	s = strings.ReplaceAll(s, "{{TIMEOUT_SEC}}", fmt.Sprintf("%d", timeoutSec))
	return s
}
