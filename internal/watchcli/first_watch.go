package watchcli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/onboarding"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/spf13/cobra"
)

func firstWatchCommand(dir *string) *cobra.Command {
	var id, delivery string
	var yes bool
	cmd := &cobra.Command{Use: "create URL", Short: "Preview or create a first HTTP health watch without YAML", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return createFirstWatch(cmd, *dir, id, args[0], delivery, yes)
	}}
	cmd.Flags().StringVar(&id, "id", "first-watch", "new watch ID; existing watches are never replaced")
	cmd.Flags().StringVar(&delivery, "delivery", "desktop", "desktop or console; use manifests for network destinations")
	cmd.Flags().BoolVar(&yes, "yes", false, "activate the previewed watch; otherwise only preview")
	return cmd
}

func createFirstWatch(cmd *cobra.Command, dir, id, url, delivery string, yes bool) error {
	raw, err := request(cmd, dir, false, "POST", "/v1/onboarding/preview", onboarding.Request{ID: id, URL: url, Delivery: delivery})
	if err != nil {
		return err
	}
	var preview control.FirstWatchPreview
	if err := json.Unmarshal(raw, &preview); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), preview.Manifest)
	fmt.Fprintln(cmd.OutOrStdout(), "Runs on this computer every 30 seconds. Three HTTP 5xx responses fire; two nonmatching responses recover. Source failures alert separately.")
	if !yes {
		fmt.Fprintln(cmd.OutOrStdout(), "Preview only. Repeat with --yes to activate. Use ding notify test before choosing desktop delivery.")
		return nil
	}
	if _, err := request(cmd, dir, false, "POST", "/v1/apply", watchrun.ApplyRequest{Manifest: preview.Manifest, Review: preview.Review.Review}); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Watch started and waiting for its first observation. Run ding status to inspect it.")
	return nil
}

func notifyCommand(dir *string) *cobra.Command {
	group := &cobra.Command{Use: "notify", Short: "Verify notification delivery from the daemon"}
	group.AddCommand(&cobra.Command{Use: "test", Short: "Ask for desktop permission and send a labeled test notification", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := control.Connect(*dir)
		if err != nil {
			return err
		}
		client.HTTP.Timeout = 65 * time.Second // Includes the server's 60-second permission flow.
		if _, err = client.Call(cmd.Context(), "POST", "/v1/desktop/test", struct{}{}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "The OS accepted the test notification. Confirm you can see it before relying on desktop alerts.")
		return nil
	}})
	return group
}
