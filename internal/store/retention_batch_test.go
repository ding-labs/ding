package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/pgtest"
	"github.com/ding-labs/ding/internal/watch"
)

func TestRetentionBatchesRespectChangingLiveReferences(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var s *Store
			if backend == "sqlite" {
				s = openTest(t, t.TempDir())
			} else {
				f := pgtest.New(t)
				if err := MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
					t.Fatal(err)
				}
				var err error
				s, err = OpenPostgres(ctx, f.App, strings.Repeat("a", 64))
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
			}
			seed(t, s)
			c, _ := fixture(t)
			if err := s.Update(ctx, func(tx *Tx) error {
				for i := range 20 {
					if _, err := tx.AppendObservation("health", c.Revision, 1, watch.Observation{InputID: fmt.Sprint(i), AcceptedAt: testNow, Health: "ok"}); err != nil {
						return err
					}
				}
				return tx.SaveEntity("health", "[]", c.Revision, map[string]any{"lastSequence": int64(1)}, testNow)
			}); err != nil {
				t.Fatal(err)
			}
			cutoff := testNow.Add(time.Hour)
			if err := s.Update(ctx, func(tx *Tx) error {
				n, e := tx.RetainBatch(cutoff, cutoff, 2)
				if n != 2 {
					t.Error("batch did not bound deletion", n)
				}
				return e
			}); err != nil {
				t.Fatal(err)
			}
			// An intervening writer moves the pin from 1 to 4. The next batch must
			// preserve 4 and may now remove 1; it cannot reuse the old pin snapshot.
			if err := s.Update(ctx, func(tx *Tx) error {
				return tx.SaveEntity("health", "[]", c.Revision, map[string]any{"lastSequence": int64(4)}, testNow)
			}); err != nil {
				t.Fatal(err)
			}
			for range 20 {
				var n int64
				if err := s.Update(ctx, func(tx *Tx) error { var e error; n, e = tx.RetainBatch(cutoff, cutoff, 2); return e }); err != nil {
					t.Fatal(err)
				}
				if n == 0 {
					break
				}
			}
			if err := s.View(ctx, func(tx *Tx) error {
				u, e := tx.Usage()
				if e != nil {
					return e
				}
				if u.Observations != 1 {
					t.Error("incorrect retained rows", u.Observations)
				}
				_, e = tx.Observations("health", []int64{4})
				return e
			}); err != nil {
				t.Fatal("live reference removed", err)
			}
		})
	}
}
