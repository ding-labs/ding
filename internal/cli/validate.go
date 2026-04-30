package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ding-labs/ding/internal/server"
)

func newValidateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the config file",
		Long: `Validate the DING config file without starting the daemon.

Checks YAML syntax, required fields, rule condition grammar, notifier
configuration, and referenced notifier names. Exits 0 if valid, 1 with
a descriptive error if not.

Use in CI pipelines to catch config errors before deployment.`,
		Example: `  ding validate
  ding validate --config /etc/myapp/alerts.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(configPath)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "ding.yaml", "path to config file")
	return cmd
}

func runValidate(configPath string) error {
	// Pass nil collector so validate does not open the alert log file as a side effect.
	_, _, _, _, _, err := server.BuildFromConfig(configPath, nil)
	if err != nil {
		return fmt.Errorf("config invalid: %w", err)
	}
	fmt.Println("config OK:", configPath)
	return nil
}
