// Package notify delivers bounded text to the current user's desktop session.
// Acceptance means the OS accepted a notification, never that a person read it.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ding-labs/ding/internal/nativehelper"
	"github.com/ding-labs/ding/internal/notificationstyle"
	"github.com/godbus/dbus/v5"
)

type Message struct {
	Status            notificationstyle.Status `json:"status,omitempty"`
	WatchID           string                   `json:"watchId,omitempty"`
	StateDir          string                   `json:"stateDir,omitempty"`
	ID                string                   `json:"id"`
	Title             string                   `json:"title"`
	Body              string                   `json:"body"`
	RequestPermission bool                     `json:"requestPermission"`
	Activation        string                   `json:"activation,omitempty"`
}

func validate(m Message) error {
	if m.ID == "" || len(m.ID) > 256 || len(m.Title) > 512 || len(m.Body) > 4096 || strings.ContainsRune(m.Title+m.Body, '\x00') || (m.Status != "" && m.Status.IconColor() == "") {
		return fmt.Errorf("invalid notification")
	}
	return nil
}

func Send(ctx context.Context, m Message) error {
	if err := validate(m); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
		if address == "" {
			address = fmt.Sprintf("unix:path=/run/user/%d/bus", os.Getuid())
		}
		conn, err := dbus.Connect(address, dbus.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("desktop session unavailable")
		}
		defer conn.Close()
		var id uint32
		err = conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(ctx, "org.freedesktop.Notifications.Notify", 0, "Ding", uint32(0), "", m.Title, html.EscapeString(m.Body), []string{}, imageHints(m.Status), int32(-1)).Store(&id)
		if err != nil || id == 0 {
			return fmt.Errorf("desktop notification was not accepted")
		}
		return nil
	case "darwin":
		path, err := nativehelper.Path()
		if err != nil {
			return err
		}
		return helper(ctx, path, []string{"--deliver"}, m)
	case "windows":
		m.Activation = ActivationURL(Target{m.StateDir, m.WatchID})
		if m.Activation == "" {
			// Disposable demos have no persistent Console instance to open.
			m.Activation = "ding-watch://dismiss"
		}
		path := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		return helper(ctx, path, []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", windowsScript}, m)
	default:
		return fmt.Errorf("desktop notifications are unavailable on this platform")
	}
}

func helper(ctx context.Context, path string, args []string, m Message) error {
	var payload any = m
	if runtime.GOOS == "windows" {
		// The local file path is added at delivery time, never pinned from source
		// fields or fetched from a notification-supplied URL.
		payload = struct {
			Message
			IconPath string `json:"iconPath,omitempty"`
		}{m, notificationIconPath(m.Status)}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = bytes.NewReader(data)
	var out limitedOutput
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && m.RequestPermission {
			return Failure{Code: "notification_permission_pending"}
		}
		var reply struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(out.Bytes(), &reply) == nil {
			switch reply.Error {
			case "notification_permission_denied", "notification_permission_pending":
				return Failure{Code: reply.Error}
			}
		}
		return Failure{Code: "notification_helper_failed"}
	}
	return nil
}

// All user content arrives as JSON on stdin and is inserted as XML text nodes.
// No notification content is executed as PowerShell or included in its arguments.
const windowsScript = `$ErrorActionPreference = 'Stop'
$m = [Console]::In.ReadToEnd() | ConvertFrom-Json
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType=WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType=WindowsRuntime] | Out-Null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml('<toast><visual><binding template="ToastGeneric"><text/><text/></binding></visual></toast>')
if ($m.activation) {
  $xml.DocumentElement.SetAttribute('activationType', 'protocol')
  $xml.DocumentElement.SetAttribute('launch', [string]$m.activation)
}
$nodes = $xml.GetElementsByTagName('text')
$nodes.Item(0).AppendChild($xml.CreateTextNode([string]$m.title)) | Out-Null
$nodes.Item(1).AppendChild($xml.CreateTextNode([string]$m.body)) | Out-Null
if ($m.iconPath) {
  $image = $xml.CreateElement('image')
  $image.SetAttribute('placement', 'appLogoOverride')
  $image.SetAttribute('src', ([System.Uri]::new([string]$m.iconPath)).AbsoluteUri)
  $image.SetAttribute('alt', 'Ding: ' + [string]$m.status)
  $xml.GetElementsByTagName('binding').Item(0).AppendChild($image) | Out-Null
}
$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Ding')
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
$notifier.Show($toast)
`
