package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"golang.org/x/mod/semver"
)

type Settings struct {
	Checks    bool `json:"checks"`
	Automatic bool `json:"automatic"`
	HourUTC   int  `json:"hourUTC"`
}
type CheckStatus struct {
	CheckedAt  time.Time `json:"checkedAt"`
	Current    string    `json:"current"`
	Available  string    `json:"available,omitempty"`
	Compatible bool      `json:"compatible"`
	Error      string    `json:"error,omitempty"`
}

func LoadSettings(dir string) (Settings, error) {
	s := Settings{Checks: true, HourUTC: 3}
	err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, "updates.json"), &s, true)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err == nil && (s.HourUTC < 0 || s.HourUTC > 23 || s.Automatic && !s.Checks) {
		err = fmt.Errorf("invalid update settings: choose hour 0–23 UTC; automatic updates require checks")
	}
	return s, err
}
func ReadCheck(dir string) (CheckStatus, error) {
	var s CheckStatus
	err := mcpconfig.ReadPrivateJSON(filepath.Join(dir, "update-status.json"), &s, true)
	return s, err
}

func CheckOnce(ctx context.Context, dir string, schema int, force bool) (CheckStatus, error) {
	var result CheckStatus
	settings, err := LoadSettings(dir)
	if err != nil {
		return result, err
	}
	if !settings.Checks && !force {
		return result, nil
	}
	r, err := install.Load(dir)
	if err != nil {
		return result, err
	}
	// Package managers own their update checks and trust roots. A Homebrew build
	// deliberately has no standalone updater key; that is not a broken build.
	if r.Owner != "standalone" {
		return CheckStatus{Current: r.Version}, nil
	}
	if PublicKey == "" || r.Channel == "development" {
		return CheckStatus{Current: r.Version, Error: "Updates are not configured in this development build."}, nil
	}
	now := time.Now().UTC()
	previous, err := ReadCheck(dir)
	if err == nil && !force && now.Sub(previous.CheckedAt) >= 0 && now.Sub(previous.CheckedAt) < 24*time.Hour && previous.Current == r.Version {
		return previous, nil
	}
	result = CheckStatus{CheckedAt: now, Current: r.Version}
	manifest, checkErr := (Client{}).Check(ctx, PublicKey, r.Channel, now)
	if checkErr != nil {
		result.Error = checkErr.Error()
	} else if semver.Compare(manifest.Version, "v"+strings.TrimPrefix(r.Version, "v")) > 0 {
		result.Available = manifest.Version
		_, err := manifest.Select(runtime.GOOS, runtime.GOARCH, r.Version, schema)
		result.Compatible = err == nil
		if err != nil {
			result.Error = err.Error()
		}
	}
	unlock, err := install.Lock(dir)
	if err != nil {
		return result, err
	}
	defer unlock()
	current, err := install.Load(dir)
	if err != nil {
		return result, err
	}
	if current.Version != r.Version {
		return result, nil
	}
	return result, install.AtomicJSON(filepath.Join(dir, "update-status.json"), result)
}

// WatchChecks never installs anything. Default checks run at most daily and can
// be disabled without disabling the daemon, local MCP, or offline operation.
func WatchChecks(ctx context.Context, dir string, schema int) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			check, stop := context.WithTimeout(ctx, 45*time.Second)
			_, _ = CheckOnce(check, dir, schema, false)
			stop()
			timer.Reset(time.Hour)
		}
	}()
	return func() { cancel(); <-done }
}
