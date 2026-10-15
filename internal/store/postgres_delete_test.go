package store

import (
	"context"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresWorkspaceDeletionIsScopedAndRepeatable(t *testing.T) {
	f := pgtest.New(t)
	ctx := context.Background()
	if err := MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	ids := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
	for _, id := range ids {
		s, err := OpenPostgres(ctx, f.App, id)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Update(ctx, func(tx *Tx) error { inspectionContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
	for i := 0; i < 2; i++ {
		if err := DeletePostgresWorkspace(ctx, f.App, ids[0], nil); err != nil {
			t.Fatal(err)
		}
	}
	s, err := OpenPostgres(ctx, f.App, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.View(ctx, func(tx *Tx) error {
		rows, err := tx.Events("health", 0, 100)
		if err == nil && len(rows) != 1 {
			t.Error("deletion affected another workspace")
		}
		checkPostgresLogicalBytes(t, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := f.App.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT set_config('ding.workspace',$1,true)", ids[0]); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM ding_runtime.workspaces").Scan(&n); err != nil || n != 0 {
		t.Fatal("workspace registry remained", err)
	}
}
