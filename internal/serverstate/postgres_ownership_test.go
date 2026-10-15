package state

import (
	"context"
	"errors"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresLostOwnerCannotCommitControlMutation(t *testing.T) {
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
	db, err := OpenPostgres(ctx, f.App, owner)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, tx.query(qConfigureHosting1), "self-hosted"); err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); !errors.Is(err, database.ErrOwnershipLost) {
		t.Fatal("stale owner committed control mutation", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = f.App.QueryRow("SELECT count(*) FROM ding_control.settings WHERE key='hosting'").Scan(&n); err != nil || n != 0 {
		t.Fatal("stale control write survived", err)
	}
}
