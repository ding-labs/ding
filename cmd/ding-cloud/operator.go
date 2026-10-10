package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"filippo.io/age"
	"github.com/ding-labs/ding/internal/cloudbackup"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

func operator(args []string, out io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "backup", "restore", "release-restore", "backup-keygen":
	default:
		return false, nil
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("data-dir", "", "absolute cloud data directory")
	output := flags.String("output", "", "new output file; never overwrites")
	input := flags.String("input", "", "encrypted backup to restore")
	recipient := flags.String("recipient", "", "age X25519 public recipient")
	identity := flags.String("identity-file", "", "owner-only age X25519 private identity file")
	confirm := flags.Bool("confirm-other-runners-stopped", false, "declare that execution ownership was checked across cloud and all transferred local copies")
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
