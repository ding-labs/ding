// Package serverops provides offline recovery operations for shared and cloud hosts.
package serverops

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/cloud"
	"github.com/ding-labs/ding/internal/cloudbackup"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

func Run(args []string, out io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "init", "recover", "backup", "restore", "release-restore", "backup-keygen", "delete-workspace":
	default:
		return false, nil
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(out)
	config := flags.String("config", "", "owner-only configuration JSON")
	username := flags.String("username", "", "built-in username")
	name := flags.String("name", "Owner", "owner display name")
	passwordFile := flags.String("password-file", "", "owner-only file containing a password")
	dir := flags.String("data-dir", "", "absolute server data directory")
	output := flags.String("output", "", "new output file; never overwrites")
	input := flags.String("input", "", "encrypted backup to restore")
	recipient := flags.String("recipient", "", "age X25519 public recipient")
	identity := flags.String("identity-file", "", "owner-only age X25519 private identity file")
	confirm := flags.Bool("confirm-other-runners-stopped", false, "declare that execution ownership was checked across cloud and all transferred local copies")
	account := flags.String("confirm-delete-workspace", "", "exact workspace ID to irreversibly remove from stopped state")
	if err := flags.Parse(args[1:]); err != nil {
		return true, err
	}
	if flags.NArg() != 0 {
		return true, fmt.Errorf("unexpected arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	started := time.Now()
	var err error
	switch args[0] {
	case "init", "recover":
		var c cloud.Config
		if err := mcpconfig.ReadPrivateJSON(*config, &c, true); err != nil {
			return true, err
		}
		password, err := ReadPassword(*passwordFile)
		if err != nil {
			return true, err
		}
		err = cloud.Initialize(ctx, c, *username, *name, password, args[0] == "recover")
		if err != nil {
			return true, err
		}
	case "delete-workspace":
		if *account == "" || !*confirm {
			return true, fmt.Errorf("--confirm-delete-workspace and --confirm-other-runners-stopped required")
		}
		err = cloud.DeleteOffline(ctx, *dir, *account)
	case "backup-keygen":
		if *output == "" {
			return true, fmt.Errorf("--output required")
		}
		key, e := age.GenerateX25519Identity()
		if e != nil {
			return true, e
		}
		f, e := mcpconfig.CreatePrivate(*output)
		if e != nil {
			return true, e
		}
		_, e = f.WriteString(key.String() + "\n")
		if e == nil {
			e = f.Sync()
		}
		closed := f.Close()
		if e != nil {
			return true, e
		}
		if closed != nil {
			return true, closed
		}
		fmt.Fprintln(out, "Public backup recipient:", key.Recipient().String())
		return true, nil
	case "backup":
		if *dir == "" || *output == "" || *recipient == "" {
			return true, fmt.Errorf("--data-dir, --output and --recipient required")
		}
		err = cloudbackup.Write(ctx, *dir, *output, *recipient)
	case "restore":
		if *dir == "" || *input == "" || *identity == "" {
			return true, fmt.Errorf("--data-dir, --input and --identity-file required")
		}
		key, e := cloudbackup.ReadIdentity(*identity)
		if e != nil {
			return true, e
		}
		err = cloudbackup.Restore(ctx, *input, *dir, key)
	case "release-restore":
		if *dir == "" || !*confirm {
			return true, fmt.Errorf("--data-dir and --confirm-other-runners-stopped required; restored watches remain paused")
		}
		err = cloudbackup.Release(ctx, *dir)
	}
	if err != nil {
		return true, err
	}
	fmt.Fprintf(out, "%s completed in %s.\n", args[0], time.Since(started).Round(time.Millisecond))
	if args[0] == "backup" {
		if info, e := os.Stat(*output); e == nil {
			fmt.Fprintf(out, "Encrypted bytes: %d. Keep the backup identity and source-credential master key separately.\n", info.Size())
		}
	}
	if args[0] == "restore" {
		fmt.Fprintln(out, "Quarantined: watches paused, stale deliveries canceled, sessions revoked. Review ownership before release-restore; release does not resume watches.")
	}
	return true, nil
}

// ReadPassword never accepts a secret as a command-line argument.
func ReadPassword(path string) (string, error) {
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
