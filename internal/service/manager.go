package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/store"
)

type Runner func(context.Context, string, ...string) ([]byte, error)
type Manager struct {
	OS, UID    string
	StateDir   string
	Definition Definition
	Run        Runner
}
type Status struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Startup   string `json:"startup"`
	Installed bool   `json:"installed"`
	State     string `json:"state"`
	Advice    string `json:"advice,omitempty"`
}

func New(r install.Record) (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Manager{}, err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return Manager{}, err
	}
	u, err := user.Current()
	if err != nil {
		return Manager{}, err
	}
	d, err := DefinitionFor(runtime.GOOS, home, config, u.Uid, r)
	return Manager{OS: runtime.GOOS, UID: u.Uid, StateDir: r.StateDir, Definition: d, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}}, err
}

func (m Manager) owned() (bool, error) {
	info, err := os.Lstat(m.Definition.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return false, fmt.Errorf("service definition is not a regular Ding-owned file")
	}
	b, err := os.ReadFile(m.Definition.Path)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(b, []byte(m.Definition.Content)) {
		return false, fmt.Errorf("service definition differs from this installation; preserve and inspect %s", m.Definition.Path)
	}
	return true, nil
}

func (m Manager) Inspect(ctx context.Context) Status {
	d := m.Definition
	s := Status{Name: d.Name, Path: d.Path, Startup: d.Startup, State: "not-installed"}
	owned, err := m.owned()
	if err != nil {
		s.State = "conflict"
		s.Advice = err.Error()
		return s
	}
	if !owned {
		s.Advice = "Run ding setup to enable background startup."
		return s
	}
	s.Installed = true
	var out []byte
	switch m.OS {
	case "darwin":
		out, err = m.Run(ctx, "launchctl", "print", "gui/"+m.UID+"/"+d.Name)
	case "linux":
		out, err = m.Run(ctx, "systemctl", "--user", "is-active", d.Name)
	case "windows":
		// Query XML avoids locale-dependent CSV columns. Readiness is established
		// by the authenticated daemon probe, never by registration alone.
		out, err = m.Run(ctx, "schtasks", "/Query", "/TN", d.Name, "/XML")
	}
	if m.OS == "windows" && err == nil && !sameTask(out, d.Content) {
		s.State = "conflict"
		s.Advice = "Registered task differs from this installation; inspect it in Task Scheduler."
		return s
	}
	s.State = "registered"
	if err != nil {
		s.State = "unavailable"
		s.Advice = "Service is not active or its session manager is unavailable; run ding service start in the owning user session."
	}
	text := string(out)
	if m.OS == "linux" && (strings.TrimSpace(text) == "inactive" || strings.TrimSpace(text) == "failed") {
		s.State = strings.TrimSpace(text)
	}
	if err == nil && ((m.OS == "darwin" && strings.Contains(text, "state = running")) || (m.OS == "linux" && strings.TrimSpace(text) == "active")) {
		s.State = "running"
	}
	return s
}

func (m Manager) Install(ctx context.Context) error {
	owned, err := m.owned()
	if err != nil {
		return err
	}
	if !owned {
		f, err := mcpconfig.CreatePrivate(m.Definition.Path)
		if err != nil {
			return err
		}
		_, writeErr := f.WriteString(m.Definition.Content)
		if writeErr == nil {
			writeErr = f.Sync()
		}
		if err := errors.Join(writeErr, f.Close()); err != nil {
			return err
		}
	}
	switch m.OS {
	case "linux":
		if err := m.command(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
			return err
		}
		return m.command(ctx, "systemctl", "--user", "enable", m.Definition.Name)
	case "windows":
		if owned {
			if _, err := m.Run(ctx, "schtasks", "/Query", "/TN", m.Definition.Name, "/XML"); err == nil {
				return m.checkRegisteredTask(ctx)
			}
		}
		return m.command(ctx, "schtasks", "/Create", "/TN", m.Definition.Name, "/XML", m.Definition.Path)
	}
	return nil
}

