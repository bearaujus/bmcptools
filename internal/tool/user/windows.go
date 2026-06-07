package user

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	notifyEnvAccent   = "BMCP_NOTIFY_ACCENT"
	notifyEnvDuration = "BMCP_NOTIFY_DURATION_SEC"
	notifyEnvMessage  = "BMCP_NOTIFY_MESSAGE"
	notifyEnvTitle    = "BMCP_NOTIFY_TITLE"
)

const wpfNotifyScript = `
Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase
$isDark = $false
try {
    $val = Get-ItemPropertyValue 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize' -Name 'AppsUseLightTheme' -ErrorAction Stop
    $isDark = ($val -eq 0)
} catch {}
$bg      = if ($isDark) { '#2D2D2D' } else { '#FFFFFF' }
$fg      = if ($isDark) { '#E0E0E0' } else { '#1A1A1A' }
$titleFg = if ($isDark) { '#FFFFFF' } else { '#000000' }
$accent  = [Environment]::GetEnvironmentVariable('BMCP_NOTIFY_ACCENT')
[int]$durationSec = 5
[void][int]::TryParse([Environment]::GetEnvironmentVariable('BMCP_NOTIFY_DURATION_SEC'), [ref]$durationSec)
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
$window.FindName('TitleBlock').Text = [Environment]::GetEnvironmentVariable('BMCP_NOTIFY_TITLE')
$window.FindName('MsgBlock').Text = [Environment]::GetEnvironmentVariable('BMCP_NOTIFY_MESSAGE')
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
    $script:toastTimer.Interval = [TimeSpan]::FromSeconds($durationSec)
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

func runPSScriptBg(script string, env map[string]string) {
	f, err := os.CreateTemp("", "bmcp-*.ps1")
	if err != nil {
		return
	}
	name := f.Name()
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
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	_ = cmd.Run()
	os.Remove(name)
}

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
	runPSScriptBg(wpfNotifyScript, map[string]string{
		notifyEnvAccent:   accent,
		notifyEnvDuration: fmt.Sprintf("%d", durationSec),
		notifyEnvMessage:  message,
		notifyEnvTitle:    title,
	})
}
