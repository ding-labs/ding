package store

import (
	"context"
	"database/sql"
	"testing"
)

func postgresScopedTx(t *testing.T, db *sql.DB, workspace string) *Tx {
	t.Helper()
	ctx := context.Background()
	raw, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Rollback() })
	if _, err = raw.Exec("SET LOCAL search_path=ding_runtime,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec("SELECT set_config('ding.workspace',$1,true)", workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec("INSERT INTO workspaces(workspace_id) VALUES($1) ON CONFLICT DO NOTHING", workspace); err != nil {
		t.Fatal(err)
	}
	return &Tx{ctx: ctx, sql: raw, postgres: true, writable: true}
}

func TestPostgresWatchAndDestinationContract(t *testing.T) {
	ctx := context.Background()
	migration := postgresTestPool(t, "migration")
	if err := MigratePostgres(ctx, migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	db := postgresTestPool(t, "app")
	c, d := fixture(t)
	// The same watch and destination IDs must coexist in different workspaces.
	for _, workspace := range []string{"records-a", "records-b"} {
		tx := postgresScopedTx(t, db, workspace)
		if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow, Cursor: workspace}, testNow); err != nil {
			t.Fatal(err)
		}
		if err := tx.SaveDestination(d, testNow); err != nil {
			t.Fatal(err)
		}
		if err := tx.sql.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	tx := postgresScopedTx(t, db, "records-a")
	r, err := tx.Watch("health")
	if err != nil {
		t.Fatal(err)
	}
	if r.Cursor != "records-a" || len(r.Lifetime) != 64 {
		t.Fatal("identity or lifetime mismatch")
	}
	lifetime := r.Lifetime
	if err = tx.SaveWatch(r, testNow); err != nil {
		t.Fatal(err)
	}
	if r, err = tx.Watch("health"); err != nil || r.Lifetime != lifetime {
		t.Fatal("ordinary update changed lifetime")
	}
	r.Status = "deleted"
	if err = tx.SaveWatch(r, testNow); err != nil {
		t.Fatal(err)
	}
	r.Status = "running"
	if err = tx.SaveWatch(r, testNow); err != nil {
		t.Fatal(err)
	}
	if r, err = tx.Watch("health"); err != nil || r.Lifetime == lifetime {
		t.Fatal("recreated watch reused producer lifetime")
	}
	for _, revision := range []string{"", d.Revision} {
		got, err := tx.Destination("ops", revision)
		if err != nil || got.Revision != d.Revision {
			t.Fatal("destination mismatch", err)
		}
	}
	if rows, err := tx.Watches(); err != nil || len(rows) != 1 {
		t.Fatal("workspace leaked watches", err)
	}
	if err = tx.Checkpoint("health", 999, testNow, testNow, "wrong"); err != ErrStale {
		t.Fatal("stale checkpoint accepted", err)
	}
	if err = tx.Checkpoint("health", 1, testNow, testNow, "rolled-back"); err != nil {
		t.Fatal(err)
	}
	if err = tx.sql.Rollback(); err != nil {
		t.Fatal(err)
	}
	next := postgresScopedTx(t, db, "records-a")
	if r, err = next.Watch("health"); err != nil || r.Cursor != "records-a" || r.Lifetime != lifetime {
		t.Fatal("rollback lost original state")
	}
}
