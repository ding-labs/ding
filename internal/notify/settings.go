package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/ding-labs/ding/internal/nativehelper"
)

type Failure struct{ Code string }

func (e Failure) Error() string {
	switch e.Code {
	case "notification_permission_denied":
		return "Ding notifications are turned off. Open notification settings, enable Ding, then send another test."
	case "notification_permission_pending":
		return "Notification permission is still waiting for a response. Look for Ding's permission prompt or open Notification Center, then try again."
	default:
		return "Ding's notification helper could not complete the request. This is not a confirmed permission denial; check notification status or repair the installation."
	}
}

type Settings struct {
	Platform        string `json:"platform"`
	AppName         string `json:"appName"`
	Authorization   string `json:"authorization"`
	Alerts          string `json:"alerts"`
	CanOpenSettings bool   `json:"canOpenSettings"`
}

func Status(ctx context.Context) (Settings, error) {
	s := Settings{Platform: runtime.GOOS, AppName: "Ding", Authorization: "unknown", Alerts: "unknown", CanOpenSettings: runtime.GOOS == "darwin"}
	if runtime.GOOS != "darwin" {
		return s, nil
	}
	path, err := nativehelper.Path()
	if err != nil {
		return s, err
	}
	cmd := exec.CommandContext(ctx, path, "--status")
	var out limitedOutput
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return s, Failure{Code: "notification_helper_failed"}
	}
	if err := json.Unmarshal(out.Bytes(), &s); err != nil {
		return s, Failure{Code: "notification_helper_failed"}
	}
	return s, nil
}

func OpenSettings(ctx context.Context) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("open your operating system's notification settings")
	}
	return exec.CommandContext(ctx, "/usr/bin/open", "x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=ing.ding.notifications").Run()
}

// Drain helper output while bounding memory. Never forward raw helper diagnostics,
// which can contain local paths or user notification text.
type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 16384 - b.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
