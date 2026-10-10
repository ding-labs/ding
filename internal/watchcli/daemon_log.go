package watchcli

import (
	"fmt"
	"path/filepath"

	"github.com/ding-labs/ding/internal/install"
	"github.com/spf13/cobra"
)

func daemonLogFlag(cmd *cobra.Command, dir *string) {
	var enabled bool
	cmd.Flags().BoolVar(&enabled, "background-log", false, "write bounded private daemon logs in the state directory")
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !enabled {
			return run(cmd, args)
		}
		log, err := install.OpenLog(filepath.Join(*dir, "daemon.log"), 1<<20)
		if err != nil {
			return err
		}
		defer log.Close()
		cmd.SetOut(log)
		cmd.SetErr(log)
		err = run(cmd, args)
		if err != nil {
			fmt.Fprintln(log, "Ding stopped:", err)
		}
		return err
	}
}
