package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/pgtest"
)

func TestEntityReferencesCommitRollbackAndDelete(t *testing.T) {
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
			save := func(tx *Tx, seq int64) error {
				return tx.SaveEntity("health", "[]", c.Revision, map[string]any{"lastSequence": seq, "freshSequence": seq, "evidence": []int64{1, 2}, "samples": []map[string]any{{"sequence": 3}}, "incidentEvent": "firing", "healthEvent": "health"}, testNow)
			}
			if err := s.Update(ctx, func(tx *Tx) error { return save(tx, 4) }); err != nil {
				t.Fatal(err)
			}
			assert := func(want map[entityPin]bool) {
				t.Helper()
				if err := s.View(ctx, func(tx *Tx) error {
					rows, err := tx.sql.QueryContext(ctx, tx.query(qEntityPins), "health", "[]")
					if err != nil {
						return err
					}
					defer rows.Close()
					got := map[entityPin]bool{}
					for rows.Next() {
						var p entityPin
						if err = rows.Scan(&p.kind, &p.value); err != nil {
							return err
						}
						got[p] = true
					}
					if !reflect.DeepEqual(got, want) {
						t.Error("references differ", got, want)
					}
					return rows.Err()
				}); err != nil {
					t.Fatal(err)
				}
			}
			want := map[entityPin]bool{{"input", "1"}: true, {"input", "2"}: true, {"input", "3"}: true, {"input", "4"}: true, {"event", "firing"}: true, {"event", "health"}: true}
			assert(want)
			abort := errors.New("abort")
			if err := s.Update(ctx, func(tx *Tx) error {
				if err := save(tx, 5); err != nil {
					return err
				}
				return abort
			}); !errors.Is(err, abort) {
				t.Fatal(err)
			}
			assert(want)
			if err := s.Update(ctx, func(tx *Tx) error { return save(tx, 5) }); err != nil {
				t.Fatal(err)
			}
			delete(want, entityPin{"input", "4"})
			want[entityPin{"input", "5"}] = true
			assert(want)
			if backend == "postgres" {
				if err := s.View(ctx, func(tx *Tx) error { checkPostgresLogicalBytes(t, tx); return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Update(ctx, func(tx *Tx) error { return tx.DeleteEntity("health", "[]") }); err != nil {
				t.Fatal(err)
			}
			assert(map[entityPin]bool{})
		})
	}
}
