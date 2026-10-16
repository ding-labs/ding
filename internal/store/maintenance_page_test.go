package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestMaintenanceCursorIsDurableAndRollbackSafe(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	seed(t, s)
	c, _ := fixture(t)
	if err := s.Update(ctx, func(tx *Tx) error {
		for i := range 5 {
			if err := tx.SaveEntity("health", fmt.Sprint(i), c.Revision, map[string]any{}, testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	page := func(tx *Tx, want string) error {
		rows, done, err := tx.NextMaintenanceEntities(2)
		if err != nil {
			return err
		}
		if done || len(rows) != 2 || rows[0].Key != want {
			t.Error("wrong cursor page", rows, done, want)
		}
		return nil
	}
	if err := s.Update(ctx, func(tx *Tx) error { return page(tx, "0") }); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("abort")
	if err := s.Update(ctx, func(tx *Tx) error {
		if err := page(tx, "2"); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	s.Close()
	s = openTest(t, dir)
	if err := s.Update(ctx, func(tx *Tx) error { return page(tx, "2") }); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		rows, done, e := tx.NextMaintenanceEntities(2)
		if done || len(rows) != 1 || rows[0].Key != "4" {
			t.Error(rows, done)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		rows, done, e := tx.NextMaintenanceEntities(2)
		if !done || len(rows) != 0 {
			t.Error(rows, done)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, func(tx *Tx) error { return page(tx, "0") }); err != nil {
		t.Fatal(err)
	}
}
