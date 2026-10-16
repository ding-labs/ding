package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresLostOwnerRollsBackAcceptance(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	if err := MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	owner, err := database.TakeOwnership(ctx, f.NewAppPool(1))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	workspace := strings.Repeat("a", 64)
	s, err := OpenPostgres(ctx, f.App, workspace, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seed(t, s)
	c, _ := fixture(t)
	err = s.Update(ctx, func(tx *Tx) error {
		if err := tx.SaveEntity("health", "uncommitted", c.Revision, map[string]bool{"mutated": true}, testNow); err != nil {
			return err
		}
		return owner.Close()
	})
	if !errors.Is(err, database.ErrOwnershipLost) {
		t.Fatal("lost owner committed state", err)
	}
	replacement, err := database.TakeOwnership(ctx, f.NewAppPool(1))
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	next, err := OpenPostgres(ctx, f.App, workspace, replacement)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err = next.View(ctx, func(tx *Tx) error {
		_, err := tx.Entity("health", "uncommitted")
		if err != ErrNotFound {
			t.Fatal("stale owner state survived", err)
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
