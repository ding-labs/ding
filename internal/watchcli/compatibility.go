package watchcli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func compatibilityCommands(root *cobra.Command, version string) {
	var structured bool
	versionCmd := &cobra.Command{Use: "version", Short: "Print the Ding version", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if structured {
			return Write(cmd.OutOrStdout(), map[string]string{"version": version, "os": runtime.GOOS, "arch": runtime.GOARCH})
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "ding version", version)
		return err
	}}
	versionCmd.Flags().BoolVar(&structured, "json", false, "emit a versioned JSON response")
	root.AddCommand(versionCmd)
	for _, name := range []string{"run", "serve", "test-rule", "install"} {
		command := &cobra.Command{Use: name, Hidden: true, DisableFlagParsing: true, RunE: func(cmd *cobra.Command, args []string) error {
			structured := false
			for _, arg := range args {
				if arg == "--" {
					break
				}
				if arg == "--json" || arg == "--json=true" {
					structured = true
				}
			}
			return Fail(cmd.ErrOrStderr(), structured, "legacy_command", fmt.Sprintf("ding %s belongs to the legacy runtime. Install v0.14.0 (https://github.com/ding-labs/ding/releases/tag/v0.14.0), or run ding migrate --config OLD.yaml --out NEW_DIRECTORY and use ding daemon. Existing rules and snapshots are not automatically loaded.", cmd.Name()))
		}}
		root.AddCommand(command)
	}
}
