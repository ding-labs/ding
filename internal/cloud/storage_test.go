package cloud

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
)

func TestPostgresServerStorageUsesOneBackendAndOwnership(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	if err := state.MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	if err := store.MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	f.SetAppEnvironment(t, "DING_STORAGE_TEST_URL")
	config := database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_STORAGE_TEST_URL"}, MaxOpenConns: 1}
	dir := t.TempDir()
	storage, err := OpenServerStorage(ctx, dir, config)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	if other, err := OpenServerStorage(ctx, t.TempDir(), config); !errors.Is(err, database.ErrOwned) {
		if other != nil {
			other.Close()
		}
		t.Fatal("competing server admitted", err)
	}
	workspace, err := storage.Control.Enroll(ctx, "issuer", "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	s, err := storage.workspaces.Open(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if h, err := s.Health(ctx); err != nil || h.Backend != "postgres" {
		t.Fatal("workspace opened wrong backend", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "control.db")); !os.IsNotExist(err) {
		t.Fatal("PostgreSQL created SQLite control state")
	}
	if _, err = os.Stat(filepath.Join(dir, "workspaces")); !os.IsNotExist(err) {
		t.Fatal("PostgreSQL created workspace SQLite directories")
	}
	if _, err = OpenServerStorage(ctx, dir, database.Config{}); err == nil {
		t.Fatal("implicit switch to SQLite succeeded")
	}
}
