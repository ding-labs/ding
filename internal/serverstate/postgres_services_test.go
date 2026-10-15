package state

import (
	"context"
	"testing"
)

func TestPostgresServiceIdentityRotationAndWorkspaceIsolation(t *testing.T) {
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
	owner, err := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.Principal(ctx, b.User, b.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.CreateBoundService(ctx, owner, "producer", []string{"ingest", "inspect"}, []string{"build"}, 5, map[string]string{"build": "lifetime"})
	if err != nil {
		t.Fatal(err)
	}
	independent, err := db.CreateService(ctx, other, "producer", []string{"inspect"}, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := db.RotateService(ctx, owner, first.ID, 7, 3600)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first.Token, rotated.Token} {
		got, err := db.ServiceToken(ctx, token)
		if err != nil || got.ID != first.ID || got.Workspace != a.ID || got.Lifetimes["build"] != "lifetime" {
			t.Fatal("rotation changed logical producer or workspace", err)
		}
	}
	if keys, err := db.ServiceKeys(ctx, a.ID, first.ID); err != nil || len(keys) != 2 {
		t.Fatal("overlap keys mismatch", err)
	}
	if err = db.RevokeServiceKey(ctx, other, first.ID, first.KeyID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ServiceToken(ctx, first.Token); err != nil {
		t.Fatal("unrelated workspace revoked key")
	}
	if err = db.RevokeServiceKey(ctx, owner, first.ID, first.KeyID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ServiceToken(ctx, first.Token); err == nil {
		t.Fatal("revoked key accepted")
	}
	db.NoteServiceUse(ctx, rotated)
	if got, err := db.ServiceToken(ctx, rotated.Token); err != nil || got.LastUsedAt == nil {
		t.Fatal("usage diagnostic missing", err)
	}
	if _, err = db.UpdateService(ctx, owner, first.ID, "updated", []string{"inspect"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := db.ServiceToken(ctx, rotated.Token); err != nil || len(got.Scopes) != 1 || got.Scopes[0] != "inspect" {
		t.Fatal("scope update stale", err)
	}
	if err = db.RevokeService(ctx, owner, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ServiceToken(ctx, rotated.Token); err == nil {
		t.Fatal("revoked producer accepted")
	}
	if _, err = db.ServiceToken(ctx, independent.Token); err != nil {
		t.Fatal("revocation affected independent workspace", err)
	}
}
