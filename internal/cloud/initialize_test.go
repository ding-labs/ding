package cloud

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	state "github.com/ding-labs/ding/internal/serverstate"
)

func TestCloudBootstrapRecoveryAndExclusiveOwnership(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	c := Config{Hosting: "cloud", LoginMethods: []string{"password"}, PublicURL: "https://cloud.ding.example", DataDir: filepath.Join(root, "data"), KeyFile: filepath.Join(root, "key")}
	if err := os.WriteFile(c.KeyFile, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(ctx, c, "owner", "Owner", "correct initial password", false); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(ctx, c, "other", "Other", "correct other password", false); err == nil {
		t.Fatal("repeated init created owner")
	}
	db, err := state.Open(ctx, c.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := db.PasswordUser(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	memberships, err := db.Memberships(ctx, user.ID)
	if err != nil || len(memberships) != 1 {
		t.Fatal(err)
	}
	session, err := db.NewSession(ctx, user.ID, memberships[0].Workspace, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := Initialize(ctx, c, "owner", "Owner", "replacement password", true); err == nil {
		t.Fatal("recovery ran against live owner")
	}
	db.Close()
	if err := Initialize(ctx, c, "owner", "Owner", "replacement password", true); err != nil {
		t.Fatal(err)
	}
	db, err = state.Open(ctx, c.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Session(ctx, session.Token, time.Now()); err == nil {
		t.Fatal("recovery retained session")
	}
	workspaces, err := db.Workspaces(ctx)
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != memberships[0].Workspace {
		t.Fatal("recovery replaced workspace", err)
	}
}
