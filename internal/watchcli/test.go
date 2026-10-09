package watchcli

import (
	"fmt"
	"os"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/replay"
	"github.com/spf13/cobra"
)

func testCommand() *cobra.Command {
	var fixture, id string
	var structured bool
	cmd := &cobra.Command{Use: "test FILE --events OBSERVATIONS.jsonl", Short: "Replay recorded observations without running sources or sending alerts", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "read_failed", "cannot read manifest")
		}
		bundle, err := plan.Parse(data)
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "invalid_manifest", err.Error())
		}
		var selected *plan.Compiled
		for i := range bundle.Watches {
			w := &bundle.Watches[i]
			if w.Definition.Metadata.ID == id || (id == "" && len(bundle.Watches) == 1) {
				selected = w
			}
		}
		if selected == nil {
			return Fail(cmd.ErrOrStderr(), structured, "invalid_watch", "select one watch with --watch")
		}
		f, err := os.Open(fixture)
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "read_failed", "cannot read observations")
		}
		defer f.Close()
		report, err := replay.Run(*selected, f)
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "invalid_fixture", err.Error())
		}
		if structured {
			return Write(cmd.OutOrStdout(), report)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %d observations, %d events\n", report.WatchID, report.Observations, len(report.Events))
		for _, event := range report.Events {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", event.At.Format("2006-01-02T15:04:05Z07:00"), event.Type, event.Message)
		}
		return nil
	}}
	cmd.Flags().StringVar(&fixture, "events", "", "JSONL observations with sequence and acceptedAt")
	_ = cmd.MarkFlagRequired("events")
	cmd.Flags().StringVar(&id, "watch", "", "watch ID when manifest contains multiple watches")
	cmd.Flags().BoolVar(&structured, "json", false, "emit a versioned JSON response")
	return cmd
}
