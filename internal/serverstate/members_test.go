package state

import (
	"context"
	"github.com/ding-labs/ding/internal/access"
	"testing"
	"time"
)

func TestMemberAdministrationCannotRemoveLastOwnerOrEscalate(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "owner", 2)
	b, _ := db.Enroll(ctx, "issuer", "member", 2)
	owner, _ := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err := db.ChangeMember(ctx, owner, a.User, access.Editor); err == nil {
		t.Fatal("removed last owner")
	}
	if err := db.ChangeMember(ctx, owner, b.User, access.Administrator); err != nil {
		t.Fatal(err)
	}
	admin, _ := db.Principal(ctx, b.User, a.ID, "browser", nil)
	if err := db.ChangeMember(ctx, admin, b.User, access.Owner); err == nil {
		t.Fatal("administrator self-promoted")
	}
	if err := db.DeleteUser(ctx, a.User); err == nil {
		t.Fatal("deleted sole owner")
	}
	if err := db.ChangeMember(ctx, owner, b.User, access.Owner); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, a.User); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Workspace(ctx, a.ID); err != nil {
		t.Fatal("user deletion removed team data")
	}
	if err := db.ChangeMember(ctx, owner, b.User, access.Viewer); err == nil {
		t.Fatal("stale actor authorized")
	}
	events, err := db.Audit(ctx, a.ID, 0)
	if err != nil || len(events) != 2 {
		t.Fatal("membership audit missing", events, err)
	}
}

func TestRemovedMembershipDoesNotResurrectDeviceCredentials(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := db.Enroll(ctx, "issuer", "owner", 2)
	b, _ := db.Enroll(ctx, "issuer", "member", 2)
	p, _ := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err := db.ChangeMember(ctx, p, b.User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	session, err := db.NewSession(ctx, b.User, a.ID, "device", []string{"inspect"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ChangeMember(ctx, p, b.User, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.ChangeMember(ctx, p, b.User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Session(ctx, session.Token, time.Now()); err == nil {
		t.Fatal("re-adding member revived old credential")
	}
}
