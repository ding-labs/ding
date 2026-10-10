package watchcli

import (
	"fmt"
	"net/url"
	"time"

	"github.com/ding-labs/ding/internal/cloudlink"
	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/spf13/cobra"
)

func cloudMoveCommands(dir *string) *cobra.Command {
	group := &cobra.Command{Use: "move", Short: "Move one eligible watch with durable pause-before-activation handoff"}
	var yes bool
	prepare := &cobra.Command{Use: "prepare WATCH", Short: "Preflight locally; --yes uploads the selected definition and sends labeled cloud tests", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		local, err := control.Connect(*dir)
		if err != nil {
			return err
		}
		preflight, err := cloudlink.Call[watchrun.HandoffPreflight](cmd.Context(), local, "GET", "/v1/handoffs/preflight/"+url.PathEscape(args[0]), nil)
		if err != nil {
			return err
		}
		if !yes {
			return Write(cmd.OutOrStdout(), map[string]any{"preflight": preflight, "note": "No account or upload. Review cadence, destination references and the changed network location. Rebind credentials in Cloud, then use --yes to prepare and send tests. Evaluation starts fresh; local history stays here."})
		}
		if !preflight.Ready {
			return fmt.Errorf("not eligible: %s", preflight.Reason)
		}
		connection, err := cloudlink.Load(*dir)
		if err != nil {
			return fmt.Errorf("eligible locally; run ding cloud login only when ready to move: %w", err)
		}
		target, err := connection.Client()
		if err != nil {
			return err
		}
		review, err := cloudlink.Call[watchrun.ApplyResult](cmd.Context(), target, "POST", "/v1/apply", watchrun.ApplyRequest{Manifest: preflight.Manifest, DryRun: true})
		if err != nil {
			return err
		}
		for _, c := range review.Credentials {
			if !c.Present {
				return fmt.Errorf("rebind credential %s in Cloud before preparing", c.Environment)
			}
		}
		id, key := cloudlink.Proof()
		transfer := cloudlink.Transfer{ID: id, URL: connection.URL, Workspace: connection.Workspace, Source: preflight, TargetReview: review.Review, TestKey: key}
		unlock, err := install.Lock(*dir)
		if err != nil {
			return err
		}
		defer unlock()
		if err := cloudlink.SaveTransfer(*dir, transfer); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Transfer %s saved. Source remains running during preparation.\n", id)
		if _, err := transfer.Prepare(cmd.Context(), target); err != nil {
			return fmt.Errorf("prepare unresolved; use ding cloud move status %s: %w", id, err)
		}
		probe, err := transfer.Test(cmd.Context(), target)
		if err != nil {
			return fmt.Errorf("test unresolved; inspect transfer %s before retrying: %w", id, err)
		}
		if probe.Outcome != "accepted" {
			return fmt.Errorf("test outcome %s; source still running, target paused; inspect transfer %s", probe.Outcome, id)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Cloud source and destination tests accepted. After receiving the labeled notification, run:\nding cloud move finish %s --confirm-delivery\n", id)
		return nil
	}}
	prepare.Flags().BoolVar(&yes, "yes", false, "upload only this manifest, create a paused target and send labeled destination tests")
	group.AddCommand(prepare)
	for _, action := range []string{"status", "test", "finish", "cancel"} {
		var confirm bool
		cmd := &cobra.Command{Use: action + " TRANSFER_ID", Short: map[string]string{"status": "Inspect both durable sides without changing execution", "test": "Recheck the same labeled test without sending it twice", "finish": "After visible delivery confirmation, pause source and activate target", "cancel": "Confirm target never activated, then release the local hold"}[action], Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			unlock, err := install.Lock(*dir)
			if err != nil {
				return err
			}
			defer unlock()
			transfer, err := cloudlink.LoadTransfer(*dir, args[0])
			if err != nil {
				return err
			}
			connection, err := cloudlink.Load(*dir)
			if err != nil {
				return err
			}
			target, err := transfer.Target(connection)
			if err != nil {
				return err
			}
			local, err := control.Connect(*dir)
			if err != nil {
				return err
			}
			switch action {
			case "status":
				source, sourceErr := cloudlink.Call[store.Handoff](cmd.Context(), local, "GET", "/v1/handoffs/"+transfer.ID, nil)
				remote, remoteErr := cloudlink.Call[store.Handoff](cmd.Context(), target, "GET", "/v1/handoffs/"+transfer.ID, nil)
				return Write(cmd.OutOrStdout(), map[string]any{"id": transfer.ID, "source": source, "target": remote, "sourceError": safeCloudError(sourceErr), "targetError": safeCloudError(remoteErr), "note": "An unreachable target does not prove inactivity. Never resume a held source on that assumption."})
			case "test":
				out, err := transfer.Test(cmd.Context(), target)
				if err != nil {
					return err
				}
				return Write(cmd.OutOrStdout(), out)
			case "cancel":
				if !confirm {
					return fmt.Errorf("use --yes to cancel a target that has never activated; source remains paused if already paused")
				}
				if err := transfer.Cancel(cmd.Context(), local, target); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Target canceled. If the source was paused, inspect it and explicitly resume it when ready.")
				return nil
			case "finish":
				if !confirm {
					return fmt.Errorf("receive the labeled cloud test, then use --confirm-delivery; a short monitoring gap and fresh evaluation state are expected")
				}
				out, err := transfer.Finish(cmd.Context(), local, target)
				if err != nil {
					return err
				}
				deadline := time.NewTimer(40 * time.Second)
				defer deadline.Stop()
				tick := time.NewTicker(time.Second)
				defer tick.Stop()
				for {
					select {
					case <-cmd.Context().Done():
						return cmd.Context().Err()
					case <-deadline.C:
						return fmt.Errorf("cloud activation committed, but first observation is not confirmed; source stays paused; inspect %s", transfer.ID)
					case <-tick.C:
					}
					record, err := cloudlink.Call[store.WatchSummary](cmd.Context(), target, "GET", "/v1/watches/"+url.PathEscape(out.WatchID), nil)
					if err == nil && !record.Watch.LastInputAt.IsZero() {
						return Write(cmd.OutOrStdout(), map[string]any{"transfer": out, "firstObservation": record.Watch.LastInputAt, "note": "Runs in Cloud. Local history is preserved and the local source is held paused."})
					}
				}
			}
			return nil
		}}
		if action == "finish" {
			cmd.Flags().BoolVar(&confirm, "confirm-delivery", false, "confirm you received the cloud test and approve the reviewed handoff")
		}
		if action == "cancel" {
			cmd.Flags().BoolVar(&confirm, "yes", false, "cancel only a never-activated target and release the source hold")
		}
		group.AddCommand(cmd)
	}
	return group
}
func safeCloudError(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
