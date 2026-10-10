package watchcli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ding-labs/ding/internal/install"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/service"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/update"
	"github.com/spf13/cobra"
)

type automaticAttempt struct {
	At      time.Time `json:"at"`
	Version string    `json:"version"`
	Outcome string    `json:"outcome"`
}

func updateScheduleCommands(group *cobra.Command, dir *string) {
	var checks, automatic bool
	var hour int
	configure := &cobra.Command{Use: "configure", Short: "Configure daily checks and opt into a UTC maintenance hour", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		settings, err := update.LoadSettings(*dir)
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("checks") && !cmd.Flags().Changed("automatic") && !cmd.Flags().Changed("hour-utc") {
			return Write(cmd.OutOrStdout(), settings)
		}
		if cmd.Flags().Changed("checks") {
			settings.Checks = checks
		}
		if cmd.Flags().Changed("automatic") {
			settings.Automatic = automatic
		}
		if cmd.Flags().Changed("hour-utc") {
			settings.HourUTC = hour
		}
		if settings.HourUTC < 0 || settings.HourUTC > 23 || settings.Automatic && !settings.Checks {
			return fmt.Errorf("choose hour 0–23 UTC; automatic updates require enabled checks")
		}
		r, err := install.Load(*dir)
		if err != nil {
			return err
		}
		if settings.Automatic && (r.Owner != "standalone" || r.Channel != "stable" || runtime.GOOS == "windows" || update.PublicKey == "") {
			return fmt.Errorf("automatic updates require a qualified standalone stable macOS/Linux release; package managers and Windows retain their own update mechanism")
		}
		unlock, err := install.Lock(*dir)
		if err != nil {
			return err
		}
		defer unlock()
		if err := install.AtomicJSON(filepath.Join(*dir, "updates.json"), settings); err != nil {
			return err
		}
		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
			job, err := service.NewUpdateJob(r)
			if err != nil {
				return err
			}
			if settings.Automatic {
				err = job.Install(cmd.Context())
			} else {
				err = job.Uninstall(cmd.Context())
			}
			if err != nil {
				return fmt.Errorf("settings saved; repair the update job before expecting automatic installation: %w", err)
			}
		}
		return Write(cmd.OutOrStdout(), settings)
	}}
	configure.Flags().BoolVar(&checks, "checks", true, "check signed metadata at most daily; disabling keeps local monitoring fully available")
	configure.Flags().BoolVar(&automatic, "automatic", false, "opt into same-schema stable updates with backup and readiness verification")
	configure.Flags().IntVar(&hour, "hour-utc", 3, "UTC maintenance hour, 0–23; at most one automatic attempt per day")
	scheduled := &cobra.Command{Use: "scheduled", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		settings, err := update.LoadSettings(*dir)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if !settings.Automatic || !settings.Checks || now.Hour() != settings.HourUTC {
			return nil
		}
		r, err := install.Load(*dir)
		if err != nil {
			return err
		}
		if r.Owner != "standalone" || r.Channel != "stable" || runtime.GOOS == "windows" {
			return fmt.Errorf("installation is not eligible for automatic replacement")
		}
		check, err := update.CheckOnce(cmd.Context(), *dir, store.SchemaVersion, false)
		if err != nil {
			return err
		}
		if check.Available == "" || !check.Compatible || check.Error != "" {
			return nil
		}
		unlock, err := install.Lock(*dir)
		if err != nil {
			return err
		}
		settings, err = update.LoadSettings(*dir)
		if err != nil || !settings.Automatic || !settings.Checks {
			unlock()
			return err
		}
		path := filepath.Join(*dir, "automatic-update.json")
		var last automaticAttempt
		_ = mcpconfig.ReadPrivateJSON(path, &last, true)
		if !last.At.IsZero() && now.Sub(last.At) < 24*time.Hour {
			unlock()
			return nil
		}
		attempt := automaticAttempt{At: now, Version: check.Available, Outcome: "started"}
		err = install.AtomicJSON(path, attempt)
		unlock()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 4*time.Minute)
		defer cancel()
		process := exec.CommandContext(ctx, r.Executable, "update", "install", "--yes", "--state-dir", *dir)
		process.Stdout, process.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
		err = process.Run()
		attempt.Outcome = "completed"
		if err != nil {
			attempt.Outcome = "failed; inspect ding status and ding update recover before retrying"
		}
		if unlock, lockErr := install.Lock(*dir); lockErr == nil {
			_ = install.AtomicJSON(path, attempt)
			unlock()
		}
		return err
	}}
	group.AddCommand(configure, scheduled)
}
