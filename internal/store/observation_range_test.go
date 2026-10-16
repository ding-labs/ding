package store

import (
	"context"
	"fmt"
	"github.com/ding-labs/ding/internal/pgtest"
	"github.com/ding-labs/ding/internal/watch"
	"strings"
	"testing"
	"time"
)

func TestBoundedObservationRangeAndRetainedWatermark(t *testing.T) {
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
				for i := range 3 {
					if _, err := tx.AppendObservation("health", c.Revision, 1, watch.Observation{InputID: fmt.Sprint(i), Health: "ok", AcceptedAt: testNow}); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := s.View(ctx, func(tx *Tx) error {
				high, err := tx.ObservationWatermark()
				if err != nil {
					return err
				}
				if high != 3 {
					t.Error(high)
				}
				rows, err := tx.ObservationRange("health", 1, 3)
				if err != nil {
					return err
				}
				if len(rows) != 3 || rows[2].Sequence != 3 || rows[2].InputID != "2" {
					t.Error("range mismatch", rows)
				}
				for _, bounds := range [][2]int64{{0, 1}, {2, 1}, {1, 1001}} {
					if _, err = tx.ObservationRange("health", bounds[0], bounds[1]); err == nil {
						t.Error("invalid bounds accepted")
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				var work int64
				if err := s.Update(ctx, func(tx *Tx) error {
					var err error
					work, err = tx.RetainBatch(testNow.Add(time.Hour), testNow.Add(time.Hour), 64, "watermark")
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if work == 0 {
					break
				}
			}
			if err := s.View(ctx, func(tx *Tx) error {
				high, err := tx.ObservationWatermark()
				if high != 3 {
					t.Error("retention rewound allocation watermark", high)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
