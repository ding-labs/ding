package state

import (
	"context"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/identity"
)

func TestBootstrapActivationAndRecoveryRevokeSessions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := db.Bootstrap(ctx, "Alice", "Alice", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Bootstrap(ctx, "another", "Another", "correct horse battery"); err == nil {
		t.Fatal("instance bootstrapped twice")
	}
	u, hash, err := db.PasswordUser(ctx, "alice")
	if err != nil || u.ID != a.User {
		t.Fatal(u, err)
	}
	if ok, err := identity.VerifyPassword(hash, "correct horse battery"); err != nil || !ok {
		t.Fatal("bootstrap password missing")
	}
	owner, _ := db.Principal(ctx, a.User, a.ID, "browser", nil)
	invite, err := db.Invite(ctx, owner, "bob", "Bob", access.Editor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.PasswordUser(ctx, "bob"); err == nil {
		t.Fatal("unactivated user can log in")
	}
	if err := db.Activate(ctx, invite.Token, "second correct password"); err != nil {
		t.Fatal(err)
	}
	if err := db.Activate(ctx, invite.Token, "replayed correct password"); err == nil {
		t.Fatal("activation replayed")
	}
	session, err := db.NewSession(ctx, invite.User, a.ID, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	reset, err := db.ResetInvitation(ctx, owner, invite.User)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Activate(ctx, reset.Token, "third correct password"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Session(ctx, session.Token, time.Now()); err == nil {
		t.Fatal("password reset kept old session")
	}
	if err := db.RecoverPassword(ctx, "alice", "replacement owner password"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Workspace(ctx, a.ID); err != nil {
		t.Fatal("recovery destroyed workspace")
	}
}

func TestInvitationCannotEscalatePrivileges(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "owner", 2)
	b, _ := db.Enroll(ctx, "issuer", "admin", 2)
	owner, _ := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err := db.ChangeMember(ctx, owner, b.User, access.Administrator); err != nil {
		t.Fatal(err)
	}
	admin, _ := db.Principal(ctx, b.User, a.ID, "browser", nil)
	if _, err := db.Invite(ctx, admin, "mallory", "Mallory", access.Owner); err == nil {
		t.Fatal("administrator created owner")
	}
	if _, err := db.ResetInvitation(ctx, admin, a.User); err == nil {
		t.Fatal("administrator reset owner")
	}
}
