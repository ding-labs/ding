package state

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPostgresUsageProbeRetentionAndDeletion(t *testing.T) {
	db := postgresTestDB(t)
	ctx := context.Background()
	a, err := db.Enroll(ctx, "issuer", "alice", 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.Enroll(ctx, "issuer", "bob", 2)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	budget := OutboundBudget{Checks: 1, Deliveries: 1, Bytes: RequestReservation * 2}
	r, err := db.ReserveBudget(ctx, a.ID, false, now, budget)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = db.Finish(ctx, r, 100); err != nil {
			t.Fatal(err)
		}
	}
	if usage, err := db.Usage(ctx, a.ID, now); err != nil || usage.Checks != 1 || usage.Bytes != 100 {
		t.Fatal("reservation refunded twice", err)
	}
	if _, err = db.ReserveBudget(ctx, a.ID, false, now, budget); err == nil {
		t.Fatal("outbound budget bypassed")
	}
	if _, err = db.ReserveBudget(ctx, b.ID, false, now, budget); err != nil {
		t.Fatal("workspace budget leaked", err)
	}
	id := strings.Repeat("a", 32)
	digest := strings.Repeat("b", 64)
	if _, fresh, err := db.BeginProbe(ctx, a.ID, id, digest); err != nil || !fresh {
		t.Fatal("probe reserve mismatch", err)
	}
	if _, fresh, err := db.BeginProbe(ctx, a.ID, id, digest); err != nil || fresh {
		t.Fatal("probe repeated external action", err)
	}
	if err = db.FinishProbe(ctx, a.ID, id, "accepted"); err != nil {
		t.Fatal(err)
	}
	proofID := strings.Repeat("c", 64)
	proof := HandoffProof{Revision: "revision", At: now}
	if err = db.SaveHandoffProof(ctx, a.ID, proofID, proof); err != nil {
		t.Fatal(err)
	}
	if got, err := db.HandoffProof(ctx, a.ID, proofID); err != nil || got.Revision != proof.Revision {
		t.Fatal("proof mismatch", err)
	}
	if _, err = db.HandoffProof(ctx, b.ID, proofID); err == nil {
		t.Fatal("proof crossed workspace")
	}
	if err = db.Collect(ctx, now.Add(30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = db.BeginWorkspaceDelete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Workspace(ctx, a.ID); err == nil {
		t.Fatal("deleting workspace still admitted")
	}
	if err = db.FinishWorkspaceDelete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Workspace(ctx, b.ID); err != nil {
		t.Fatal("deleted independent workspace", err)
	}
}
