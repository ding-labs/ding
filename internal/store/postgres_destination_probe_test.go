package store

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresDestinationProbeIsolationConcurrencyAndAccounting(t *testing.T) {
	ctx := context.Background()
	a := openPostgresTestStore(t)
	b := openPostgresTestStore(t)
	peer, err := OpenPostgres(ctx, postgresTestPool(t, "app"), a.workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	const key = "postgres-probe-operation-0001"
	digest := strings.Repeat("a", 64)
	var starts atomic.Int64
	var wg sync.WaitGroup
	for _, s := range []*Store{a, peer} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Update(ctx, func(tx *Tx) error {
				_, start, err := tx.BeginDestinationProbe(key, digest, time.Now())
				if start {
					starts.Add(1)
				}
				return err
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if starts.Load() != 1 {
		t.Fatalf("concurrent callers reserved %d sends", starts.Load())
	}
	if err := a.Update(ctx, func(tx *Tx) error {
		if err := tx.FinishDestinationProbe(key, "accepted"); err != nil {
			return err
		}
		checkPostgresLogicalBytes(t, tx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// An identical operation in another tenant must not see or change A's receipt.
	if err := b.Update(ctx, func(tx *Tx) error {
		p, start, err := tx.BeginDestinationProbe(key, digest, time.Now())
		if err == nil && (!start || p.Outcome != "pending") {
			t.Fatal("cross-workspace receipt reused", p, start)
		}
		checkPostgresLogicalBytes(t, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := peer.Update(ctx, func(tx *Tx) error {
		p, start, err := tx.BeginDestinationProbe(key, digest, time.Now())
		if err == nil && (start || p.Outcome != "accepted") {
			t.Fatal("completed receipt changed", p, start)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
