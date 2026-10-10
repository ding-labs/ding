package watchcli

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"

	"github.com/ding-labs/ding/internal/install"
	"github.com/spf13/cobra"
)

func secretCommands(dir *string) *cobra.Command {
	group := &cobra.Command{Use: "secret", Short: "Manage credentials for background operation without exposing their values"}
	var stdin bool
	backend := "private-file"
	if runtime.GOOS == "windows" {
		backend = "dpapi"
	}
	if runtime.GOOS == "darwin" {
		backend = "keychain"
	}
	set := &cobra.Command{Use: "set NAME --stdin", Short: "Read a secret from stdin into the selected local credential store", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
		switch backend {
		case "dpapi":
			err = install.SetDPAPISecret(*dir, args[0], value)
		case "keychain":
			err = install.SetKeychainSecret(cmd.Context(), *dir, args[0], value)
		case "private-file":
			err = install.SetSecret(*dir, args[0], value)
		default:
			return fmt.Errorf("choose keychain, dpapi, or private-file storage")
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Saved using %s storage. Restart Ding after adding or replacing a reference. Credentials need separate protected recovery; a database backup alone is insufficient.\n", backend)
		return nil
	}}
	set.Flags().BoolVar(&stdin, "stdin", false, "read the value from standard input; private file storage is explicit")
	set.Flags().StringVar(&backend, "store", backend, "keychain (macOS), dpapi (Windows), or explicit private-file storage")
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
		keychain, err := install.KeychainNames(*dir)
		if err != nil {
			return err
		}
		native := []string{}
		for name := range keychain {
			native = append(native, name)
		}
		sort.Strings(native)
		protected, err := install.DPAPINames(*dir)
		if err != nil {
			return err
		}
		sort.Strings(protected)
		return Write(cmd.OutOrStdout(), map[string]any{"privateFile": names, "keychain": native, "dpapi": protected})
	}}
	group.AddCommand(set, list)
	return group
}
