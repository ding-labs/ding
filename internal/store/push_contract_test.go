package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func initializePostgresPush(tx *Tx) error {
	_, err := tx.sql.Exec("INSERT INTO push_usage(singleton) VALUES(1) ON CONFLICT DO NOTHING; INSERT INTO push_receipt_stats(singleton,total,expired,checked_at,earliest) VALUES(1,0,0,0,0) ON CONFLICT DO NOTHING")
	return err
}

func pushContract(t *testing.T, tx *Tx) {
	t.Helper()
	c, _ := fixture(t)
	if err := tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
		t.Fatal(err)
	}
	r, err := tx.Watch("health")
	if err != nil {
		t.Fatal(err)
	}
	receipt := PushReceipt{AcceptanceID: "stable", WatchID: "health", First: 1, Last: 18, AcceptedAt: testNow, ExpiresAt: testNow.Add(time.Second)}
	if err = tx.SavePushReceipt(r.Lifetime, "producer", "key", "digest", receipt, 600000, 18); err != nil {
		t.Fatal(err)
	}
	if got, err := tx.PushReceipt(r.Lifetime, "producer", "key", "digest", testNow); err != nil || !got.Duplicate || got.AcceptanceID != receipt.AcceptanceID || got.Last != 18 {
		t.Fatal("receipt identity mismatch", err)
	}
	if _, err = tx.PushReceipt(r.Lifetime, "producer", "key", "changed", testNow); err != ErrIdempotency {
		t.Fatal("changed bytes accepted", err)
	}
	check := func(now time.Time, want int) {
		t.Helper()
		if tx.postgres {
			checkPostgresLogicalBytes(t, tx)
		}
		if got, err := tx.PushReceiptCount(now); err != nil || got != want {
			t.Fatalf("count=%d want=%d: %v", got, want, err)
		}
	}
	check(testNow, 1)
	check(receipt.ExpiresAt, 0)
	receipt.ExpiresAt = testNow.Add(time.Hour)
	if err = tx.SavePushReceipt(r.Lifetime, "producer", "key", "new-digest", receipt, 55, 1); err != nil {
		t.Fatal(err)
	}
	check(testNow.Add(2*time.Second), 1)
	receipt.ExpiresAt = testNow.Add(-time.Second)
	if err = tx.SavePushReceipt(r.Lifetime, "producer", "expired", "other", receipt, 50, 1); err != nil {
		t.Fatal(err)
	}
	check(testNow, 1)
	check(testNow.Add(-2*time.Second), 2)
	check(testNow, 1)
	if _, err = tx.exec(statement{sqlite: "DELETE FROM push_receipts WHERE expires_at<=?", postgres: "DELETE FROM push_receipts WHERE workspace_id=current_setting('ding.workspace') AND expires_at<=$1"}, timestamp(testNow)); err != nil {
		t.Fatal(err)
	}
	check(testNow, 1)
	usage, err := tx.PushUsage()
	if err != nil || usage.Requests != 3 || usage.Bytes != 600105 || usage.Observations != 20 {
		t.Fatalf("usage mismatch %+v: %v", usage, err)
	}
	if last, err := tx.LastPush(r.Lifetime, "health"); err != nil || last.AcceptanceID != "stable" {
		t.Fatal("last receipt mismatch", err)
	}
}

func TestPostgresPushCompletionRollsBackTogether(t *testing.T) {
	s := openPostgresTestStore(t)
	ctx := context.Background()
	c, _ := fixture(t)
	if err := s.Update(ctx, func(tx *Tx) error {
		return tx.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running", NextAt: testNow}, testNow)
	}); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("abort after receipt")
	var lifetime string
	if err := s.Update(ctx, func(tx *Tx) error {
		r, err := tx.Watch("health")
		if err != nil {
			return err
		}
		lifetime = r.Lifetime
		receipt := PushReceipt{AcceptanceID: "rolled-back", WatchID: "health", AcceptedAt: testNow, ExpiresAt: testNow.Add(time.Hour)}
		if err := tx.SavePushReceipt(lifetime, "producer", "key", "digest", receipt, 55, 1); err != nil {
			return err
		}
		checkPostgresLogicalBytes(t, tx)
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if err := s.View(ctx, func(tx *Tx) error {
		if _, err := tx.PushReceipt(lifetime, "producer", "key", "digest", testNow); !errors.Is(err, ErrNotFound) {
			t.Fatal("receipt survived rollback", err)
		}
		if _, err := tx.LastPush(lifetime, "health"); !errors.Is(err, ErrNotFound) {
			t.Fatal("latest receipt survived rollback", err)
		}
		if usage, err := tx.PushUsage(); err != nil || usage != (PushUsage{}) {
			t.Fatal("usage survived rollback", usage, err)
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPushContractSQLiteAndPostgres(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		s := openTest(t, t.TempDir())
		if err := s.Update(context.Background(), func(tx *Tx) error { pushContract(t, tx); return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		migration := postgresTestPool(t, "migration")
		if err := MigratePostgres(context.Background(), migration, "ding_app"); err != nil {
			t.Fatal(err)
		}
		db := postgresTestPool(t, "app")
		id := fmt.Sprintf("push-%d", time.Now().UnixNano())
		for _, suffix := range []string{"a", "b"} {
			tx := postgresScopedTx(t, db, id+suffix)
			if err := initializePostgresPush(tx); err != nil {
				t.Fatal(err)
			}
			pushContract(t, tx)
			if err := tx.sql.Commit(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
