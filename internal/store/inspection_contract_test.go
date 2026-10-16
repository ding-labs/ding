package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func inspectionContract(t *testing.T, tx *Tx) {
	t.Helper()
	stateContract(t, tx)
	eventContract(t, tx)
	if page, err := tx.EventPage("health", "", 10); err != nil || len(page.Events) != 1 || page.Cursor == "" {
		t.Fatal("event page mismatch", err)
	}
	event, err := tx.Event("same-event")
	if err != nil {
		t.Fatal(err)
	}
	if inputs, err := tx.EvidenceInputs(event.ID, 0); err != nil || len(inputs.Observations) != 1 || inputs.Observations[0].Sequence != event.Evidence[0] {
		t.Fatal("evidence range mismatch", err)
	}
	if err = tx.SaveReplay(event.ID, map[string]any{"checkpoint": "bytes"}); err != nil {
		t.Fatal(err)
	}
	if raw, err := tx.Replay(event.ID); err != nil || string(raw) != `{"checkpoint":"bytes"}` {
		t.Fatal("replay bytes mismatch", err)
	}
	if def, err := tx.Definition("health", event.Revision); err != nil || def.Metadata.ID != "health" {
		t.Fatal("definition mismatch", err)
	}
	if summary, err := tx.InspectSummary("health", "", 0); err != nil || len(summary.Entities) != 1 || summary.Entities[0].Matches != 2 || len(summary.Deliveries) != 1 {
		t.Fatal("inspection summary mismatch", err)
	}
	if unhealthy, open, err := tx.EntityHealth("health"); err != nil || unhealthy != 0 || open != 0 {
		t.Fatal("health mismatch", err)
	}
	if counts, err := tx.DeliveryCounts(); err != nil || counts["delivered"] != 1 {
		t.Fatal("delivery counts mismatch", err)
	}
	if destinations, err := tx.Destinations(); err != nil || len(destinations) != 1 {
		t.Fatal("destination inspection mismatch", err)
	}
	if d, err := tx.Delivery(1, 0); err != nil || d.Intent.EventID != event.ID || len(d.Attempts) != 0 {
		t.Fatal("delivery inspection mismatch", err)
	}
}
func TestInspectionContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { inspectionContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		tx := postgresScopedTx(t, db, fmt.Sprintf("inspection-%d", time.Now().UnixNano()))
		if _, err := tx.sql.Exec("INSERT INTO metadata(key,value) VALUES('store_id','inspection-store')"); err != nil {
			t.Fatal(err)
		}
		inspectionContract(t, tx)
		if err := tx.sql.Commit(); err != nil {
			t.Fatal(err)
		}
	})
}
