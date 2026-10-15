package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPostgresLogicalQuotaIsAtomic(t *testing.T) {
	s := openPostgresTestStore(t)
	ctx := context.Background()
	if err := s.Update(ctx, func(tx *Tx) error { stateContract(t, tx); return nil }); err != nil {
		t.Fatal(err)
	}
	var before Usage
	if err := s.View(ctx, func(tx *Tx) error { var err error; before, err = tx.Usage(); return err }); err != nil {
		t.Fatal(err)
	}
	if before.Bytes <= 0 || before.Entities != 1 || before.Observations != 2 || before.Timers != 1 {
		t.Fatal("usage mismatch", before)
	}
	if err := s.LimitBytes(ctx, before.Bytes-1); !errors.Is(err, ErrQuota) {
		t.Fatal("limit below retained state accepted", err)
	}
	if err := s.LimitBytes(ctx, before.Bytes+1); err != nil {
		t.Fatal(err)
	}
	c, _ := fixture(t)
	err := s.Update(ctx, func(tx *Tx) error {
		return tx.SaveEntity("health", "group", c.Revision, map[string]string{"large": strings.Repeat("x", 4096)}, testNow)
	})
	if !errors.Is(err, ErrQuota) {
		t.Fatal("over-limit transaction accepted", err)
	}
	if err = s.View(ctx, func(tx *Tx) error {
		got, err := tx.Budget()
		if err != nil {
			return err
		}
		if got.Bytes != before.Bytes {
			t.Fatal("failed transaction changed quota accounting")
		}
		raw, err := tx.Entity("health", "group")
		if err != nil {
			return err
		}
		if string(raw) != `{"matches":2}` {
			t.Fatal("failed transaction changed entity")
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	h, err := s.Health(ctx)
	if err != nil || h.Backend != "postgres" || h.Postgres == "" || h.Integrity != "ok" || h.SQLite != "" || h.LogicalBytes != before.Bytes {
		t.Fatal("backend readiness mismatch", h, err)
	}
}
