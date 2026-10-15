package cloud

import (
	"context"
	"errors"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresOfflineMigrationAndInitialization(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	f.SetAppEnvironment(t, "DING_TEST_APP_DATABASE")
	f.SetMigrationEnvironment(t, "DING_TEST_MIGRATION_DATABASE")
	c := Config{DataDir: t.TempDir(), Database: database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_TEST_APP_DATABASE"}, MaxOpenConns: 1}}
	if _, err := OpenServerStorage(ctx, c.DataDir, c.Database); err == nil {
		t.Fatal("unmigrated database started")
	}
	if err := MigrateDatabase(ctx, c, "DING_TEST_MIGRATION_DATABASE"); err != nil {
		t.Fatal(err)
	}
	storage, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	if err = MigrateDatabase(ctx, c, "DING_TEST_MIGRATION_DATABASE"); !errors.Is(err, database.ErrOwned) {
		t.Fatal("migration ran while server owned the instance", err)
	}
	if err = storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(ctx, c, "DING_TEST_MIGRATION_DATABASE"); err != nil {
		t.Fatal("repeat migration failed", err)
	}
}
