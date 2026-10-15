package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestEntityBatchCommitRollbackAndPinRemoval(t *testing.T) {
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
			ctx := context.Background()
			var other *Store
			if backend == "postgres" {
				var err error
				other, err = OpenPostgres(ctx, s.db, strings.Repeat("b", 64))
				if err != nil {
					t.Fatal(err)
				}
				defer other.Close()
				seed(t, other)
				if err := other.Update(ctx, func(tx *Tx) error {
					return tx.SaveEntities("health", c.Revision, map[string]json.RawMessage{"0": json.RawMessage(`{"lastSequence":777}`)}, testNow)
				}); err != nil {
					t.Fatal(err)
				}
			}
			states := map[string]json.RawMessage{}
			for i := range 205 { // Cross two PostgreSQL chunk boundaries.
				states[fmt.Sprint(i)] = json.RawMessage(fmt.Sprintf(`{"lastSequence":%d,"freshSequence":%d}`, i+1, i+1))
			}
			if err := s.Update(ctx, func(tx *Tx) error {
				return tx.SaveEntities("health", c.Revision, states, testNow)
			}); err != nil {
				t.Fatal(err)
			}
			assert := func(want map[string]json.RawMessage, pins int) {
				t.Helper()
				if err := s.View(ctx, func(tx *Tx) error {
					got, err := tx.Entities("health")
					if err != nil {
						return err
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatal("entity batch differs")
					}
					var count int
					if err := tx.sql.QueryRow("SELECT count(*) FROM entity_pins").Scan(&count); err != nil {
						return err
					}
					if count != pins {
						t.Fatalf("pin count %d, want %d", count, pins)
					}
					if tx.postgres {
						checkPostgresLogicalBytes(t, tx)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			assert(states, 205)
			changed := map[string]json.RawMessage{"0": json.RawMessage(`{}`), "204": json.RawMessage(`{"lastSequence":999,"incidentEvent":"event"}`)}
			abort := errors.New("rollback batch")
			if err := s.Update(ctx, func(tx *Tx) error {
				if err := tx.SaveEntities("health", c.Revision, changed, testNow); err != nil {
					return err
				}
				return abort
			}); !errors.Is(err, abort) {
				t.Fatal(err)
			}
			assert(states, 205)
			if err := s.Update(ctx, func(tx *Tx) error {
				return tx.SaveEntities("health", c.Revision, changed, testNow)
			}); err != nil {
				t.Fatal(err)
			}
			for key, value := range changed {
				states[key] = value
			}
			assert(states, 205)
			if err := s.View(ctx, func(tx *Tx) error {
				rows, err := tx.sql.QueryContext(ctx, tx.query(qEntityPins), "health", "204")
				if err != nil {
					return err
				}
				defer rows.Close()
				got := map[entityPin]bool{}
				for rows.Next() {
					var p entityPin
					if err := rows.Scan(&p.kind, &p.value); err != nil {
						return err
					}
					got[p] = true
				}
				if !reflect.DeepEqual(got, map[entityPin]bool{{"input", "999"}: true, {"event", "event"}: true}) {
					t.Fatal("replacement pins differ", got)
				}
				return rows.Err()
			}); err != nil {
				t.Fatal(err)
			}
			if other != nil {
				if err := other.View(ctx, func(tx *Tx) error {
					data, err := tx.Entity("health", "0")
					if err == nil && string(data) != `{"lastSequence":777}` {
						t.Fatal("cross-workspace entity mutation")
					}
					checkPostgresLogicalBytes(t, tx)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
