package store

import (
	"context"
	"errors"
	"github.com/ding-labs/ding/internal/delivery"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/plan"
)

func TestPurgeWatchRemovesHistoryAtomically(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := transition(tx, c, d, "purge-input", testNow); err != nil {
			return err
		}
		if err := tx.SaveReplay("event-purge-input", map[string]any{"test": true}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error { return tx.PurgeWatch("health") }); err == nil {
		t.Fatal("purged running watch")
	}
	leased, err := s.Claim(ctx, testNow, time.Minute)
	if err != nil || leased == nil {
		t.Fatal(leased, err)
	}
	if err := s.Finish(ctx, *leased, delivery.Result{Outcome: delivery.Retryable}, testNow, testNow); err != nil {
		t.Fatal(err)
	}
	leased, err = s.Claim(ctx, testNow, time.Minute)
	if err != nil || leased == nil {
		t.Fatal(leased, err)
	}
	if count(t, s, "delivery_attempts") != 1 {
		t.Fatal("missing attempt fixture")
	}

	if err := s.Update(ctx, func(tx *Tx) error {
		r, err := tx.Watch("health")
		if err != nil {
			return err
		}
		r.Status = "deleted"
		if err := tx.SaveWatch(r, testNow); err != nil {
			return err
		}
		return tx.SaveHandoff(Handoff{ID: strings.Repeat("a", 64), WatchID: "health", Held: true})
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error { return tx.PurgeWatch("health") }); err == nil {
		t.Fatal("purged held watch")
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := tx.SaveHandoff(Handoff{ID: strings.Repeat("a", 64), WatchID: "health"}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	aborted := errors.New("abort purge")
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := tx.PurgeWatch("health"); err != nil {
			return err
		}
		return aborted
	}); !errors.Is(err, aborted) {
		t.Fatal(err)
	}
	if count(t, s, "events") != 1 || count(t, s, "outbox") != 1 {
		t.Fatal("rollback lost history")
	}
	if err := s.Update(ctx, func(tx *Tx) error { return tx.PurgeWatch("health") }); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"watches", "watch_revisions", "observations", "events", "event_evidence", "event_replays", "entities", "timers", "input_receipts", "outbox", "delivery_attempts", "handoffs"} {
		if n := count(t, s, table); n != 0 {
			t.Fatalf("%s retained %d rows", table, n)
		}
	}
	if err := s.Finish(ctx, *leased, delivery.Result{Outcome: delivery.Delivered}, testNow, testNow); !errors.Is(err, ErrStale) {
		t.Fatal("purged lease accepted completion", err)
	}
	if count(t, s, "destinations") != 1 {
		t.Fatal("shared destination removed")
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		return tx.SaveWatch(WatchRecord{Plan: c, Status: "running", Generation: 1}, testNow)
	}); err == nil {
		t.Fatal("reused purged ID")
	}
	if err := s.View(ctx, func(tx *Tx) error { _, err := tx.Watch("health"); return err }); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestPurgeWatchPreservesOtherWatch(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	c.Definition.Metadata.ID = "other"
	other, err := plan.Compile(c.Definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := tx.SaveWatch(WatchRecord{Plan: other, Status: "running", Generation: 1}, testNow); err != nil {
			return err
		}
		if err := transition(tx, other, d, "other-input", testNow); err != nil {
			return err
		}
		r, err := tx.Watch("health")
		if err != nil {
			return err
		}
		r.Status = "deleted"
		if err := tx.SaveWatch(r, testNow); err != nil {
			return err
		}
		return tx.PurgeWatch("health")
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"watches", "watch_revisions", "observations", "events", "event_evidence", "entities", "timers", "input_receipts", "outbox"} {
		if n := count(t, s, table); n != 1 {
			t.Fatalf("%s: expected other watch's row, got %d", table, n)
		}
	}
}
