package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/watch"
)

func consoleReadContract(t *testing.T, tx *Tx) {
	t.Helper()
	inspectionContract(t, tx)
	first, err := tx.ConsoleEvents(ConsoleQuery{Watch: "health", Type: "firing", From: testNow.Format(time.RFC3339), To: testNow.Format(time.RFC3339), Limit: 1})
	if err != nil || first.Total != 1 || len(first.Events) != 1 {
		t.Fatal("filtered event page mismatch", err)
	}
	c, _ := fixture(t)
	if _, err = tx.AppendEvent(watch.Event{ID: "new-event", WatchID: "health", Revision: c.Revision, At: testNow, Type: "recovery", Message: "Newest"}); err != nil {
		t.Fatal(err)
	}
	if live, err := tx.EventPage("health", first.FollowCursor, 10); err != nil || len(live.Events) != 1 || live.Events[0].ID != "new-event" {
		t.Fatal("follow cursor mismatch", err)
	}
	if page, err := tx.ConsoleEvents(ConsoleQuery{Limit: 1}); err != nil || page.Total != 2 || !page.More || page.Events[0].Message != "Newest" {
		t.Fatal("event projection mismatch", err)
	}
	if page, err := tx.ConsoleDeliveries(ConsoleQuery{Limit: 1, Status: "delivered", Destination: "ops"}); err != nil || page.Total != 1 || len(page.Deliveries) != 1 {
		t.Fatal("delivery page mismatch", err)
	}
	if page, err := tx.ConsoleDestinations(ConsoleQuery{Limit: 1}); err != nil || len(page.Destinations) != 1 || page.Destinations[0].Watches != 1 {
		t.Fatal("destination page mismatch", err)
	}
	if refs, err := tx.SecretNames(); err != nil || len(refs) != 0 {
		t.Fatal("unexpected secret reference", err)
	}
	def := c.Definition
	def.Spec.Source.URL = ""
	def.Spec.Source.URLRef = &watch.SecretRef{Env: "SOURCE_KEY"}
	changed, err := plan.Compile(def)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.SaveWatch(WatchRecord{Plan: changed, Generation: 2, Status: "running", NextAt: testNow}, testNow); err != nil {
		t.Fatal(err)
	}
	refs, err := tx.SecretNames()
	if err != nil || len(refs["health"]) != 1 || refs["health"][0] != "SOURCE_KEY" {
		t.Fatal("source reference traversal mismatch", err)
	}
}
func TestConsoleReadContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { consoleReadContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		tx := postgresScopedTx(t, db, fmt.Sprintf("console-read-%d", time.Now().UnixNano()))
		if _, err := tx.sql.Exec("INSERT INTO metadata(key,value) VALUES('store_id','console-read-store')"); err != nil {
			t.Fatal(err)
		}
		consoleReadContract(t, tx)
		if err := tx.sql.Commit(); err != nil {
			t.Fatal(err)
		}
	})
}
