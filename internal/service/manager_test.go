package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerOwnsOnlyExactDefinition(t *testing.T) {
	dir := t.TempDir()
	r := record()
	r.StateDir = filepath.Join(dir, "state")
	d, err := DefinitionFor("linux", dir, dir, "1000", r)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	m := Manager{OS: "linux", UID: "1000", Definition: d, Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return []byte("active\n"), nil
	}}
	ctx := context.Background()
	if s := m.Inspect(ctx); s.Installed || s.State != "not-installed" {
		t.Fatal(s)
	}
	if err := m.Install(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Action(ctx, "start"); err != nil {
		t.Fatal(err)
	}
	if s := m.Inspect(ctx); !s.Installed || s.State != "running" {
		t.Fatal(s)
	}
	if err := os.WriteFile(d.Path, []byte("user-owned unit"), 0600); err != nil {
		t.Fatal(err)
	}
	before := len(calls)
	if err := m.Install(ctx); err == nil {
		t.Fatal("overwrote foreign unit")
	}
	if err := m.Action(ctx, "stop"); err == nil {
		t.Fatal("stopped foreign unit")
	}
	if err := m.Uninstall(ctx); err == nil {
		t.Fatal("removed foreign unit")
	}
	if len(calls) != before {
		t.Fatal("called manager for foreign unit")
	}
	if err := os.WriteFile(d.Path, []byte(d.Content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(r.StateDir, 0700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(r.StateDir, "important.db")
	if err := os.WriteFile(keep, []byte("state"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("uninstall lost state", err)
	}
}

func TestManagerFailurePreservesRegistration(t *testing.T) {
	dir := t.TempDir()
	r := record()
	r.StateDir = dir
	d, err := DefinitionFor("linux", dir, dir, "1000", r)
	if err != nil {
		t.Fatal(err)
	}
	m := Manager{OS: "linux", Definition: d, Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("manager unavailable")
	}}
	if err := m.Install(context.Background()); err == nil {
		t.Fatal("claimed install succeeded")
	}
	if err := m.Uninstall(context.Background()); err == nil {
		t.Fatal("claimed uninstall succeeded")
	}
	if _, err := os.Stat(d.Path); err != nil {
		t.Fatal("deleted registration after failed stop", err)
	}
}
