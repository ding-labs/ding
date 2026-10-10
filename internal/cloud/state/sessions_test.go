package state

import (
	"context"
	"crypto/rand"
	"testing"
	"time"
)

func TestSessionExpiryRevocationAndOneUseLogin(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "subject", 1)
	now := time.Now()
	s, err := db.NewSession(ctx, a.ID, "browser", now)
	if err != nil {
		t.Fatal(err)
	}
	read, err := db.Session(ctx, s.Token, now)
	if err != nil || !read.ValidCSRF(s.CSRF) || read.ValidCSRF("bad") || read.ValidCSRF("") {
		t.Fatal("session or CSRF failed", err)
	}
	if _, err := db.Session(ctx, s.Token, now.Add(8*24*time.Hour)); err == nil {
		t.Fatal("expired session accepted")
	}
	if err := db.RevokeSession(ctx, s.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Session(ctx, s.Token, now); err == nil {
		t.Fatal("revoked session accepted")
	}
	key := make([]byte, 32)
	rand.Read(key)
	v, _ := NewVault(db, key)
	flow := ID()
	login := Login{Verifier: ID(), Nonce: ID(), ReturnPath: "/ui/"}
	if err := v.BeginLogin(ctx, flow, login, now); err != nil {
		t.Fatal(err)
	}
	got, err := v.ConsumeLogin(ctx, flow, now)
	if err != nil || got != login {
		t.Fatal("login mismatch", err)
	}
	if _, err := v.ConsumeLogin(ctx, flow, now); err == nil {
		t.Fatal("replayed login")
	}
}
