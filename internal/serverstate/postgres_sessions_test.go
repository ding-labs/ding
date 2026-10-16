package state

import (
	"context"
	"testing"
	"time"
)

func TestPostgresSessionsDevicesAndOneUseLogin(t *testing.T) {
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
	session, err := db.NewSession(ctx, a.User, a.ID, "browser", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.Session(ctx, session.Token, now)
	if err != nil || !got.ValidCSRF(session.CSRF) || got.ValidCSRF("wrong") || got.User != a.User {
		t.Fatal("session binding mismatch", err)
	}
	if _, err = db.NewSession(ctx, b.User, a.ID, "browser", nil, now); err == nil {
		t.Fatal("nonmember received session")
	}
	if err = db.RevokeSession(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Session(ctx, session.Token, now); err == nil {
		t.Fatal("revoked session accepted")
	}
	verifier := ID()
	id, err := db.BeginDevice(ctx, TokenHash(verifier), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ready, err := db.ClaimDevice(ctx, id, verifier, now); err != nil || ready {
		t.Fatal("pending device consumed", err)
	}
	if err = db.ApproveDevice(ctx, id, a.User, a.ID, []string{"inspect"}, now); err != nil {
		t.Fatal(err)
	}
	if err = db.ApproveDevice(ctx, id, b.User, b.ID, []string{"inspect"}, now); err == nil {
		t.Fatal("device rebound")
	}
	if _, _, err = db.ClaimDevice(ctx, id, ID(), now); err == nil {
		t.Fatal("wrong verifier accepted")
	}
	if s, ready, err := db.ClaimDevice(ctx, id, verifier, now); err != nil || !ready || s.Workspace != a.ID {
		t.Fatal("device claim mismatch", err)
	}
	if _, _, err = db.ClaimDevice(ctx, id, verifier, now); err == nil {
		t.Fatal("device claim replayed")
	}
	vault, err := NewVault(db, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	flow := ID()
	login := Login{Verifier: ID(), Nonce: ID(), ReturnPath: "/ui/"}
	if err = vault.BeginLogin(ctx, flow, login, now); err != nil {
		t.Fatal(err)
	}
	if got, err := vault.ConsumeLogin(ctx, flow, now); err != nil || got != login {
		t.Fatal("login state mismatch", err)
	}
	if _, err = vault.ConsumeLogin(ctx, flow, now); err == nil {
		t.Fatal("login state replayed")
	}
}
