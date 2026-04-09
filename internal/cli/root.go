package cli

import "github.com/spf13/cobra"

// BuildRoot constructs the full cobra command tree.
// version is injected via ldflags in the release binary; pass "dev" from cmd/docgen.
func BuildRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "ding",
		Short: "DING — stream-based alerting daemon",
		Long: `DING is a stream-based alerting daemon.

Pipe metrics in via HTTP POST or stdin. Define rules in a YAML file.
DING evaluates rules and fires alerts in ~4ms. Single binary. No database.
No agents. No cloud account.`,
	}
	root.AddCommand(
		newServeCmd(),
		newValidateCmd(),
		newVersionCmd(version),
	)
	return root
}
