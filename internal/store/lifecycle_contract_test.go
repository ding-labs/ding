package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

func lifecycleContract(t *testing.T, tx *Tx) {
	t.Helper()
	stateContract(t, tx)
	c, d := fixture(t)
	timer, err := tx.Timer("health", "group", "missing")
	if err != nil || string(timer.Body) != `{"test":true}` {
		t.Fatal("timer mismatch", err)
	}
	if rows, err := tx.DueTimers(testNow, 100); err != nil || len(rows) != 1 {
		t.Fatal("due timers mismatch", err)
	}
	stale := timer
	stale.Due = stale.Due.Add(time.Second)
	if err = tx.DeleteTimer(stale); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Timer("health", "group", "missing"); err != nil {
		t.Fatal("stale timer removed current timer")
	}
	if err = tx.DeleteTimer(timer); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Timer("health", "group", "missing"); err != ErrNotFound {
		t.Fatal("timer deletion failed", err)
	}
	if err = tx.SaveDestination(d, testNow); err != nil {
		t.Fatal(err)
	}
	first, _, err := tx.Receipt("health", 1, "batch", testNow)
	if err != nil {
		t.Fatal(err)
	}
	event := watch.Event{ID: "pinned-event", WatchID: "health", Revision: c.Revision, At: testNow, Type: "firing", Evidence: []int64{first}}
	if _, err = tx.AppendEvent(event); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Enqueue(Intent{EventID: event.ID, WatchID: "health", DestinationID: "ops", DestinationRevision: d.Revision, Payload: []byte(`{}`), NextAt: testNow, CreatedAt: testNow}); err != nil {
		t.Fatal(err)
	}
	// Pins and pending outbox references must survive an otherwise expired cutoff.
	cutoff := testNow.Add(24 * time.Hour)
	now := cutoff.Add(time.Hour)
	if err = tx.Retain(cutoff, now, []int64{first}, []string{event.ID}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = tx.sql.QueryRow("SELECT count(*) FROM observations").Scan(&n); err != nil || n != 1 {
		t.Fatal("retention lost pinned input or retained ordinary input", err)
	}
	if err = tx.sql.QueryRow("SELECT count(*) FROM events").Scan(&n); err != nil || n != 1 {
		t.Fatal("retention lost pending event", err)
	}
	if err = tx.CancelDeliveries("health", now); err != nil {
		t.Fatal(err)
	}
	if err = tx.Retain(cutoff, now, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err = tx.sql.QueryRow("SELECT count(*) FROM events").Scan(&n); err != nil || n != 0 {
		t.Fatal("released event not reclaimed", err)
	}
	if err = tx.sql.QueryRow("SELECT count(*) FROM observations").Scan(&n); err != nil || n != 0 {
		t.Fatal("released evidence not reclaimed", err)
	}
}
func TestLifecycleContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { lifecycleContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		tx := postgresScopedTx(t, db, fmt.Sprintf("retention-%d", time.Now().UnixNano()))
		if err := initializePostgresPush(tx); err != nil {
			t.Fatal(err)
		}
		lifecycleContract(t, tx)
		if err := tx.sql.Commit(); err != nil {
			t.Fatal(err)
		}
		var gone bool
		if err := db.QueryRow("SELECT to_regclass('pg_temp.retained_inputs') IS NULL").Scan(&gone); err != nil || !gone {
			t.Fatal("temporary pins leaked past transaction", err)
		}
	})
}
