package state

import (
	"context"
	"github.com/ding-labs/ding/internal/access"
	"testing"
)

func TestUsersAndWorkspacesHaveIndependentIdentity(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := db.Enroll(ctx, "issuer", "alice", 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.Enroll(ctx, "issuer", "bob", 2)
	if err != nil {
		t.Fatal(err)
	}
	if a.User == a.ID || a.User == b.User {
		t.Fatal("conflated user and workspace identity")
	}
	if _, err := db.Principal(ctx, b.User, a.ID, "browser", nil); err == nil {
		t.Fatal("nonmember admitted")
	}
	if _, err := db.sql.ExecContext(ctx, "INSERT INTO memberships(user,workspace,role) VALUES(?,?,?)", b.User, a.ID, access.Viewer); err != nil {
		t.Fatal(err)
	}
	p, err := db.Principal(ctx, b.User, a.ID, "browser", nil)
	if err != nil || !p.Allows(access.Inspect) || p.Allows(access.Manage) {
		t.Fatal("incorrect member permissions", p, err)
	}
	if _, err := db.sql.ExecContext(ctx, "DELETE FROM users WHERE id=?", b.User); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Workspace(ctx, a.ID); err != nil {
		t.Fatal("user removal deleted shared workspace", err)
	}
	if _, err := db.Principal(ctx, b.User, a.ID, "browser", nil); err == nil {
		t.Fatal("deleted member admitted")
	}
}
