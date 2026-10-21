package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/delivery"
	"github.com/ding-labs/ding/internal/plan"
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
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var s *Store
			if backend == "postgres" {
				s = openPostgresTestStore(t)
			} else {
				s = openTest(t, t.TempDir())
			}
			testDeliveryDeadline(t, s)
		})
	}
}
func testDeliveryDeadline(t *testing.T, s *Store) {
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

	// Current main leases per watch/destination stream. A different watch sharing
	// the destination must remain immediately runnable, despite this live lease.
	definition := c.Definition
	definition.Metadata.ID = "other"
	other, err := plan.Compile(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := tx.SaveWatch(WatchRecord{Plan: other, Generation: 1, Status: "running", NextAt: testNow}, testNow); err != nil {
			return err
		}
		return transition(tx, other, d, "other-stream", testNow)
	}); err != nil {
		t.Fatal(err)
	}
	assertNext(testNow)
	concurrent, err := s.Claim(ctx, testNow, 30*time.Second)
	if err != nil || concurrent == nil || concurrent.WatchID != "other" {
		t.Fatal(concurrent, err)
	}
	if err := s.Finish(ctx, *concurrent, delivery.Result{Outcome: delivery.Delivered}, testNow, testNow); err != nil {
		t.Fatal(err)
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

func TestCommitBroadcastReachesEverySleeper(t *testing.T) {
	s := openTest(t, t.TempDir())
	ctx := context.Background()
	for generation := 0; generation < 20; generation++ {
		changed := s.Changes()
		received := make(chan struct{}, 32)
		for range 32 {
			go func() { <-changed; received <- struct{}{} }()
		}
		// The commit may win before any waiter enters select; the closed broadcast
		// must still wake every waiter and the next generation must remain asleep.
		if err := s.Update(ctx, func(tx *Tx) error {
			_, err := tx.exec(statement{sqlite: "INSERT INTO metadata(key,value) VALUES('broadcast','1') ON CONFLICT(key) DO UPDATE SET value=metadata.value+1"})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for range 32 {
			select {
			case <-received:
			case <-time.After(time.Second):
				t.Fatal("lost commit wakeup")
			}
		}
		select {
		case <-s.Changes():
			t.Fatal("new generation received stale signal")
		default:
		}
	}
}
