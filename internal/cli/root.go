package cli

import "github.com/spf13/cobra"

// BuildRoot constructs the full cobra command tree.
// version is injected via ldflags in the release binary; pass "dev" from cmd/docgen.
func BuildRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "ding",
		Short: "DING — alerting that ships with the workload",
		Long: `DING is alerting that ships with the workload.

Drop it into your CI job, your ML training run, your batch pipeline, or your
game server. The job emits events; DING evaluates rules in-process; alerts
fire during the run and a summary fires when the job exits. Both die together.

Single binary. No agents. No cloud account. ~4ms alert latency.

Two modes:

  ding run -- <cmd>     Wrap a command; alert on the events it emits and on
                        job-level outcomes (exit code, duration, summaries).

  ding serve            Long-running HTTP alerting daemon for persistent
                        services and fleet-wide push-based alerting.`,
	}
	root.AddCommand(
		newServeCmd(),
		newRunCmd(),
		newValidateCmd(),
		newVersionCmd(version),
		newInstallCmd(),
	)
	return root
}
