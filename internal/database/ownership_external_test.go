package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestOwnershipExcludesReplacementAndFencesStaleWork(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, f.Migration, "runtime", []string{"CREATE SCHEMA ding_runtime;"}, "ding_app"); err != nil {
		t.Fatal(err)
	}
	owner, err := database.TakeOwnership(ctx, f.NewAppPool(1))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if other, err := database.TakeOwnership(ctx, f.NewAppPool(1)); !errors.Is(err, database.ErrOwned) {
		if other != nil {
			other.Close()
		}
		t.Fatal("competing owner admitted", err)
	}
	tx, err := f.App.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = owner.Fence(ctx, tx, true); err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err = owner.Fence(ctx, tx, true); !errors.Is(err, database.ErrOwnershipLost) {
		t.Fatal("stale owner authorized commit", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	replacement, err := database.TakeOwnership(ctx, f.NewAppPool(1))
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	tx, err = f.App.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = replacement.Fence(ctx, tx, true); err != nil {
		t.Fatal(err)
	}
}

func TestOwnershipDetectsTerminatedDatabaseSession(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, f.Migration, "runtime", []string{"CREATE SCHEMA ding_runtime;"}, "ding_app"); err != nil {
		t.Fatal(err)
	}
	owner, err := database.TakeOwnership(ctx, f.NewAppPool(1))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	var pid int
	if err = f.App.QueryRowContext(ctx, "SELECT pid FROM ding_meta.owner WHERE singleton=1").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	f.SetAdminEnvironment(t, "DING_OWNERSHIP_TEST_ADMIN")
	admin, err := database.OpenPostgres(ctx, database.Config{Backend: "postgres", DSNRef: &database.SecretRef{Env: "DING_OWNERSHIP_TEST_ADMIN"}, MaxOpenConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var terminated bool
	// A positive timeout waits for termination; the one-argument function only
	// confirms that PostgreSQL sent the signal, while the lock may still exist.
	if err = admin.QueryRowContext(ctx, "SELECT pg_terminate_backend($1,5000)", pid).Scan(&terminated); err != nil || !terminated {
		t.Fatal("could not terminate owned test session", err)
	}
	tx, err := f.App.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.Fence(ctx, tx, true); !errors.Is(err, database.ErrOwnershipLost) {
		t.Fatal("lost session authorized a write", err)
	}
	tx.Rollback()
	replacement, err := database.TakeOwnership(ctx, f.NewAppPool(1))
	if err != nil {
		t.Fatal("replacement could not acquire released lock", err)
	}
	defer replacement.Close()
	select {
	case <-owner.Context().Done():
	case <-time.After(8 * time.Second):
		t.Fatal("owner failed to cancel workers after session loss")
	}
}
