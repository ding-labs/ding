package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

type UpdateJob struct {
	Manager   Manager
	companion *Manager
}

func NewUpdateJob(r install.Record) (UpdateJob, error) {
	base, err := New(r)
	if err != nil {
		return UpdateJob{}, err
	}
	d := base.Definition
	args := []string{r.Executable, "update", "scheduled", "--state-dir", r.StateDir}
	switch base.OS {
	case "darwin":
		d.Name += ".updates"
		d.Path = filepath.Join(filepath.Dir(d.Path), d.Name+".plist")
		var values strings.Builder
		for _, arg := range args {
			values.WriteString("<string>" + xmlText(arg) + "</string>")
		}
		d.Content = `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Label</key><string>` + d.Name + `</string><key>ProgramArguments</key><array>` + values.String() + `</array><key>StartInterval</key><integer>3600</integer><key>ProcessType</key><string>Background</string></dict></plist>`
		base.Definition = d
		return UpdateJob{Manager: base}, nil
	case "linux":
		name := strings.TrimSuffix(d.Name, ".service") + ".updates"
		for i, arg := range args {
			args[i] = systemdQuote(arg)
		}
		companion := base
		companion.Definition = Definition{Name: name + ".service", Path: filepath.Join(filepath.Dir(d.Path), name+".service"), Startup: "maintenance", Content: "[Unit]\nDescription=Ding opted-in compatible update\n[Service]\nType=oneshot\nExecStart=" + strings.Join(args, " ") + "\nTimeoutStartSec=5min\nUMask=0077\n"}
		d.Name = name + ".timer"
		d.Path = filepath.Join(filepath.Dir(d.Path), d.Name)
		d.Content = "[Unit]\nDescription=Ding opted-in update window check\n[Timer]\nOnCalendar=hourly\nRandomizedDelaySec=300\nPersistent=true\n[Install]\nWantedBy=timers.target\n"
		base.Definition = d
		return UpdateJob{Manager: base, companion: &companion}, nil
	default:
		return UpdateJob{}, fmt.Errorf("automatic updates are available for standalone macOS and Linux installs; use the signed installer on Windows")
	}
}
func (j UpdateJob) Install(ctx context.Context) error {
	if j.companion != nil {
		owned, err := j.companion.owned()
		if err != nil {
			return err
		}
		if !owned {
			f, err := mcpconfig.CreatePrivate(j.companion.Definition.Path)
			if err != nil {
				return err
			}
			_, err = f.WriteString(j.companion.Definition.Content)
			closed := f.Close()
			if err != nil {
				return err
			}
			if closed != nil {
				return closed
			}
		}
	}
	if err := j.Manager.Install(ctx); err != nil {
		return err
	}
	return j.Manager.Action(ctx, "start")
}
func (j UpdateJob) Uninstall(ctx context.Context) error {
	if err := j.Manager.Uninstall(ctx); err != nil {
		return err
	}
	if j.companion != nil {
		owned, err := j.companion.owned()
		if err != nil {
			return err
		}
		if owned {
			if err := os.Remove(j.companion.Definition.Path); err != nil {
				return err
			}
			return j.Manager.command(ctx, "systemctl", "--user", "daemon-reload")
		}
	}
	return nil
}
