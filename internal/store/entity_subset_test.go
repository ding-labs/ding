package store

import (
	"context"
	"testing"
)

func TestEntitySubsetIncludesOnlyRequestedExistingKeys(t *testing.T) {
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
			if err := s.Update(ctx, func(tx *Tx) error {
				for _, key := range []string{"wanted", "unwanted"} {
					if err := tx.SaveEntity("health", key, c.Revision, map[string]string{"key": key}, testNow); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := s.View(ctx, func(tx *Tx) error {
				states, err := tx.EntitySubset("health", []string{"wanted", "missing", "wanted"})
				if err == nil && (len(states) != 1 || string(states["wanted"]) != `{"key":"wanted"}`) {
					t.Fatal("subset lost identity or included an unrelated group", states)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
