package watchcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/service"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/update"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/spf13/cobra"
)

func updateCommands(root *cobra.Command, dir *string) {
	group := &cobra.Command{Use: "update", Short: "Check signed releases and safely update a standalone installation"}
	updateScheduleCommands(group, dir)
	for _, action := range []string{"check", "install", "recover"} {
		var yes bool
		command := &cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := install.Load(*dir)
			if err != nil {
				return err
			}
			if r.Owner != "standalone" {
				return fmt.Errorf("this installation belongs to %s; update it through that package manager", r.Owner)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
			defer cancel()
			if action != "check" {
				unlock, err := install.Lock(*dir)
				if err != nil {
					return err
				}
				defer unlock()
				// Refresh ownership after acquiring the lock.
				r, err = install.Load(*dir)
				if err != nil {
					return err
				}
			}
			m, err := service.New(r)
			if err != nil {
				return err
			}
			hooks := update.Hooks{
				Backup: func(ctx context.Context, path string) error {
					c, err := control.Connect(*dir)
					if err != nil {
						return err
					}
					_, err = c.Call(ctx, "POST", "/v1/backup", map[string]string{"path": path})
					return err
				},
				Stop: func(ctx context.Context) error { return m.Action(ctx, "stop") },
				Start: func(ctx context.Context) error {
					if err := m.Action(ctx, "start"); err != nil {
						return err
					}
					probe, cancel := context.WithTimeout(ctx, 30*time.Second)
					defer cancel()
					return waitForDaemon(probe, *dir)
				},
			}
			if action == "recover" {
				if err := update.Recover(ctx, *dir, hooks); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Update recovery completed; watch state was retained.")
				return nil
			}
			client := update.Client{}
			manifest, err := client.Check(ctx, update.PublicKey, r.Channel, time.Now())
			if err != nil {
				return err
			}
			artifact, err := manifest.Select(runtime.GOOS, runtime.GOARCH, r.Version, store.SchemaVersion)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Verified %s release %s (%d bytes). A database backup is retained before a brief service restart.\n", manifest.Channel, manifest.Version, artifact.Bytes)
			if action == "check" {
				return nil
			}
			if !yes {
				return fmt.Errorf("review the release, then run ding update install --yes")
			}
			if runtime.GOOS == "windows" {
				return fmt.Errorf("use the signed Windows installer to update this installation")
			}
			if err := update.CheckSpace(filepath.Dir(r.Executable), *dir, artifact.Bytes); err != nil {
				return err
			}
			stage, err := os.MkdirTemp(filepath.Dir(r.Executable), ".ding-update-")
			if err != nil {
				return err
			}
			defer func() {
				if _, err := os.Lstat(filepath.Join(*dir, "update.json")); os.IsNotExist(err) {
					_ = os.RemoveAll(stage)
				}
			}()
			candidate, err := client.Stage(ctx, artifact, stage)
			if err != nil {
				return err
			}
			if err := verifyCandidate(ctx, candidate, manifest); err != nil {
				return err
			}
			if err := update.Apply(ctx, r, strings.TrimPrefix(manifest.Version, "v"), stage, hooks); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Ding updated and the background daemon is ready.")
			return nil
		}}
		if action == "install" {
			command.Flags().BoolVar(&yes, "yes", false, "install the verified release with a backup and service restart")
		}
		group.AddCommand(command)
	}
	root.AddCommand(group)
}

func verifyCandidate(ctx context.Context, path string, manifest update.Manifest) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, path, "version", "--json").Output()
	if err != nil || len(data) > 1<<20 {
		return fmt.Errorf("candidate executable did not report a valid version")
	}
	var result struct {
		APIVersion string `json:"apiVersion"`
		Data       struct {
			Version, OS, Arch string
			Schema            int
		} `json:"data"`
	}
	if json.Unmarshal(data, &result) != nil || result.APIVersion != watch.APIVersion || "v"+strings.TrimPrefix(result.Data.Version, "v") != manifest.Version || result.Data.OS != runtime.GOOS || result.Data.Arch != runtime.GOARCH || result.Data.Schema != store.SchemaVersion || result.Data.Schema != manifest.Schema {
		return fmt.Errorf("candidate version, platform, or schema differs from the signed release")
	}
	return nil
}
