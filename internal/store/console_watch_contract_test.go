package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func consoleWatchContract(t *testing.T, tx *Tx) {
	t.Helper()
	c, d := fixture(t)
	if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
		t.Fatal(err)
	}
	if err := tx.SaveDestination(d, testNow); err != nil {
		t.Fatal(err)
	}
	q := ConsoleQuery{Limit: 1, Attention: "yes"}
	page, err := tx.ConsoleWatches(q, map[string][]string{"health": {"API_KEY"}})
	if err != nil || page.Total != 1 || page.All != 1 || page.Attention != 1 || len(page.Watches) != 1 || page.Watches[0].Missing[0] != "API_KEY" {
		t.Fatalf("attention page mismatch %+v: %v", page, err)
	}
	page, err = tx.ConsoleWatches(ConsoleQuery{Limit: 1, Search: "absent"}, nil)
	if err != nil || page.Total != 0 || page.All != 1 || len(page.Watches) != 0 {
		t.Fatal("search mismatch", err)
	}
	page, err = tx.ConsoleWatches(ConsoleQuery{Limit: 1, Destination: "ops", Source: "http"}, nil)
	if err != nil || page.Total != 1 || len(page.Watches) != 1 {
		t.Fatal("source/destination filter mismatch", err)
	}
}
func TestConsoleWatchContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { consoleWatchContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		tx := postgresScopedTx(t, db, fmt.Sprintf("console-%d", time.Now().UnixNano()))
		if _, err := tx.sql.Exec("INSERT INTO metadata(key,value) VALUES('store_id','console-store')"); err != nil {
			t.Fatal(err)
		}
		consoleWatchContract(t, tx)
		if err := tx.sql.Commit(); err != nil {
			t.Fatal(err)
		}
	})
}
