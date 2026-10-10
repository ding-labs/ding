package watchcli

import (
	"path/filepath"

	"github.com/ding-labs/ding/internal/install"
	"github.com/spf13/cobra"
)

func daemonLogFlag(cmd *cobra.Command, dir *string) {
	cmd.Flags().Bool("background-log", false, "write bounded private daemon logs in the state directory")
}

// Called only after the daemon owns the state lock: a second process must not
// rotate the active daemon's files while reporting a lock conflict.
func prepareDaemonLog(cmd *cobra.Command, dir string) (func(), error) {
	enabled, _ := cmd.Flags().GetBool("background-log")
	if !enabled {
		return func() {}, nil
	}
	log, err := install.OpenLog(filepath.Join(dir, "daemon.log"), 1<<20)
	if err != nil {
		return nil, err
	}
	cmd.SetOut(log)
	cmd.SetErr(log)
	return func() { _ = log.Close() }, nil
}
