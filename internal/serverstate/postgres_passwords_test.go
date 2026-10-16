package state

import (
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/identity"
)

func TestPostgresPasswordActivationRecoveryAndIdentityLink(t *testing.T) {
	db := postgresTestDB(t)
	ctx := context.Background()
	a, err := db.Bootstrap(ctx, "Alice", "Alice", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Bootstrap(ctx, "Other", "Other", "correct horse battery"); err == nil {
		t.Fatal("initialized twice")
	}
	owner, err := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := db.Invite(ctx, owner, "bob", "Bob", access.Editor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.PasswordUser(ctx, "bob"); err == nil {
		t.Fatal("unactivated user admitted")
	}
	if err = db.Activate(ctx, invite.Token, "second correct password"); err != nil {
		t.Fatal(err)
	}
	if err = db.Activate(ctx, invite.Token, "replayed correct password"); err == nil {
		t.Fatal("activation replayed")
	}
	if err = db.ChangePassword(ctx, invite.User, "second correct password", "third correct password"); err != nil {
		t.Fatal(err)
	}
	if err = db.DisableUser(ctx, owner, invite.User, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.PasswordUser(ctx, "bob"); err == nil {
		t.Fatal("disabled user admitted")
	}
	if err = db.DisableUser(ctx, owner, invite.User, false); err != nil {
		t.Fatal(err)
	}
	if err = db.RecoverPassword(ctx, "alice", "replacement owner password"); err != nil {
		t.Fatal(err)
	}
	_, hash, err := db.PasswordUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := identity.VerifyPassword(hash, "replacement owner password"); err != nil || !ok {
		t.Fatal("recovery password missing")
	}
	if err = db.ConfigureLoginMethods(ctx, []string{"password"}); err != nil {
		t.Fatal(err)
	}
	if err = db.EnableOIDC(ctx, IdentityLink{User: a.User, Issuer: "https://issuer.example", Subject: "immutable"}); err != nil {
		t.Fatal(err)
	}
	if got, err := db.IdentityUser(ctx, "https://issuer.example", "immutable"); err != nil || got.ID != a.User {
		t.Fatal("identity link changed user", err)
	}
	if err = db.EnableOIDC(ctx, IdentityLink{User: invite.User, Issuer: "https://issuer.example", Subject: "immutable"}); err == nil {
		t.Fatal("identity reassigned to another user")
	}
}