func (m Manager) Action(ctx context.Context, action string) error {
	owned, err := m.owned()
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("service not installed; run ding setup")
	}
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("unsupported service action")
	}
	d := m.Definition
	switch m.OS {
	case "darwin":
		target := "gui/" + m.UID + "/" + d.Name
		_, loadedErr := m.Run(ctx, "launchctl", "print", target)
		if loadedErr != nil && !hasExitCode(loadedErr, 113) {
			return fmt.Errorf("cannot inspect launchd service: %w", loadedErr)
		}
		if action == "stop" {
			if loadedErr != nil {
				return nil
			}
			return m.bootout(ctx, target)
		}
		if loadedErr != nil {
			return m.bootstrap(ctx)
		}
		if action == "restart" {
			// bootout requests graceful termination; kickstart -k would SIGKILL.
			if err := m.bootout(ctx, target); err != nil {
				return err
			}
			return m.bootstrap(ctx)
		}
		return m.command(ctx, "launchctl", "kickstart", target)
	case "linux":
		return m.command(ctx, "systemctl", "--user", action, d.Name)
	case "windows":
		if err := m.checkRegisteredTask(ctx); err != nil {
			return err
		}
		if action == "stop" || action == "restart" {
			if m.StateDir == "" {
				return fmt.Errorf("missing owned state directory")
			}
			if c, err := control.Connect(m.StateDir); err == nil {
				_, _ = c.Call(ctx, "POST", "/v1/local/shutdown", nil)
			}
			if err := store.WaitWriterStopped(ctx, m.StateDir); err != nil {
				return err
			}
		}
		// The writer has drained; end only the residual task process before restart.
		if action == "stop" || action == "restart" {
			_, _ = m.Run(ctx, "schtasks", "/End", "/TN", d.Name)
		}
		if action != "stop" {
			return m.command(ctx, "schtasks", "/Run", "/TN", d.Name)
		}
		return nil
	}
	return fmt.Errorf("unsupported service manager")
}

func hasExitCode(err error, code int) bool {
	var exited interface{ ExitCode() int }
	return errors.As(err, &exited) && exited.ExitCode() == code
}

func (m Manager) bootout(ctx context.Context, target string) error {
	if err := m.command(ctx, "launchctl", "bootout", target); err != nil {
		return err
	}
	for {
		_, err := m.Run(ctx, "launchctl", "print", target)
		if hasExitCode(err, 113) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m Manager) bootstrap(ctx context.Context) error {
	// launchd can remove a label before its teardown finishes. Retry only its
	// observed transient bootstrap I/O error, with a short bounded budget.
	var last error
	for attempt := 0; attempt < 20; attempt++ {
		_, last = m.Run(ctx, "launchctl", "bootstrap", "gui/"+m.UID, m.Definition.Path)
		if last == nil {
			return nil
		}
		if !hasExitCode(last, 5) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("launchd could not start the owned service: %w", last)
}

func (m Manager) Uninstall(ctx context.Context) error {
	owned, err := m.owned()
	if err != nil {
		return err
	}
	if !owned {
		return nil
	}
	switch m.OS {
	case "linux":
		err = m.command(ctx, "systemctl", "--user", "disable", "--now", m.Definition.Name)
	case "windows":
		// Task removal alone does not stop an already running executable.
		if err = m.Action(ctx, "stop"); err == nil {
			err = m.command(ctx, "schtasks", "/Delete", "/TN", m.Definition.Name, "/F")
		}
	case "darwin":
		err = m.Action(ctx, "stop")
	}
	if err != nil {
		return err
	}
	if err = os.Remove(m.Definition.Path); err != nil {
		return err
	}
	if m.OS == "linux" {
		return m.command(ctx, "systemctl", "--user", "daemon-reload")
	}
	return nil
}

func (m Manager) command(ctx context.Context, name string, args ...string) error {
	if _, err := m.Run(ctx, name, args...); err != nil {
		return fmt.Errorf("%s %s failed: %w; inspect the service with its native manager", name, strings.Join(args, " "), err)
	}
	return nil
}

// StableExecutable preserves package-manager symlinks only when they resolve to
// the executable actually running. An unrelated PATH entry cannot take ownership.
func StableExecutable() (string, string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", "", err
	}
	owner := "standalone"
	if strings.Contains(filepath.ToSlash(resolved), "/Cellar/ding/") {
		owner = "homebrew"
	} else if marker, err := os.ReadFile(filepath.Join(filepath.Dir(resolved), "installation-owner")); err == nil && string(marker) == "external\n" {
		owner = "external"
	}
	if path, e := exec.LookPath("ding"); e == nil {
		if real, e := filepath.EvalSymlinks(path); e == nil && real == resolved {
			current, _ = filepath.Abs(path)
		}
	}
	return current, owner, nil
}
