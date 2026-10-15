package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/watch"
)

func eventContract(t *testing.T, tx *Tx) {
	t.Helper()
	c, d := fixture(t)
	if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
		t.Fatal(err)
	}
	if err := tx.SaveDestination(d, testNow); err != nil {
		t.Fatal(err)
	}
	seq, err := tx.AppendObservation("health", c.Revision, 1, watch.Observation{InputID: "event-input", AcceptedAt: testNow})
	if err != nil {
		t.Fatal(err)
	}
	event := watch.Event{ID: "same-event", WatchID: "health", Revision: c.Revision, At: testNow, Type: "firing", Evidence: []int64{seq}}
	eventSeq, err := tx.AppendEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	id, err := tx.Enqueue(Intent{EventID: event.ID, WatchID: "health", DestinationID: "ops", DestinationRevision: d.Revision, Payload: []byte(`{"id":"same-event"}`), NextAt: testNow, CreatedAt: testNow})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := tx.Intent(id)
	if err != nil || intent.EventID != event.ID || intent.Destination.Revision != d.Revision {
		t.Fatal("intent identity mismatch", err)
	}
	for _, watchID := range []string{"", "health"} {
		rows, err := tx.Events(watchID, 0, 100)
		if err != nil || len(rows) != 1 || rows[0].Sequence != eventSeq || rows[0].Evidence[0] != seq {
			t.Fatal("event or evidence mismatch", err)
		}
	}
	var claim int64
	if err = tx.sql.QueryRowContext(tx.ctx, tx.query(qClaim), timestamp(testNow), timestamp(testNow), timestamp(testNow), timestamp(testNow)).Scan(&claim); err != nil || claim != id {
		t.Fatal("claim query mismatch", err)
	}
	if _, err = tx.exec(qLeaseIntent, "token", timestamp(testNow.Add(time.Second)), id); err != nil {
		t.Fatal(err)
	}
	result, err := tx.exec(qFinishIntent, "delivered", timestamp(testNow), "ok", id, "stale-token")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := result.RowsAffected(); n != 0 {
		t.Fatal("stale acknowledgement accepted")
	}
	result, err = tx.exec(qFinishIntent, "delivered", timestamp(testNow), "ok", id, "token")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		t.Fatal("current acknowledgement rejected")
	}
}

func TestEventContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { eventContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		for _, suffix := range []string{"a", "b"} {
			tx := postgresScopedTx(t, db, fmt.Sprintf("event-%d-%s", time.Now().UnixNano(), suffix))
			eventContract(t, tx)
			if err := tx.sql.Commit(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
