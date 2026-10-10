package watchcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ding-labs/ding/internal/cloudlink"
	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/install"
	"github.com/spf13/cobra"
)

func cloudCommands(dir *string) *cobra.Command {
	group := &cobra.Command{Use: "cloud", Short: "Optionally connect hosted execution; local Ding remains account-free"}
	var endpoint string
	login := &cobra.Command{Use: "login --url HTTPS_ORIGIN", Short: "Approve this computer in your browser; never uploads a local watch", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		path := filepath.Join(*dir, "cloud.json")
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cloud connection already exists or cannot be read; use ding cloud logout first")
		}
		connection := cloudlink.Connection{URL: endpoint}
		client, err := connection.Client()
		if err != nil {
			return err
		}
		verifier, challenge := cloudlink.Proof()
		device, err := cloudlink.Call[cloudlink.Device](cmd.Context(), client, "POST", "/v1/cloud/device/start", map[string]string{"challenge": challenge})
		if err != nil {
			return err
		}
		if len(device.ID) != 64 || device.URL != client.URL+"/connect/"+device.ID || device.Code != device.ID[:8] {
			return fmt.Errorf("invalid device response")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Open %s\nApprove only if it displays code %s. Local monitoring continues.\n", device.URL, device.Code)
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()
		timer := time.NewTicker(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("connection canceled or expired; local watches are unchanged")
			case <-timer.C:
			}
			claim, err := cloudlink.Call[cloudlink.Claim](ctx, client, "POST", "/v1/cloud/device/claim", map[string]string{"id": device.ID, "verifier": verifier})
			if err != nil {
				return fmt.Errorf("could not claim connection; start a new login: %w", err)
			}
			if !claim.Ready {
				continue
			}
			if len(claim.Token) != 64 || len(claim.Workspace) != 64 || !claim.ExpiresAt.After(time.Now()) {
				return fmt.Errorf("invalid connection response")
			}
			connection.Token, connection.Workspace, connection.ExpiresAt = claim.Token, claim.Workspace, claim.ExpiresAt
			unlock, err := install.Lock(*dir)
			if err != nil {
				return err
			}
			defer unlock()
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("another login completed; use cloud logout before reconnecting")
			}
			if err := install.AtomicJSON(path, connection); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Connected. Use ding cloud status. No watches or source credentials were uploaded.")
			return nil
		}
	}}
	login.Flags().StringVar(&endpoint, "url", "", "explicit HTTPS origin of the cloud service")
	_ = login.MarkFlagRequired("url")
	status := &cobra.Command{Use: "status", Short: "Inspect cloud connection and hosted usage", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := cloudlink.Load(*dir)
		if err != nil {
			return err
		}
		client, err := c.Client()
		if err != nil {
			return err
		}
		usage, err := client.Call(cmd.Context(), "GET", "/v1/cloud/usage", nil)
		if err != nil {
			return err
		}
		return Write(cmd.OutOrStdout(), map[string]any{"url": c.URL, "workspace": c.Workspace, "expiresAt": c.ExpiresAt, "usage": usage})
	}}
	logout := &cobra.Command{Use: "logout", Short: "Revoke this computer's cloud session; hosted watches keep running", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		unlock, err := install.Lock(*dir)
		if err != nil {
			return err
		}
		defer unlock()
		c, err := cloudlink.Load(*dir)
		if err != nil {
			return err
		}
		client, err := c.Client()
		if err != nil {
			return err
		}
		if _, err := client.Call(cmd.Context(), "DELETE", "/v1/browser/session", nil); err != nil {
			if api, ok := err.(*control.APIError); !ok || api.Code != "sign_in_required" {
				return fmt.Errorf("revocation unconfirmed; retained the connection so you can retry: %w", err)
			}
		}
		if err := os.Remove(filepath.Join(*dir, "cloud.json")); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Disconnected. Cloud and local watches keep their current execution state.")
		return nil
	}}
	group.AddCommand(login, status, logout, cloudMoveCommands(dir))
	return group
}
