package store

import (
	"context"
	"errors"
	"testing"
)

func TestEntityCountIncludesTransactionalChangesAndRollback(t *testing.T) {
	s := openTest(t, t.TempDir())
	seed(t, s)
	c, _ := fixture(t)
	abort := errors.New("rollback")
	err := s.Update(context.Background(), func(tx *Tx) error {
		for _, key := range []string{"one", "two"} {
			if err := tx.SaveEntity("health", key, c.Revision, map[string]bool{"open": false}, testNow); err != nil {
				return err
			}
		}
		if n, err := tx.EntityCount("health"); err != nil || n != 2 {
			t.Fatal(n, err)
		}
		if n, err := tx.EntityCount("absent"); err != nil || n != 0 {
			t.Fatal(n, err)
		}
		if err := tx.DeleteEntity("health", "one"); err != nil {
			return err
		}
		if n, err := tx.EntityCount("health"); err != nil || n != 1 {
			t.Fatal(n, err)
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if err := s.View(context.Background(), func(tx *Tx) error {
		if n, err := tx.EntityCount("health"); err != nil || n != 0 {
			t.Fatal(n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
