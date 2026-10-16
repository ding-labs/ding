package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ding-labs/ding/internal/database"
)

func TestAcceptanceGuardFinalCheckRollsBackChanges(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var s *Store
			if backend == "postgres" {
				s = openPostgresTestStore(t)
			} else {
				s = openTest(t, t.TempDir())
			}
			seed(t, s)
			sentinel := errors.New("credential expired before commit")
			locked := false
			ctx := database.WithTransactionGuard(context.Background(), database.TransactionGuard{Pool: s.db, Lock: func(context.Context, *sql.Tx) (func() error, error) {
				locked = true
				return func() error { return sentinel }, nil
			}})
			err := s.Update(ctx, func(tx *Tx) error { return tx.Checkpoint("health", 1, testNow, testNow, "must-rollback") })
			if !locked || !errors.Is(err, sentinel) {
				t.Fatal("final authorization ignored", err)
			}
			if err = s.View(context.Background(), func(tx *Tx) error {
				r, err := tx.Watch("health")
				if err == nil && r.Cursor == "must-rollback" {
					t.Error("failed guard committed state")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
