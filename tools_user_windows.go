package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// wpfNotifyScriptTmpl is a PowerShell WPF script that renders a themed toast-style
// popup in the bottom-right corner. Placeholders: {{ACCENT}}, {{TITLE}}, {{MESSAGE}}.
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

// runPSScriptBg writes a PowerShell script to a temp file and runs it synchronously
// (intended to be called from a goroutine for fire-and-forget behaviour).
func runPSScriptBg(script string) {
	f, err := os.CreateTemp("", "bmcp-*.ps1")
	if err != nil {
		return
	}
	name := f.Name()
	// Write UTF-8 BOM so PowerShell 5.1 reads the file as UTF-8.
	const utf8BOM = "\xef\xbb\xbf"
	if _, err := f.WriteString(utf8BOM + script); err != nil {
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

// sendNotificationWindows shows a WPF toast-style popup anchored to the bottom-right corner.
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
