// Package watchcli presents the versioned watch API; runtime services stay out
// of command handlers so agents and the local API can share their contracts.
package watchcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
	"github.com/spf13/cobra"
)

func Root(version string) *cobra.Command {
	root := &cobra.Command{Use: "ding-watch", Short: "Persistent watches for developers and agents (experimental)", Version: version, SilenceUsage: true, SilenceErrors: true}
	for _, name := range []string{"validate", "explain"} {
		var structured bool
		cmd := &cobra.Command{Use: name + " FILE", Short: "Compile a watch manifest without starting I/O", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return Fail(cmd.ErrOrStderr(), structured, "read_failed", "cannot read manifest")
			}
			bundle, err := plan.Parse(data)
			if err != nil {
				return Fail(cmd.ErrOrStderr(), structured, "invalid_manifest", err.Error())
			}
			if structured {
				return Write(cmd.OutOrStdout(), bundle)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Valid: %d watches, %d destinations\n", len(bundle.Watches), len(bundle.Destinations))
			if cmd.Name() == "explain" {
				for _, w := range bundle.Watches {
					fmt.Fprintf(cmd.OutOrStdout(), "%s revision %s\n  Source: %s; policy: %s (%d matching / %d recovering)\n  Permissions: %v\n  Destination references must exist when applied.\n", w.Definition.Metadata.ID, w.Revision, w.Definition.Spec.Source.Type, w.Definition.Spec.Policy.Trigger, w.Definition.Spec.Policy.Consecutive, w.Definition.Spec.Policy.RecoverAfter, w.Permissions)
				}
			}
			return nil
		}}
		cmd.Flags().BoolVar(&structured, "json", false, "emit a versioned JSON response")
		root.AddCommand(cmd)
	}
	return root
}
func Write(w io.Writer, value any) error {
	return json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Data: value})
}
func Fail(w io.Writer, structured bool, code, message string) error {
	if structured {
		_ = json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Error: &watch.Error{Code: code, Message: message}})
	} else {
		fmt.Fprintln(w, message)
	}
	return &ReportedError{Code: code}
}

type ReportedError struct{ Code string }

func (e *ReportedError) Error() string { return e.Code }

// Execute keeps parse/argument failures in the same machine-readable contract.
func Execute(version string, args []string, stdout, stderr io.Writer) error {
	root := Root(version)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	if err == nil {
		return nil
	}
	var reported *ReportedError
	if errors.As(err, &reported) {
		return err
	}
	structured := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			structured = arg != "--json=false"
		}
	}
	return Fail(stderr, structured, "invalid_arguments", err.Error())
}
