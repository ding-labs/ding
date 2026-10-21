package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
)

func TestChangesOnlyAfterCommittedMutation(t *testing.T) {
	s := openTest(t, t.TempDir())
	changed, other := s.Changes(), s.Changes()
	ctx := context.Background()
	if _, err := s.Claim(ctx, testNow, time.Minute); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback")
	if err := s.Update(ctx, func(tx *Tx) error {
		if _, err := tx.sql.Exec("INSERT INTO metadata VALUES('probe','1')"); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Fatal("empty claim or rollback woke sleepers")
	default:
	}
	seed(t, s)
	for _, ch := range []<-chan struct{}{changed, other} {
		select {
		case <-ch:
		default:
			t.Fatal("commit failed to broadcast")
		}
	}
	select {
	case <-s.Changes():
		t.Fatal("new subscriber received stale wakeup")
	default:
	}
}

func TestDeliveryDeadlineRespectsOrderingBackoffAndLease(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, d := fixture(t)
	ctx := context.Background()
	assertNext := func(want time.Time) {
		t.Helper()
		got, err := s.NextDelivery(ctx)
		if err != nil || !got.Equal(want) {
			t.Fatalf("deadline %v, want %v: %v", got, want, err)
		}
	}
	assertNext(time.Time{})
	for _, id := range []string{"first", "second"} {
		if err := s.Update(ctx, func(tx *Tx) error { return transition(tx, c, d, id, testNow) }); err != nil {
			t.Fatal(err)
		}
	}
	assertNext(testNow)
	i, err := s.Claim(ctx, testNow, 30*time.Second)
	if err != nil || i == nil {
		t.Fatal(i, err)
	}
	assertNext(testNow.Add(30 * time.Second))
	if err := s.Finish(ctx, *i, delivery.Result{Outcome: delivery.Retryable}, testNow.Add(time.Minute), testNow); err != nil {
		t.Fatal(err)
	}
	assertNext(testNow.Add(time.Minute)) // successor is already due but blocked
	if i, err := s.Claim(ctx, testNow.Add(59*time.Second), time.Minute); err != nil || i != nil {
		t.Fatal("claimed before backoff", i, err)
	}
	i, err = s.Claim(ctx, testNow.Add(time.Minute), time.Minute)
	if err != nil || i == nil {
		t.Fatal(i, err)
	}
	if err := s.Finish(ctx, *i, delivery.Result{Outcome: delivery.Delivered}, testNow, testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertNext(testNow.Add(time.Minute))
}
