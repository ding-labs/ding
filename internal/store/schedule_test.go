package store

import (
	"context"
	"testing"
	"time"
)

func TestScheduleIgnoresPausedAndStaleTimers(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var s *Store
			if backend == "postgres" {
				s = openPostgresTestStore(t)
			} else {
				s = openTest(t, t.TempDir())
			}
			ctx := context.Background()
			c, _ := fixture(t)
			r := WatchRecord{Plan: c, Generation: 2, Status: "running", NextAt: testNow}
			if err := s.Update(ctx, func(tx *Tx) error {
				if err := tx.SaveWatch(r, testNow); err != nil {
					return err
				}
				if err := tx.SaveTimer(Timer{WatchID: "health", Entity: "old", Kind: "missing", Generation: 1, Due: testNow}); err != nil {
					return err
				}
				return tx.SaveTimer(Timer{WatchID: "health", Entity: "current", Kind: "missing", Generation: 2, Due: testNow.Add(time.Hour)})
			}); err != nil {
				t.Fatal(err)
			}
			got, err := s.Schedule(ctx)
			if err != nil || !got.TimerAt.Equal(testNow.Add(time.Hour)) || len(got.Watches) != 1 {
				t.Fatal("stale timer scheduled", got.TimerAt, err)
			}
			r.Status = "paused"
			if err := s.Update(ctx, func(tx *Tx) error { return tx.SaveWatch(r, testNow) }); err != nil {
				t.Fatal(err)
			}
			got, err = s.Schedule(ctx)
			if err != nil || !got.TimerAt.IsZero() {
				t.Fatal("paused timer scheduled", got.TimerAt, err)
			}
		})
	}
}
