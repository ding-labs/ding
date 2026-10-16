package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/database"
	"github.com/ding-labs/ding/internal/pgtest"
)

func TestPostgresEntityAccountingUpgradeAndQuota(t *testing.T) {
	ctx := context.Background()
	f := pgtest.New(t)
	if err := database.Migrate(ctx, f.Migration, "runtime", postgresMigrations[:15], "ding_app"); err != nil {
		t.Fatal(err)
	}
	old := postgresScopedTx(t, f.App, strings.Repeat("c", 64))
	// Seed the durable workspace metadata written by the previous binary.
	for _, q := range []string{"INSERT INTO push_usage(singleton) VALUES(1)", "INSERT INTO metadata(key,value) VALUES('store_id','upgrade')", "INSERT INTO push_receipt_stats(singleton,total,expired,checked_at,earliest) VALUES(1,0,0,0,0)"} {
		if _, err := old.sql.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	c, _ := fixture(t)
	if err := old.SaveWatch(WatchRecord{Plan: c, Generation: 1, Status: "running"}, testNow); err != nil {
		t.Fatal(err)
	}
	states := map[string]json.RawMessage{
		"a": json.RawMessage(`{"lastSequence":1,"incidentEvent":"é\\event"}`),
		"b": json.RawMessage(`{"lastSequence":2}`),
	}
	if err := old.SaveEntities("health", c.Revision, states, testNow); err != nil {
		t.Fatal(err)
	}
	u, err := old.Budget()
	if err != nil {
		t.Fatal(err)
	}
	before := u.Bytes
	if err := old.sql.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := MigratePostgres(ctx, f.Migration, "ding_app"); err != nil {
		t.Fatal(err)
	}
	s, err := OpenPostgres(ctx, f.App, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.View(ctx, func(tx *Tx) error {
		checkPostgresLogicalBytes(t, tx)
		u, err := tx.Budget()
		if u.Bytes != before {
			t.Fatal("upgrade changed retained bytes", u.Bytes, before)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.LimitBytes(ctx, before+1); err != nil {
		t.Fatal(err)
	}
	states["new"] = json.RawMessage(`{"lastSequence":3,"healthEvent":"new-health"}`)
	err = s.Update(ctx, func(tx *Tx) error { return tx.SaveEntities("health", c.Revision, states, testNow) })
	if !errors.Is(err, ErrQuota) {
		t.Fatal("entity batch bypassed quota", err)
	}
	if err := s.Update(ctx, func(tx *Tx) error {
		checkPostgresLogicalBytes(t, tx)
		if _, err := tx.Entity("health", "new"); !errors.Is(err, ErrNotFound) {
			t.Fatal("rejected batch remained", err)
		}
		if _, err := tx.sql.Exec("SET LOCAL bytea_output='escape'"); err != nil {
			return err
		}
		// UPDATE changes row size; entity deletion cascades to its pins.
		if err := tx.SaveEntities("health", c.Revision, map[string]json.RawMessage{"a": json.RawMessage(`{}`)}, testNow); err != nil {
			return err
		}
		if err := tx.DeleteEntity("health", "b"); err != nil {
			return err
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
