package watchcli

import (
	"io"

	"github.com/ding-labs/ding/internal/notify"
	"github.com/spf13/cobra"
)

func notificationCommand() *cobra.Command {
	return &cobra.Command{Use: "notification-open URI", Hidden: true, Short: "Open a validated notification target", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		target, err := notify.ParseActivation(args[0])
		if err != nil {
			return err
		}
		root := Root(cmd.Root().Version)
		root.SetArgs([]string{"ui", "--state-dir", target.StateDir, "--watch", target.WatchID})
		root.SetOut(io.Discard) // Never log the ephemeral browser handoff.
		root.SetErr(cmd.ErrOrStderr())
		return root.ExecuteContext(cmd.Context())
	}}
}
