package store

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"
)

func TestPushReceiptCountMatchesModelAcrossExpiryReuseAndRollback(t *testing.T) {
	s := openTest(t, t.TempDir())
	ctx := context.Background()
	model := map[int]int64{}
	random := rand.New(rand.NewSource(7))
	insert := "INSERT INTO push_receipts VALUES('life','producer',?,'digest','{}',?) ON CONFLICT(lifetime,producer,input_key) DO UPDATE SET expires_at=excluded.expires_at"
	check := func(tx *Tx, at int64, want map[int]int64) error {
		n := 0
		for _, expiry := range want {
			if expiry > at {
				n++
			}
		}
		got, err := tx.PushReceiptCount(time.Unix(0, at))
		if err != nil || got != n {
			t.Fatalf("at=%d count=%d want=%d err=%v", at, got, n, err)
		}
		return nil
	}
	abort := errors.New("rollback")
	for step := range 300 {
		next := map[int]int64{}
		for k, v := range model {
			next[k] = v
		}
		key, at := random.Intn(20), int64(random.Intn(100)-30)
		err := s.Update(ctx, func(tx *Tx) error {
			if step%3 == 0 {
				if _, err := tx.sql.Exec("DELETE FROM push_receipts WHERE input_key=?", key); err != nil {
					return err
				}
				delete(next, key)
			} else {
				expiry := int64(random.Intn(120) - 20)
				if _, err := tx.sql.Exec(insert, key, expiry); err != nil {
					return err
				}
				next[key] = expiry
			}
			check(tx, at, next)
			if step%17 == 0 {
				return abort
			}
			return nil
		})
		if step%17 == 0 {
			if !errors.Is(err, abort) {
				t.Fatal(err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			model = next
		}
		if err := s.View(ctx, func(tx *Tx) error { return check(tx, at+10, model) }); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPushReceiptCountRepairsMissingMetadataWithoutWritingFromView(t *testing.T) {
	s := openTest(t, t.TempDir())
	ctx := context.Background()
	if _, err := s.db.Exec("INSERT INTO push_receipts VALUES('life','producer','key','digest','{}',10); DELETE FROM push_receipt_stats"); err != nil {
		t.Fatal(err)
	}
	check := func(at int64, want int) func(*Tx) error {
		return func(tx *Tx) error {
			n, err := tx.PushReceiptCount(time.Unix(0, at))
			if err != nil || n != want {
				t.Fatal(n, want, err)
			}
			return nil
		}
	}
	if err := s.View(ctx, check(0, 1)); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "push_receipt_stats") != 0 {
		t.Fatal("read repaired metadata")
	}
	if err := s.Update(ctx, check(0, 1)); err != nil {
		t.Fatal(err)
	}
	if err := s.View(ctx, check(20, 0)); err != nil {
		t.Fatal(err)
	}
	var watermark int64
	if err := s.db.QueryRow("SELECT checked_at FROM push_receipt_stats").Scan(&watermark); err != nil || watermark != 0 {
		t.Fatal("read advanced watermark", watermark, err)
	}
	if err := s.Update(ctx, check(20, 0)); err != nil {
		t.Fatal(err)
	}
	assertPushStats(t, s, 1, 1, 10)
}
