package cloud

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ding-labs/ding/internal/identity"
	state "github.com/ding-labs/ding/internal/serverstate"
)

func TestEnableOIDCLoginPreservesPasswordAccountAndRequiresStoppedOwner(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	c := Config{Hosting: "cloud", LoginMethods: []string{"password"}, PublicURL: "https://cloud.ding.example", DataDir: filepath.Join(root, "data"), KeyFile: filepath.Join(root, "key")}
	if err := os.WriteFile(c.KeyFile, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(ctx, c, "owner", "Owner", "correct initial password", false); err != nil {
		t.Fatal(err)
	}
	db, err := state.Open(ctx, c.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	owner, hash, err := db.PasswordUser(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := db.Workspaces(ctx)
	if err != nil || len(workspaces) != 1 {
		t.Fatal("missing owner workspace", err)
	}
	c.LoginMethods = []string{"password", "oidc"}
	c.Identity = identity.Config{Issuer: "https://auth.example", ClientID: "console"}
	if err := EnableOIDCLogin(ctx, c); err == nil {
		t.Fatal("enabled provider while state was owned")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := EnableOIDCLogin(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	c.Identity.Issuer = "https://different.example"
	if err := EnableOIDCLogin(ctx, c); err == nil {
		t.Fatal("replaced pinned provider")
	}
	db, err = state.Open(ctx, c.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ConfigureLoginMethods(ctx, c.LoginMethods); err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigureOIDCIssuer(ctx, "https://auth.example"); err != nil {
		t.Fatal("failed replacement changed provider", err)
	}
	got, gotHash, err := db.PasswordUser(ctx, "owner")
	if err != nil || got != owner || gotHash != hash {
		t.Fatal("password account changed", err)
	}
	remaining, err := db.Workspaces(ctx)
	if err != nil || len(remaining) != 1 || remaining[0].ID != workspaces[0].ID {
		t.Fatal("existing workspace changed", err)
	}
	if _, err := db.Enroll(ctx, "https://auth.example", "new-person", 0); err == nil {
		t.Fatal("enabled provider bypassed closed enrollment")
	}
	fresh, err := db.Enroll(ctx, "https://auth.example", "new-person", 2)
	if err != nil || fresh.User == owner.ID || fresh.ID == workspaces[0].ID {
		t.Fatal("new identity inherited existing account", err)
	}
}
