package watchcli

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/ding-labs/ding/internal/service"
	"github.com/spf13/cobra"
)

func serviceHostCommand(root *cobra.Command, dir *string) *cobra.Command {
	var name string
	command := &cobra.Command{Use: "host", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !regexp.MustCompile(`^DingSystem-[a-f0-9]{16}$`).MatchString(name) || !filepath.IsAbs(*dir) {
			return fmt.Errorf("use the generated boot template's service name and absolute state directory")
		}
		return service.RunSystem(name, func(ctx context.Context) error {
			daemon := Root(root.Version)
			daemon.SetArgs([]string{"daemon", "--state-dir", *dir, "--listen", "127.0.0.1:7676", "--background-log"})
			daemon.SetOut(cmd.OutOrStdout())
			daemon.SetErr(cmd.ErrOrStderr())
			return daemon.ExecuteContext(ctx)
		}, func(ctx context.Context) error { return waitForDaemon(ctx, *dir) })
	}}
	command.Flags().StringVar(&name, "name", "", "SCM registration name from the boot template")
	return command
}
