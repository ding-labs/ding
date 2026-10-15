package state

import (
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/access"
)

func TestPostgresIdentityMembershipAndPinnedSettings(t *testing.T) {
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
	if a.ID == a.User || a.ID == b.ID {
		t.Fatal("identity conflation")
	}
	if _, err = db.Principal(ctx, b.User, a.ID, "browser", nil); err == nil {
		t.Fatal("unrelated user admitted")
	}
	owner, err := db.Principal(ctx, a.User, a.ID, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ChangeMember(ctx, owner, b.User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := db.Principal(ctx, b.User, a.ID, "browser", nil)
	if err != nil || !viewer.Allows(access.Inspect) || viewer.Allows(access.Manage) {
		t.Fatal("membership scope mismatch", err)
	}
	if err = db.ChangeMember(ctx, owner, b.User, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Principal(ctx, b.User, a.ID, "browser", nil); err == nil {
		t.Fatal("removed member admitted")
	}
	if _, err = db.Enroll(ctx, "issuer", "other", 2); err == nil {
		t.Fatal("enrollment quota bypassed")
	}
	if again, err := db.Enroll(ctx, "issuer", "alice", 0); err != nil || again.ID != a.ID {
		t.Fatal("closed admission lost existing account", err)
	}
	for i := 0; i < 2; i++ {
		if err = db.ConfigureHosting(ctx, "self-hosted"); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.ConfigureHosting(ctx, "cloud"); err == nil {
		t.Fatal("hosting identity changed")
	}
	if err = db.ConfigureLoginMethods(ctx, []string{"password"}); err != nil {
		t.Fatal(err)
	}
	if err = db.ConfigureDataKey(ctx, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if rows, err := db.Audit(ctx, a.ID, 0); err != nil || len(rows) != 2 {
		t.Fatal("control audit mismatch", err)
	}
}
