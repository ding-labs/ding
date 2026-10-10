package watchcli

import (
	"context"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/service"
	"github.com/spf13/cobra"
)

func serviceRepairCommand(dir *string) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "repair", Short: "Preview a narrow repair of this installation's user startup", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := install.Load(*dir)
		if err != nil {
			return err
		}
		m, err := service.New(r)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
		defer cancel()
		status := m.Inspect(ctx)
		if status.State == "conflict" {
			return fmt.Errorf("preserve the conflicting registration and inspect it before repair: %s", status.Advice)
		}
		if !yes {
			return Write(cmd.OutOrStdout(), map[string]any{"service": status, "proposedAction": "Ensure the exact owned user-startup definition is registered, start it, and wait for authenticated readiness.", "preserved": "Watch state, credentials, grants, transfer holds and existing service identities.", "apply": "ding service repair --yes (pass the same --state-dir)"})
		}
		probe, stop := context.WithTimeout(ctx, time.Second)
		_, live := daemonHealth(probe, *dir)
		stop()
		if live == nil && !status.Installed {
			return fmt.Errorf("a manually managed daemon is live; stop its supervisor before enabling user startup")
		}
		if err := m.Install(ctx); err != nil {
			return err
		}
		if err := m.Action(ctx, "start"); err != nil {
			return err
		}
		if err := waitForDaemon(ctx, *dir); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Owned startup repaired; daemon is ready. Watch state and grants were retained.")
		return nil
	}}
	cmd.Flags().BoolVar(&yes, "yes", false, "apply the described registration/start repair; never replace a conflicting definition")
	return cmd
}
