package push

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// LocalNotifier defines the interface for triggering notifications on the local host system.
type LocalNotifier interface {
	Notify(ctx context.Context, content string) error
	NotifyAsync(content string)
	IsEnabled() bool
}

// LocalSystemNotifier implements LocalNotifier displaying only the notification content.
type LocalSystemNotifier struct {
	enabled bool
	logger  *slog.Logger
	timeout time.Duration
}

// NewLocalSystemNotifier creates a new LocalSystemNotifier.
func NewLocalSystemNotifier(logger *slog.Logger, enabled bool) *LocalSystemNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &LocalSystemNotifier{
		enabled: enabled,
		logger:  logger,
		timeout: 2000 * time.Second,
	}
}

// IsEnabled reports whether local notifications are active.
func (n *LocalSystemNotifier) IsEnabled() bool {
	return n != nil && n.enabled
}

// Notify triggers a desktop notification synchronously on the local system showing only the content.
func (n *LocalSystemNotifier) Notify(ctx context.Context, content string) error {
	if !n.IsEnabled() {
		return nil
	}

	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		trimmedContent = "You have a new notification."
	}

	execCtx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = buildWindowsNotificationCmd(execCtx, trimmedContent)
	case "darwin":
		cmd = buildDarwinNotificationCmd(execCtx, trimmedContent)
	default: // Linux / Unix
		cmd = buildLinuxNotificationCmd(execCtx, trimmedContent)
	}

	if cmd == nil {
		n.logger.DebugContext(ctx, "Local notification unsupported on this platform", "goos", runtime.GOOS)
		return nil
	}

	if err := cmd.Run(); err != nil {
		n.logger.WarnContext(ctx, "Failed to display local system push notification",
			"error", err,
			"platform", runtime.GOOS,
			"content", trimmedContent,
		)
		return err
	}

	n.logger.InfoContext(ctx, "[LOCAL_PUSH_NOTIFICATION] Displayed notification on local system (content-only)",
		"content", trimmedContent,
		"platform", runtime.GOOS,
	)
	return nil
}

// NotifyAsync runs Notify in a background goroutine so it never blocks HTTP/gRPC requests or DB writes.
func (n *LocalSystemNotifier) NotifyAsync(content string) {
	if !n.IsEnabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
		defer cancel()
		_ = n.Notify(ctx, content)
	}()
}

// encodePowerShellCommand converts a PowerShell script to UTF-16LE Base64 for -EncodedCommand.
func encodePowerShellCommand(script string) string {
	runes := utf16.Encode([]rune(script))
	b := make([]byte, len(runes)*2)
	for i, r := range runes {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func buildWindowsNotificationCmd(ctx context.Context, content string) *exec.Cmd {
	escapedContent := strings.ReplaceAll(content, "'", "''")

	xmlContent := strings.ReplaceAll(content, "&", "&amp;")
	xmlContent = strings.ReplaceAll(xmlContent, "<", "&lt;")
	xmlContent = strings.ReplaceAll(xmlContent, ">", "&gt;")
	xmlContent = strings.ReplaceAll(xmlContent, `"`, `&quot;`)
	xmlContent = strings.ReplaceAll(xmlContent, "'", "&apos;")

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName PresentationFramework -ErrorAction SilentlyContinue
$msg = '%s'
$xaml = '<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation" Title="GoCart" Height="105" Width="420" WindowStyle="None" AllowsTransparency="True" Background="Transparent" Topmost="True" ShowInTaskbar="False" WindowStartupLocation="Manual"><Border Background="#FFFFFF" BorderBrush="#00A651" BorderThickness="2" CornerRadius="12" Padding="14,12,14,12"><StackPanel VerticalAlignment="Center"><StackPanel Orientation="Horizontal" Margin="0,0,0,6"><TextBlock FontSize="14" FontWeight="Bold"><Run Text="go" Foreground="#00A651"/><Run Text="cart" Foreground="#1E293B"/><Run Text="." Foreground="#00A651"/></TextBlock><TextBlock Text=" • Notification" Foreground="#64748B" FontSize="11.5" VerticalAlignment="Center" Margin="6,0,0,0"/></StackPanel><TextBlock Text="%s" Foreground="#0F172A" FontSize="12.5" TextWrapping="Wrap" LineHeight="17"/></StackPanel></Border></Window>'
try {
    $reader = [System.Xml.XmlReader]::Create([System.IO.StringReader]::new($xaml))
    $window = [System.Windows.Markup.XamlReader]::Load($reader)
    $workArea = [System.Windows.SystemParameters]::WorkArea
    $window.Left = $workArea.Right - $window.Width - 20
    $window.Top = $workArea.Bottom - $window.Height - 20
    $timer = New-Object System.Windows.Threading.DispatcherTimer
    $timer.Interval = [TimeSpan]::FromSeconds(4)
    $timer.Add_Tick({ $window.Close(); $timer.Stop() })
    $timer.Start()
    $window.ShowDialog() | Out-Null
} catch {
    (New-Object -ComObject Wscript.Shell).Popup($msg, 3, 'GoCart Notification', 64) | Out-Null
}
`, escapedContent, xmlContent)

	encoded := encodePowerShellCommand(psScript)
	return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-EncodedCommand", encoded)
}

func buildDarwinNotificationCmd(ctx context.Context, content string) *exec.Cmd {
	escapedContent := strings.ReplaceAll(content, `"`, `\"`)
	script := fmt.Sprintf(`display notification "%s"`, escapedContent)
	return exec.CommandContext(ctx, "osascript", "-e", script)
}

func buildLinuxNotificationCmd(ctx context.Context, content string) *exec.Cmd {
	return exec.CommandContext(ctx, "notify-send", "", content)
}
