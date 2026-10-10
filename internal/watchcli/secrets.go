package watchcli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ding-labs/ding/internal/install"
	"github.com/spf13/cobra"
)

func secretCommands(dir *string) *cobra.Command {
	group := &cobra.Command{Use: "secret", Short: "Manage explicit private-file credentials for background operation"}
	var stdin bool
	set := &cobra.Command{Use: "set NAME --stdin", Short: "Read a secret from stdin into the local private credential file", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !stdin {
			return fmt.Errorf("use --stdin; never put a secret value in command arguments")
		}
		data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), (64<<10)+1))
		if err != nil {
			return err
		}
		value := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
		if len(data) > 64<<10 {
			return fmt.Errorf("secret exceeds 64 KiB")
		}
		if err := install.SetSecret(*dir, args[0], value); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Saved in private local secrets.json. Restart Ding to load the updated credential. Include this file separately in protected backups.")
		return nil
	}}
	set.Flags().BoolVar(&stdin, "stdin", false, "read the value from standard input; private file storage is explicit")
	list := &cobra.Command{Use: "list", Short: "List credential names without their values", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		values, err := install.ReadSecrets(*dir)
		if err != nil {
			return err
		}
		names := []string{}
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)
		return Write(cmd.OutOrStdout(), map[string]any{"backend": "private-file", "names": names})
	}}
	group.AddCommand(set, list)
	return group
}
