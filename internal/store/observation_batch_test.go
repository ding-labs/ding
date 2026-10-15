package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ding-labs/ding/internal/watch"
)

func TestObservationBatchRangeRollsBackWithRows(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var s *Store
			if backend == "postgres" {
				s = openPostgresTestStore(t)
			} else {
				s = openTest(t, t.TempDir())
			}
			seed(t, s)
			c, _ := fixture(t)
			batch := make([]watch.Observation, 101)
			for i := range batch {
				batch[i] = watch.Observation{InputID: fmt.Sprint(i), AcceptedAt: testNow, Health: "ok", Fields: map[string]any{"value": i}}
			}
			rollback := errors.New("rollback")
			ctx := context.Background()
			for _, abort := range []bool{true, false} {
				err := s.Update(ctx, func(tx *Tx) error {
					seq, err := tx.AppendObservations("health", c.Revision, 1, batch)
					if err != nil {
						return err
					}
					for i, n := range seq {
						if n != int64(i+1) {
							t.Fatal("range has a gap", seq)
						}
					}
					if abort {
						return rollback
					}
					return nil
				})
				if (abort && !errors.Is(err, rollback)) || (!abort && err != nil) {
					t.Fatal(err)
				}
			}
			if err := s.View(ctx, func(tx *Tx) error {
				rows, err := tx.ObservationRange("health", 1, 101)
				if err != nil {
					return err
				}
				if len(rows) != len(batch) {
					t.Fatal("lost batch rows")
				}
				for i, row := range rows {
					if row.Sequence != int64(i+1) || row.InputID != batch[i].InputID || !row.AcceptedAt.Equal(testNow) {
						t.Fatal("range no longer matches input order", i)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			// A unique-input conflict must roll back both the rows and allocator.
			if err := s.Update(ctx, func(tx *Tx) error {
				_, err := tx.AppendObservations("health", c.Revision, 1, batch)
				return err
			}); err == nil {
				t.Fatal("duplicate inputs accepted")
			}
			if err := s.Update(ctx, func(tx *Tx) error {
				n, err := tx.AppendObservation("health", c.Revision, 1, watch.Observation{InputID: "after", AcceptedAt: testNow})
				if err == nil && n != 102 {
					t.Fatal("failed batch consumed sequence", n)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
