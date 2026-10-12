package watchrun

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

const receiptFixture = `apiVersion: ding.ing/v1alpha1
kind: Watch
metadata: {id: push}
spec:
  source: {type: push}
  condition: {field: value, operator: gt, value: 5}
`

func TestPushReceiptSurvivesResetRestartAndExpiresIndependently(t *testing.T) {
	a, dir := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: receiptFixture}); err != nil {
		t.Fatal(err)
	}
	r, err := a.Record(ctx, "push")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"value":9}`)
	input := NewPushInput("producer", "request", raw, 10, time.Hour)
	batch, err := PreparePush(ctx, r, raw, 32, 32000)
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.CommitPush(ctx, r, batch, input)
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(receiptFixture, "value: 5", "value: 10", 1)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	r, err = a.Record(ctx, "push")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a = New(db)
	a.Now = func() time.Time { return start.Add(30 * time.Minute) }
	receipt, err := a.ReplayPush(ctx, r, input)
	if err != nil || !receipt.Duplicate || receipt.AcceptanceID != first.AcceptanceID || receipt.Revision == r.Plan.Revision {
		t.Fatal("lost original receipt after reset/restart", receipt, err)
	}
	// Exact expiry permits a new acceptance even while its old observation exists.
	a.Now = func() time.Time { return start.Add(time.Hour) }
	if _, err := a.ReplayPush(ctx, r, input); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("expiry", err)
	}
	batch, err = PreparePush(ctx, r, raw, 32, 32000)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.CommitPush(ctx, r, batch, input)
	if err != nil || second.AcceptanceID == first.AcceptanceID || second.First <= first.Last {
		t.Fatal("expired receipt collision", second, err)
	}
	if err := db.View(ctx, func(tx *store.Tx) error {
		u, e := tx.PushUsage()
		if u.Requests != 2 || u.Observations != 2 {
			t.Error(u)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
}
func TestPushReceiptCapacityAndExpiredCredentialsDoNotMutate(t *testing.T) {
	a, _ := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: receiptFixture}); err != nil {
		t.Fatal(err)
	}
	r, _ := a.Record(ctx, "push")
	raw := []byte(`{"value":9}`)
	batch, _ := PreparePush(ctx, r, raw, 32, 32000)
	first := NewPushInput("producer", "1", raw, 1, time.Hour)
	if _, err := a.CommitPush(ctx, r, batch, first); err != nil {
		t.Fatal(err)
	}
	next := NewPushInput("producer", "2", raw, 1, time.Hour)
	if _, err := a.CommitPush(ctx, r, batch, next); !errors.Is(err, ErrQuota) {
		t.Fatal("receipt bound", err)
	}
	if _, err := a.ReplayPush(ctx, r, first); err != nil {
		t.Fatal("capacity blocked reconciliation", err)
	}
	next.MaxReceipts = 10
	next.ValidUntil = start
	if _, err := a.CommitPush(ctx, r, batch, next); !errors.Is(err, ErrProducerExpired) {
		t.Fatal("expiry bypass", err)
	}
}

func TestThousandPushInputsReconcileAcrossRestarts(t *testing.T) {
	a, dir := setup(t)
	if _, err := a.Apply(ctx, ApplyRequest{Manifest: receiptFixture}); err != nil {
		t.Fatal(err)
	}
	firstSequences := map[int64]bool{}
	for i := 0; i < 1000; i++ {
		now := start.Add(time.Duration(i) * time.Second)
		a.Now = func() time.Time { return now }
		r, err := a.Record(ctx, "push")
		if err != nil {
			t.Fatal(err)
		}
		raw := []byte(fmt.Sprintf(`{"value":%d}`, i%10))
		input := NewPushInput("stable-producer", fmt.Sprint(i), raw, 2000, 24*time.Hour)
		batch, err := PreparePush(ctx, r, raw, 32, 32000)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := a.CommitPush(ctx, r, batch, input)
		if err != nil {
			t.Fatalf("input %d: %v", i, err)
		}
		if firstSequences[receipt.First] {
			t.Fatal("reused observation sequence")
		}
		firstSequences[receipt.First] = true
		if i%10 == 0 { // Simulate the producer losing its first acknowledgment.
			retry, err := a.CommitPush(ctx, r, batch, input)
			if err != nil || !retry.Duplicate || retry.AcceptanceID != receipt.AcceptanceID {
				t.Fatal("retry", retry, err)
			}
		}
		if i%250 == 249 {
			if err := a.Store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := store.Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			a = New(db)
			a.Now = func() time.Time { return now }
			replay, err := a.ReplayPush(ctx, r, input)
			if err != nil || replay.AcceptanceID != receipt.AcceptanceID {
				t.Fatal("restart lost receipt", err)
			}
		}
	}
	if err := a.Store.View(ctx, func(tx *store.Tx) error {
		usage, err := tx.PushUsage()
		if usage.Requests != 1000 || usage.Observations != 1000 {
			t.Error("ledger does not reconcile", usage)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
