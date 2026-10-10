package watchcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/service"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/spf13/cobra"
)

type localStatus struct {
	Version      string           `json:"version"`
	StateDir     string           `json:"stateDir"`
	Daemon       string           `json:"daemon"`
	Advice       string           `json:"advice,omitempty"`
	Installation *install.Record  `json:"installation,omitempty"`
	Service      *service.Status  `json:"service,omitempty"`
	Health       *watchrun.Doctor `json:"health,omitempty"`
}

func daemonHealth(ctx context.Context, dir string) (*watchrun.Doctor, error) {
	c, err := control.Connect(dir)
	if err != nil {
		return nil, err
	}
	raw, err := c.Call(ctx, "GET", "/v1/doctor", nil)
	if err != nil {
		return nil, err
	}
	var d watchrun.Doctor
	if err = json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func readLocalStatus(ctx context.Context, dir, version string) localStatus {
	s := localStatus{Version: version, StateDir: dir, Daemon: "unavailable"}
	r, err := install.Load(dir)
	if err == nil {
		s.Installation = &r
		if m, e := service.New(r); e == nil {
			state := m.Inspect(ctx)
			s.Service = &state
		} else {
			s.Advice = e.Error()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		s.Advice = err.Error()
	}
	d, err := daemonHealth(ctx, dir)
	if err != nil {
		if s.Advice == "" {
			s.Advice = "Run ding setup, or ding service start for an installed service. " + err.Error()
		}
		return s
	}
	s.Health = d
	s.Daemon = "ready"
	if !d.Running || d.Closing {
		s.Daemon = "starting-or-stopping"
	}
	return s
}

func serviceCommands(root *cobra.Command, dir *string) {
	var structured bool
	status := &cobra.Command{Use: "status", Short: "Show service and monitoring status, even when the daemon is offline", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		s := readLocalStatus(ctx, *dir, root.Version)
		if structured {
			return Write(cmd.OutOrStdout(), s)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Ding %s — daemon %s\nState: %s\n", s.Version, s.Daemon, s.StateDir)
		if s.Service != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Background service: %s (startup: %s)\n", s.Service.State, s.Service.Startup)
		}
		if s.Health != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Watches: %d; delivery states: %v\n", len(s.Health.Sources), s.Health.Deliveries)
			for _, w := range s.Health.Sources {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s; last input %s; source error %q\n", w.ID, w.Status, w.LastInputAt, w.LastError)
			}
		}
		if s.Advice != "" {
			fmt.Fprintln(cmd.OutOrStdout(), s.Advice)
		}
		if s.Service != nil && s.Service.Advice != "" {
			fmt.Fprintln(cmd.OutOrStdout(), s.Service.Advice)
		}
		return nil
	}}
	status.Flags().BoolVar(&structured, "json", false, "emit a versioned status response")
	root.AddCommand(status)
	group := &cobra.Command{Use: "service", Short: "Manage this user's background Ding service"}
	for _, action := range []string{"install", "start", "stop", "restart", "status", "uninstall"} {
		command := &cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
			defer cancel()
			r, err := install.Load(*dir)
			if action == "install" && errors.Is(err, os.ErrNotExist) {
				r, err = recordInstallation(*dir, root.Version)
			}
			if err != nil {
				return err
			}
			m, err := service.New(r)
			if err != nil {
				return err
			}
			switch action {
			case "install":
				err = m.Install(ctx)
			case "uninstall":
				err = m.Uninstall(ctx)
			case "status":
				return present(cmd, true, m.Inspect(ctx))
			default:
				err = m.Action(ctx, action)
			}
			if err != nil {
				return err
			}
			if action == "start" || action == "restart" {
				if err = waitForDaemon(ctx, *dir); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Service %s completed. Watch state is retained.\n", action)
			return nil
		}}
		group.AddCommand(command)
	}
	root.AddCommand(group, setupCommand(root, dir))
}

func recordInstallation(dir, version string) (install.Record, error) {
	executable, owner, err := service.StableExecutable()
	if err != nil {
		return install.Record{}, err
	}
	r, err := install.Inspect(executable, dir, version, owner)
	if err != nil {
		return r, err
	}
	if err = install.Create(r); err != nil {
		return r, err
	}
	return install.Load(dir)
}

func waitForDaemon(ctx context.Context, dir string) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		probe, cancel := context.WithTimeout(ctx, time.Second)
		d, err := daemonHealth(probe, dir)
		cancel()
		if err == nil && d.Running && !d.Closing {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("daemon did not become ready; run ding status: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func setupCommand(root *cobra.Command, dir *string) *cobra.Command {
	var yes, headless bool
	cmd := &cobra.Command{Use: "setup", Short: "Start account-free Ding with automatic background startup", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "Set up Ding in %s.\nThe background service starts at login and continues when the terminal or AI client closes.\nThis computer must stay awake and connected to monitor watches. No Ding account is needed.\n", *dir)
		if !yes {
			fmt.Fprint(cmd.OutOrStdout(), "Enable background startup? [y/N] ")
			answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			if err != nil || !strings.EqualFold(strings.TrimSpace(answer), "y") {
				return fmt.Errorf("setup canceled; use --yes for explicit noninteractive setup")
			}
		}
		r, err := install.Load(*dir)
		if errors.Is(err, os.ErrNotExist) {
			r, err = recordInstallation(*dir, root.Version)
		}
		if err != nil {
			return err
		}
		m, err := service.New(r)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
		defer cancel()
		probe, stop := context.WithTimeout(ctx, time.Second)
		_, live := daemonHealth(probe, *dir)
		stop()
		if live == nil && !m.Inspect(ctx).Installed {
			return fmt.Errorf("a manually started daemon is already using this state; stop it before enabling managed startup")
		}
		if err = m.Install(ctx); err != nil {
			return err
		}
		if err = m.Action(ctx, "start"); err != nil {
			return err
		}
		if err = waitForDaemon(ctx, *dir); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Ding is ready. Use ding status to inspect monitoring and ding mcp setup to connect an AI client.")
		if headless {
			return nil
		}
		ui := uiCommand(dir)
		ui.SetOut(cmd.OutOrStdout())
		ui.SetErr(cmd.ErrOrStderr())
		ui.SetContext(cmd.Context())
		return ui.RunE(ui, nil)
	}}
	cmd.Flags().BoolVar(&yes, "yes", false, "enable the described background service without an interactive prompt")
	cmd.Flags().BoolVar(&headless, "headless", false, "set up the service without opening a browser")
	return cmd
}
