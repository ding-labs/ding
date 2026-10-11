package state

import (
	"context"
	"testing"
)

func TestProbeReplayPreservesUnknownOutcomeAndTenantBinding(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "a", 2)
	b, _ := db.Enroll(ctx, "issuer", "b", 2)
	id, digest := ID(), TokenHash("test")
	_, start, err := db.BeginProbe(ctx, a.ID, id, digest)
	if err != nil || !start {
		t.Fatal(err)
	}
	p, start, err := db.BeginProbe(ctx, a.ID, id, digest)
	if err != nil || start || p.Outcome != "pending" {
		t.Fatal("pending test was repeated")
	}
	if _, _, err := db.BeginProbe(ctx, a.ID, id, TokenHash("changed")); err == nil {
		t.Fatal("key reused for a changed test")
	}
	if err := db.FinishProbe(ctx, a.ID, id, "accepted"); err != nil {
		t.Fatal(err)
	}
	p, start, err = db.BeginProbe(ctx, a.ID, id, digest)
	if err != nil || start || p.Outcome != "accepted" {
		t.Fatal("receipt was not retained")
	}
	_, start, err = db.BeginProbe(ctx, b.ID, id, digest)
	if err != nil || !start {
		t.Fatal("receipt crossed workspace")
	}
}
