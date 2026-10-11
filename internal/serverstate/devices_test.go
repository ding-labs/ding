package state

import (
	"context"
	"testing"
	"time"
)

func TestDeviceApprovalIsBoundToInitiatorAndConsumedOnce(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "a", 2)
	b, _ := db.Enroll(ctx, "issuer", "b", 2)
	now := time.Now()
	verifier := ID()
	id, err := db.BeginDevice(ctx, TokenHash(verifier), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ready, err := db.ClaimDevice(ctx, id, verifier, now); err != nil || ready {
		t.Fatal("unapproved device claimed", err)
	}
	if err := db.ApproveDevice(ctx, id, a.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ApproveDevice(ctx, id, b.ID, now); err == nil {
		t.Fatal("approval changed account")
	}
	if _, _, err := db.ClaimDevice(ctx, id, ID(), now); err == nil {
		t.Fatal("wrong device claimed")
	}
	s, ready, err := db.ClaimDevice(ctx, id, verifier, now)
	if err != nil || !ready || s.Account != a.ID {
		t.Fatal(s, ready, err)
	}
	if _, err := db.Session(ctx, s.Token, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.ClaimDevice(ctx, id, verifier, now); err == nil {
		t.Fatal("device approval replayed")
	}
	id, _ = db.BeginDevice(ctx, TokenHash(verifier), now)
	if err := db.ApproveDevice(ctx, id, a.ID, now.Add(11*time.Minute)); err == nil {
		t.Fatal("expired approval accepted")
	}
	if err := db.ApproveDevice(ctx, id, a.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.BeginAccountDelete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.ClaimDevice(ctx, id, verifier, now); err == nil {
		t.Fatal("deleting account issued a device connection")
	}
}
