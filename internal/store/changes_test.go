package store

import (
	"context"
	"errors"
	"testing"
)

func TestSchedulingHintsFollowCommitSQLiteAndPostgres(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var s *Store
			if backend == "postgres" {
				s = openPostgresTestStore(t)
			} else {
				s = openTest(t, t.TempDir())
			}
			ctx := context.Background()
			before := s.ChangeVersion()
			rollback := errors.New("rollback")
			mutate := func(tx *Tx) error {
				_, err := tx.exec(statement{sqlite: "INSERT INTO metadata(key,value) VALUES('wake-test','1')", postgres: "INSERT INTO metadata(key,value) VALUES('wake-test','1')"})
				return err
			}
			if err := s.Update(ctx, func(tx *Tx) error {
				if err := mutate(tx); err != nil {
					return err
				}
				return rollback
			}); !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			if s.ChangeVersion() != before {
				t.Fatal("rolled-back callback published a change")
			}
			if err := s.Update(ctx, func(*Tx) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if s.ChangeVersion() != before {
				t.Fatal("empty transaction published a change")
			}
			if err := s.Update(ctx, mutate); err != nil {
				t.Fatal(err)
			}
			if s.ChangeVersion() != before+1 {
				t.Fatal("committed mutation did not advance version")
			}
			select {
			case <-s.Changes():
			default:
				t.Fatal("missing commit wakeup")
			}
			if err := s.View(ctx, func(tx *Tx) error {
				var value string
				return tx.sql.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='wake-test'").Scan(&value)
			}); err != nil {
				t.Fatal("wake preceded durable state", err)
			}
		})
	}
}
