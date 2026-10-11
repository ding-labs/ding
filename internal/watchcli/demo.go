package watchcli

import (
	"context"
	"fmt"
	"github.com/ding-labs/ding/internal/demo"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"time"
)

func demoCommand() *cobra.Command {
	var desktop bool
	cmd := &cobra.Command{Use: "demo", Short: "Run a labeled one-minute local alert demo, then remove its temporary state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer cancel()
		fmt.Fprintln(cmd.OutOrStdout(), "DEMONSTRATION: temporary localhost fixture, not monitoring your service. Healthy → failing → recovered. Your existing watches are untouched.")
		return runDemo(ctx, cmd, desktop, 20*time.Second)
	}}
	cmd.Flags().BoolVar(&desktop, "desktop", false, "request desktop permission and deliver the demo alerts as native notifications")
	return cmd
}

func runDemo(ctx context.Context, cmd *cobra.Command, desktop bool, phase time.Duration) error {
	if err := demo.Run(ctx, demo.Options{Desktop: desktop, Phase: phase, Output: cmd.OutOrStdout()}); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Demo recorded a firing and a recovery. Temporary fixture and watch state were removed. Create a real watch with ding setup or ding watch create.")
	return nil
}
