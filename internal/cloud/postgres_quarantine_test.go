package cloud

import (
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresQuarantineSurvivesAnotherDataDirectory(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	f.SetAppEnvironment(t, "DING_QUARANTINE_APP")
	f.SetMigrationEnvironment(t, "DING_QUARANTINE_ADMIN")
	c := Config{DataDir: t.TempDir(), Database: database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_QUARANTINE_APP"}, MaxOpenConns: 1}}
	if err := MigrateDatabase(ctx, c, "DING_QUARANTINE_ADMIN"); err != nil {
		t.Fatal(err)
	}
	s, err := OpenServerStorage(ctx, c.DataDir, c.Database)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.SetQuarantine(ctx, s.pool, s.owner, true); err != nil {
		t.Fatal(err)
	}
	s.Close()
	next, err := OpenServerStorage(ctx, t.TempDir(), c.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err = next.CheckQuarantine(ctx); err == nil {
		t.Fatal("changing local artifacts bypassed restore quarantine")
	}
}
