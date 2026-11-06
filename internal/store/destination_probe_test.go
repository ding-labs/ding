package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDestinationProbeSurvivesRestartWithoutResending(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	key, digest := "test-operation-0001", strings.Repeat("a", 64)
	reserve := func(wantStart bool, wantOutcome string) {
		t.Helper()
		if err := db.Update(ctx, func(tx *Tx) error {
			p, start, err := tx.BeginDestinationProbe(key, digest, time.Now())
			if err == nil && (start != wantStart || p.Outcome != wantOutcome) {
				t.Fatalf("unexpected receipt %+v start=%t", p, start)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	reserve(true, "pending")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	reserve(false, "pending")
	if err := db.Update(ctx, func(tx *Tx) error { return tx.FinishDestinationProbe(key, "accepted") }); err != nil {
		t.Fatal(err)
	}
	reserve(false, "accepted")
	if err := db.Update(ctx, func(tx *Tx) error {
		_, _, err := tx.BeginDestinationProbe(key, strings.Repeat("b", 64), time.Now())
		return err
	}); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("changed request reused key", err)
	}
	if err := db.Update(ctx, func(tx *Tx) error { return tx.FinishDestinationProbe(key, "failed") }); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("completed receipt changed", err)
	}
}
