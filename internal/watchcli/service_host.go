package watchcli

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"

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
			daemon.SetArgs([]string{"daemon", "--state-dir", *dir, "--listen", "127.0.0.1:0", "--background-log"})
			daemon.SetOut(cmd.OutOrStdout())
			daemon.SetErr(cmd.ErrOrStderr())
			return daemon.ExecuteContext(ctx)
		}, func(ctx context.Context) error { return waitForDaemon(ctx, *dir) })
	}}
	command.Flags().StringVar(&name, "name", "", "SCM registration name from the boot template")
	return command
}

func bootTemplateCommand(dir *string) *cobra.Command {
	var executable, user, group string
	cmd := &cobra.Command{Use: "boot-template", Short: "Print an administrator-reviewed unattended startup definition", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("state-dir") {
			return fmt.Errorf("choose a separate absolute --state-dir explicitly for the service account")
		}
		d, err := service.BootTemplate(runtime.GOOS, executable, *dir, user, group)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Review before installing as administrator: %s\nCreate the service account and private state directory first (Windows script creates its directory). Use remote notifications and explicitly provision private-file credentials under that identity. Never share a writer with a user service.\n", d.Path)
		_, err = fmt.Fprint(cmd.OutOrStdout(), d.Content)
		return err
	}}
	cmd.Flags().StringVar(&executable, "executable", "", "absolute stable installed Ding path")
	cmd.Flags().StringVar(&user, "user", "", "existing non-root service account")
	cmd.Flags().StringVar(&group, "group", "", "existing service group; defaults to user")
	return cmd
}
