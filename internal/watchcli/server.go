package watchcli

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ding-labs/ding/internal/cloud"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/serverops"
	"github.com/spf13/cobra"
)

func serverCommand(version string) *cobra.Command {
	var config string
	root := &cobra.Command{Use: "server", Short: "Operate a shared self-hosted Ding instance"}
	root.PersistentFlags().StringVar(&config, "config", "", "owner-only server configuration JSON")
	read := func() (cloud.Config, error) {
		var c cloud.Config
		if err := mcpconfig.ReadPrivateJSON(config, &c, true); err != nil {
			return c, err
		}
		if c.Hosting == "" {
			c.Hosting = "self-hosted"
		}
		if c.Hosting != "self-hosted" {
			return c, fmt.Errorf("ding server requires self-hosted hosting")
		}
		return c, c.Validate()
	}
	var check bool
	run := &cobra.Command{Use: "run", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := read()
		if err != nil {
			return err
		}
		if check {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Self-hosted configuration is valid.")
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return cloud.Run(ctx, c, version, cmd.ErrOrStderr())
	}}
	run.Flags().BoolVar(&check, "check-config", false, "validate configuration without starting services")
	root.AddCommand(run)
	for _, name := range []string{"init", "recover"} {
		var username, displayName, passwordFile string
		command := &cobra.Command{Use: name, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := read()
			if err != nil {
				return err
			}
			password, err := serverPassword(passwordFile)
			if err != nil {
				return err
			}
			if err := cloud.ValidateServerDirectory(c.DataDir); err != nil {
				return err
			}
			storage, err := cloud.OpenServerStorage(cmd.Context(), c.DataDir, c.Database)
			if err != nil {
				return err
			}
			defer storage.Close()
			db := storage.Control
			if err := db.ConfigureHosting(cmd.Context(), c.Hosting); err != nil {
				return err
			}
			if err := db.ConfigureLoginMethods(cmd.Context(), c.LoginMethods); err != nil {
				return err
			}
			if cmd.Name() == "recover" {
				if err := db.RecoverPassword(cmd.Context(), username, password); err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Password recovered; previous sessions and credentials revoked.")
				return err
			}
			if err := serverKey(c); err != nil {
				return err
			}
			owner, err := db.Bootstrap(cmd.Context(), username, displayName, password)
			if err != nil {
				return err
			}
			return Write(cmd.OutOrStdout(), owner)
		}}
		command.Flags().StringVar(&username, "username", "", "built-in username")
		command.Flags().StringVar(&displayName, "name", "Administrator", "display name")
		command.Flags().StringVar(&passwordFile, "password-file", "", "owner-only file containing the password")
		root.AddCommand(command)
	}
	for _, name := range []string{"backup", "restore", "release-restore", "backup-keygen", "delete-workspace"} {
		root.AddCommand(&cobra.Command{Use: name, Short: "Offline host operation; stop the server before changing its state", DisableFlagParsing: true, RunE: func(cmd *cobra.Command, args []string) error {
			_, err := serverops.Run(append([]string{cmd.Name()}, args...), cmd.OutOrStdout())
			return err
		}})
	}
	return root
}

func serverPassword(path string) (string, error) {
	f, err := mcpconfig.OpenPrivate(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1027))
	if err != nil {
		return "", err
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	if len(password) < 12 || len(password) > 1024 {
		return "", fmt.Errorf("password must contain 12–1024 bytes")
	}
	return password, nil
}

func serverKey(c cloud.Config) error {
	dir, err := filepath.EvalSymlinks(c.DataDir)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(c.KeyFile))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, filepath.Join(parent, filepath.Base(c.KeyFile)))
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("keep encryption key outside the data and backup volume")
	}
	f, err := mcpconfig.OpenPrivate(c.KeyFile)
	if err == nil {
		defer f.Close()
		b, e := io.ReadAll(io.LimitReader(f, 33))
		if e != nil {
			return e
		}
		if len(b) != 32 {
			return fmt.Errorf("keyFile must contain exactly 32 private bytes")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	f, err = mcpconfig.CreatePrivate(c.KeyFile)
	if err != nil {
		return err
	}
	defer f.Close()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if _, err := f.Write(key); err != nil {
		return err
	}
	return f.Sync()
}
