package database_test

import (
	"context"
	"errors"
	"testing"

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
