package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/watch"
)

func TestQuotaBytesMatchesBudgetAcrossCommitAndRollback(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var s *Store
			if backend == "postgres" {
				s = openPostgresTestStore(t)
			} else {
				s = openTest(t, t.TempDir())
			}
			seed(t, s)
			ctx := context.Background()
			compiled, _ := fixture(t)
			check := func(tx *Tx) int64 {
				t.Helper()
				budget, err := tx.Budget()
				if err != nil {
					t.Fatal(err)
				}
				bytes, err := tx.QuotaBytes()
				if err != nil {
					t.Fatal(err)
				}
				if bytes != budget.Bytes {
					t.Fatalf("quota bytes %d != budget %d", bytes, budget.Bytes)
				}
				return bytes
			}
			var committed int64
			err := s.Update(ctx, func(tx *Tx) error {
				before := check(tx)
				_, err := tx.AppendObservation("health", compiled.Revision, 1, watch.Observation{InputID: "committed", Fields: map[string]any{"data": strings.Repeat("x", 16384)}})
				if err != nil {
					return err
				}
				committed = check(tx)
				if committed <= before {
					t.Fatal("new retained data did not increase quota bytes")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			rollback := errors.New("rollback")
			err = s.Update(ctx, func(tx *Tx) error {
				_, err := tx.AppendObservation("health", compiled.Revision, 1, watch.Observation{InputID: "rolled-back", Fields: map[string]any{"data": strings.Repeat("y", 16384)}})
				if err != nil {
					return err
				}
				if check(tx) <= committed {
					t.Fatal("transaction did not see new bytes")
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			if err := s.View(ctx, func(tx *Tx) error {
				if check(tx) != committed {
					t.Fatal("rollback changed quota bytes")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
