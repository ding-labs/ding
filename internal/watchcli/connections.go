package watchcli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ding-labs/ding/internal/cloudlink"
	"github.com/ding-labs/ding/internal/control"
	"github.com/spf13/cobra"
)

var connectionName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func connectionPath(dir, name string) (string, error) {
	if !connectionName.MatchString(name) {
		return "", fmt.Errorf("connection name must contain 1–64 letters, digits, underscores or hyphens")
	}
	return filepath.Join(dir, "connections", name), nil
}
func connectionsCommand(dir *string) *cobra.Command {
	var name, profileDir string
	group := cloudCommands(&profileDir)
	group.Use = "connection"
	group.Short = "Save a named connection to a shared or cloud server"
	group.PersistentFlags().StringVar(&name, "name", "", "connection name")
	group.PersistentPreRunE = func(*cobra.Command, []string) error {
		var err error
		profileDir, err = connectionPath(*dir, name)
		return err
	}
	for _, c := range group.Commands() {
		if c.Name() == "move" {
			group.RemoveCommand(c)
		}
	}
	return group
}
func requestClient(cmd *cobra.Command, dir string) (control.Client, error) {
	name, err := cmd.Flags().GetString("connection")
	if err != nil || name == "" {
		return control.Connect(dir)
	}
	path, err := connectionPath(dir, name)
	if err != nil {
		return control.Client{}, err
	}
	connection, err := cloudlink.Load(path)
	if err != nil {
		return control.Client{}, fmt.Errorf("cannot load named connection %q: %w", name, err)
	}
	return connection.Client()
}
func connectionFlags(root *cobra.Command) {
	root.PersistentFlags().String("connection", "", "explicit named remote connection for watch operations")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		name, _ := cmd.Flags().GetString("connection")
		if name == "" {
			return nil
		}
		if !connectionName.MatchString(name) {
			return fmt.Errorf("invalid connection name")
		}
		path := strings.TrimPrefix(cmd.CommandPath(), "ding ")
		switch path {
		case "apply", "watch list", "watch inspect", "watch pause", "watch resume", "watch delete", "events", "events inspect", "events observations", "delivery inspect", "delivery retry", "doctor", "export":
			return nil
		}
		return fmt.Errorf("--connection is not supported for this command; host operations require an explicit local invocation")
	}
}
