package store

import (
	"context"
	"testing"
)

func TestPostgresReopenRebuildsMissingReceiptStatsAndPreservesUsage(t *testing.T) {
	s := openPostgresTestStore(t)
	ctx := context.Background()
	if err := s.Update(ctx, func(tx *Tx) error {
		pushContract(t, tx)
		_, err := tx.sql.Exec("DELETE FROM push_receipt_stats")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := OpenPostgres(ctx, s.db, s.workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.View(ctx, func(tx *Tx) error {
		n, err := tx.PushReceiptCount(testNow)
		if err != nil || n != 1 {
			t.Fatal("missing stats reset receipt capacity", n, err)
		}
		usage, err := tx.PushUsage()
		if err != nil || usage.Requests != 3 {
			t.Fatal("reopen changed durable usage", err)
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Update(ctx, func(tx *Tx) error { _, err := tx.sql.Exec("DELETE FROM push_usage"); return err }); err != nil {
		t.Fatal(err)
	}
	reopened.Close()
	if next, err := OpenPostgres(ctx, s.db, s.workspace); err == nil {
		next.Close()
		t.Fatal("missing durable usage silently reset")
	}
}
