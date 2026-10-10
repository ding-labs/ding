package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ding-labs/ding/internal/install"
)

func TestUpdateCommitAndReadinessRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses the signed installer")
	}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			root, state := t.TempDir(), t.TempDir()
			binary := filepath.Join(root, "ding")
			if err := os.WriteFile(binary, []byte("old"), 0700); err != nil {
				t.Fatal(err)
			}
			r, err := install.Inspect(binary, state, "1.0.0", "standalone")
			if err != nil {
				t.Fatal(err)
			}
			if err := install.Create(r); err != nil {
				t.Fatal(err)
			}
			stage, err := os.MkdirTemp(root, ".ding-update-")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, "ding"), []byte("new"), 0700); err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS == "darwin" {
				for _, dir := range []string{root, stage} {
					if err := os.Mkdir(filepath.Join(dir, "DingNotifications.app"), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			backed, stopped, starts := false, false, 0
			hooks := Hooks{
				Backup: func(_ context.Context, path string) error {
					backed = true
					return os.WriteFile(path, []byte("database"), 0600)
				},
				Stop: func(context.Context) error {
					if !backed {
						t.Fatal("stopped before backup")
					}
					stopped = true
					return nil
				},
				Start: func(context.Context) error {
					if !stopped {
						t.Fatal("started before stop")
					}
					starts++
					if fail && starts == 1 {
						return errors.New("not ready")
					}
					return nil
				},
			}
			err = Apply(context.Background(), r, "1.1.0", stage, hooks)
			if (err != nil) != fail {
				t.Fatalf("apply = %v", err)
			}
			want := "new"
			if fail {
				want = "old"
			}
			data, err := os.ReadFile(binary)
			if err != nil || string(data) != want {
				t.Fatalf("binary = %q, %v", data, err)
			}
			got, err := install.Load(state)
			if err != nil {
				t.Fatal(err)
			}
			wantVersion := "1.1.0"
			if fail {
				wantVersion = "1.0.0"
			}
			if got.Version != wantVersion {
				t.Fatal(got.Version)
			}
			if _, err := os.Stat(journalPath(state)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("journal remains", err)
			}
			backups, err := filepath.Glob(filepath.Join(state, "backups", "*.db"))
			if err != nil || len(backups) != 1 {
				t.Fatal("backup missing")
			}
		})
	}
}
