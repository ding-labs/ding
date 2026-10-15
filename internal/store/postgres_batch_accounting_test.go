package store

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/watch"
)

func TestPostgresBatchAccountingReconcilesAndEnforcesQuota(t *testing.T) {
	s := openPostgresTestStore(t)
	seed(t, s)
	ctx := context.Background()
	c, _ := fixture(t)
	rng := rand.New(rand.NewPCG(123, 456))
	var before int64
	for round := range 12 {
		err := s.Update(ctx, func(tx *Tx) error {
			if _, err := tx.sql.Exec("SET LOCAL bytea_output='escape'"); err != nil {
				return err
			}
			batch := make([]watch.Observation, 1+rng.IntN(100))
			for i := range batch {
				batch[i] = watch.Observation{InputID: fmt.Sprintf("%d-%d", round, i), AcceptedAt: testNow, Fields: map[string]any{"value": strings.Repeat("é\\", rng.IntN(500))}}
			}
			if _, err := tx.AppendObservations("health", c.Revision, 1, batch); err != nil {
				return err
			}
			checkPostgresLogicalBytes(t, tx)
			// Retention and exceptional updates still use row-level accounting.
			if _, err := tx.sql.Exec("DELETE FROM observations WHERE sequence%3=0"); err != nil {
				return err
			}
			if _, err := tx.sql.Exec("UPDATE observations SET input_id=input_id||'x' WHERE sequence%7=0"); err != nil {
				return err
			}
			checkPostgresLogicalBytes(t, tx)
			usage, err := tx.Budget()
			before = usage.Bytes
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.LimitBytes(ctx, before+1); err != nil {
		t.Fatal(err)
	}
	err := s.Update(ctx, func(tx *Tx) error {
		_, err := tx.AppendObservations("health", c.Revision, 1, []watch.Observation{{InputID: "over-1"}, {InputID: "over-2"}})
		return err
	})
	if !errors.Is(err, ErrQuota) {
		t.Fatal("batch bypassed quota", err)
	}
	if err := s.View(ctx, func(tx *Tx) error {
		checkPostgresLogicalBytes(t, tx)
		usage, err := tx.Budget()
		if usage.Bytes != before {
			t.Fatal("rejected batch changed usage")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
