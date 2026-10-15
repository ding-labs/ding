package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

func stateContract(t *testing.T, tx *Tx) {
	t.Helper()
	c, _ := fixture(t)
	if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
		t.Fatal(err)
	}
	first, err := tx.AppendObservation("health", c.Revision, 1, watch.Observation{InputID: "first", AcceptedAt: testNow, Fields: map[string]any{"typed": 42}})
	if err != nil {
		t.Fatal(err)
	}
	last, err := tx.AppendObservation("health", c.Revision, 1, watch.Observation{InputID: "last", AcceptedAt: testNow})
	if err != nil || last != first+1 {
		t.Fatal("noncontiguous batch", err)
	}
	if err = tx.SaveReceipt("health", 1, "batch", first, last, testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	a, b, err := tx.Receipt("health", 1, "batch", testNow)
	if err != nil || a != first || b != last {
		t.Fatal("receipt range mismatch", err)
	}
	if _, _, err = tx.Receipt("health", 1, "batch", testNow.Add(time.Hour)); err != ErrNotFound {
		t.Fatal("expiry boundary mismatch", err)
	}
	if err = tx.SaveEntity("health", "group", c.Revision, map[string]any{"matches": 2}, testNow); err != nil {
		t.Fatal(err)
	}
	if n, err := tx.EntityCount("health"); err != nil || n != 1 {
		t.Fatal("entity count mismatch", err)
	}
	body, err := tx.Entity("health", "group")
	if err != nil || string(body) != `{"matches":2}` {
		t.Fatal("entity bytes changed", err)
	}
	if rows, err := tx.Entities("health"); err != nil || len(rows) != 1 || string(rows["group"]) != string(body) {
		t.Fatal("entity listing mismatch", err)
	}
	if err = tx.SaveTimer(Timer{WatchID: "health", Entity: "group", Kind: "missing", Generation: 1, Due: testNow, Body: json.RawMessage(`{"test":true}`)}); err != nil {
		t.Fatal(err)
	}
}

func TestStateContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { stateContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		// Separate workspaces may return identical local ranges. Rollback must restore
		// the allocator too; unlike nextval(), it cannot consume uncommitted IDs.
		id := fmt.Sprintf("state-%d", time.Now().UnixNano())
		for _, suffix := range []string{"a", "b"} {
			tx := postgresScopedTx(t, db, id+suffix)
			stateContract(t, tx)
			if err := tx.sql.Rollback(); err != nil {
				t.Fatal(err)
			}
			tx = postgresScopedTx(t, db, id+suffix)
			stateContract(t, tx)
			var high int64
			if err := tx.sql.QueryRow("SELECT observation_sequence FROM workspaces").Scan(&high); err != nil || high != 2 {
				t.Fatal("allocator leaked rolled-back range", err)
			}
			if err := tx.sql.Commit(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
