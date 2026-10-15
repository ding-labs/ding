package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func assertPushStats(t *testing.T, s *Store, total, expired, earliest int64) {
	t.Helper()
	var gotTotal, gotExpired, gotEarliest int64
	if err := s.db.QueryRow("SELECT total,expired,earliest FROM push_receipt_stats").Scan(&gotTotal, &gotExpired, &gotEarliest); err != nil {
		t.Fatal(err)
	}
	if gotTotal != total || gotExpired != expired || gotEarliest != earliest {
		t.Fatal(gotTotal, gotExpired, gotEarliest, "want", total, expired, earliest)
	}
}

func TestPushStatsMigrationBackfillsExistingReceipts(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "ding.db"))
	if err != nil {
		t.Fatal(err)
	}
	schemas := []string{initialSchema, inspectionSchema, queueIndexSchema, integrationSchema, handoffSchema, auditSchema, pushSchema}
	if _, err := db.Exec(strings.Join(schemas, ";\n") + `
INSERT INTO push_receipts VALUES('life','producer','old','digest','{}',-1);
INSERT INTO push_receipts VALUES('life','producer','next','digest','{}',20);
PRAGMA user_version=7;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s := openTest(t, dir)
	assertPushStats(t, s, 2, 1, -1)
}

func TestPushStatsTriggersTrackReuseExpiryDeletionAndRollback(t *testing.T) {
	s := openTest(t, t.TempDir())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	insert := "INSERT INTO push_receipts VALUES('life','producer',?,'digest','{}',?) ON CONFLICT(lifetime,producer,input_key) DO UPDATE SET expires_at=excluded.expires_at"
	exec(insert, "a", 10)
	exec(insert, "b", 20)
	exec("UPDATE push_receipt_stats SET expired=1,checked_at=10")
	assertPushStats(t, s, 2, 1, 10)
	exec("DELETE FROM push_receipts WHERE input_key='a'")
	assertPushStats(t, s, 1, 0, 20)
	exec("UPDATE push_receipts SET expires_at=5 WHERE input_key='b'")
	assertPushStats(t, s, 1, 1, 5)
	exec(insert, "b", 30)
	assertPushStats(t, s, 1, 0, 30)
	abort := errors.New("abort receipt")
	if err := s.Update(context.Background(), func(tx *Tx) error {
		if _, err := tx.sql.Exec(insert, "c", 1); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	assertPushStats(t, s, 1, 0, 30)
	exec("DELETE FROM push_receipts")
	assertPushStats(t, s, 0, 0, 0)
}
