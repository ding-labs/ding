package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/ding-labs/ding/internal/plan"
)

func TestWatchListMatchesOrderedIndividualReads(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var s *Store
			if backend == "postgres" {
				s = openPostgresTestStore(t)
			} else {
				s = openTest(t, t.TempDir())
			}
			ctx := context.Background()
			definition, _ := fixture(t)
			if err := s.Update(ctx, func(tx *Tx) error {
				for _, id := range []string{"z", "a", "health"} {
					d := definition.Definition
					d.Metadata.ID = id
					compiled, err := plan.Compile(d)
					if err != nil {
						return err
					}
					if err = tx.SaveWatch(WatchRecord{Plan: compiled, Generation: 2, Status: "paused", Cursor: id, LastError: "sample", LastInputAt: testNow, NextAt: testNow}, testNow); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := s.View(ctx, func(tx *Tx) error {
				all, err := tx.Watches()
				if err != nil {
					return err
				}
				if len(all) != 3 {
					t.Fatal("wrong watch count", len(all))
				}
				for i, id := range []string{"a", "health", "z"} {
					one, err := tx.Watch(id)
					if err != nil {
						return err
					}
					if !reflect.DeepEqual(one, all[i]) {
						t.Fatal("list lost fields or order", id)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
