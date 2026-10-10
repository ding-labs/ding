package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/install"
)

func TestRecoveryAtReplacementBoundaries(t *testing.T) {
	for _, phase := range []string{"prepared", "old-moved", "replaced", "record-updated", "committed", "rollback-interrupted"} {
		t.Run(phase, func(t *testing.T) {
			root, state := t.TempDir(), t.TempDir()
			exe := filepath.Join(root, "ding")
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(os.WriteFile(exe, []byte("old"), 0700))
			before, err := install.Inspect(exe, state, "1.0.0", "standalone")
			must(err)
			must(install.Create(before))
			stage, err := os.MkdirTemp(root, ".ding-update-")
			must(err)
			candidate := filepath.Join(stage, "ding")
			must(os.WriteFile(candidate, []byte("new"), 0700))
			after, err := install.Inspect(candidate, state, "1.1.0", "standalone")
			must(err)
			after.Executable = before.Executable
			j := journal{Phase: "prepared", Stage: stage, Before: before, After: after, Assets: []asset{{exe, candidate, filepath.Join(stage, "previous-0")}}}
			must(install.AtomicJSON(journalPath(state), j))
			must(os.WriteFile(filepath.Join(state, "ding.db"), []byte("new committed watch evidence"), 0600))
			if phase != "prepared" {
				must(os.Rename(exe, j.Assets[0].Previous))
			}
			if phase != "prepared" && phase != "old-moved" {
				must(os.Rename(candidate, exe))
			}
			if phase == "record-updated" || phase == "committed" {
				must(install.Replace(after))
			}
			if phase == "committed" {
				j.Phase = "committed"
				must(install.AtomicJSON(journalPath(state), j))
			}
			if phase == "rollback-interrupted" {
				must(os.Remove(exe))
				must(os.Rename(j.Assets[0].Previous, exe))
			}
			starts := 0
			hooks := Hooks{Stop: func(context.Context) error { return nil }, Start: func(context.Context) error {
				starts++
				if phase == "rollback-interrupted" && starts == 1 {
					return errors.New("temporary readiness failure")
				}
				return nil
			}}
			err = Recover(context.Background(), state, hooks)
			if phase == "rollback-interrupted" {
				if err == nil {
					t.Fatal("claimed recovery despite failed readiness")
				}
				if _, err := os.Stat(journalPath(state)); err != nil {
					t.Fatal("lost recovery journal", err)
				}
				err = Recover(context.Background(), state, hooks)
			}
			must(err)
			want := "old"
			if phase == "committed" {
				want = "new"
			}
			got, err := os.ReadFile(exe)
			must(err)
			if string(got) != want {
				t.Fatal("incorrect executable after recovery", string(got))
			}
			data, err := os.ReadFile(filepath.Join(state, "ding.db"))
			must(err)
			if string(data) != "new committed watch evidence" {
				t.Fatal("recovery rolled back user evidence")
			}
		})
	}
}
