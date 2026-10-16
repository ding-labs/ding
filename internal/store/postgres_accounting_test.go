package store

import (
	"context"
	"testing"
)

var postgresAccountedTables = []string{"metadata", "watch_revisions", "watches", "destination_revisions", "destinations", "observations", "entities", "input_receipts", "timers", "events", "event_evidence", "event_replays", "outbox", "delivery_attempts", "audit", "integration_grants", "integration_previews", "integration_operations", "handoffs", "push_receipts", "push_usage", "push_last", "push_receipt_stats"}

func checkPostgresLogicalBytes(t *testing.T, tx *Tx) {
	t.Helper()
	if _, err := tx.sql.Exec("SET LOCAL bytea_output='hex'"); err != nil {
		t.Fatal(err)
	}
	var actual int64
	for _, table := range postgresAccountedTables {
		var n int64
		if err := tx.sql.QueryRow("SELECT coalesce(sum(octet_length((to_jsonb(r)-'workspace_id')::text)),0) FROM " + table + " r").Scan(&n); err != nil {
			t.Fatal(err)
		}
		actual += n
	}
	var accounted int64
	if err := tx.sql.QueryRow("SELECT logical_bytes FROM workspaces").Scan(&accounted); err != nil {
		t.Fatal(err)
	}
	if actual != accounted {
		t.Fatalf("logical bytes mismatch actual=%d counter=%d", actual, accounted)
	}
}

func TestPostgresLogicalAccountingIgnoresSessionEncoding(t *testing.T) {
	s := openPostgresTestStore(t)
	ctx := context.Background()
	if err := s.Update(ctx, func(tx *Tx) error {
		stateContract(t, tx)
		if _, err := tx.sql.Exec("SET LOCAL bytea_output='escape'"); err != nil {
			return err
		}
		c, _ := fixture(t)
		if err := tx.SaveEntity("health", "group", c.Revision, map[string]any{"encoding": "\\x00\\x7f"}, testNow); err != nil {
			return err
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.View(ctx, func(tx *Tx) error { checkPostgresLogicalBytes(t, tx); return nil }); err != nil {
		t.Fatal(err)
	}
}
