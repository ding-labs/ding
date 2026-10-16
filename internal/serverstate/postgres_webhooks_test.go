package state

import (
	"context"
	"testing"
)

func TestPostgresWebhookSecretsRotationAndWorkspaceBoundary(t *testing.T) {
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
	vault, err := NewVault(db, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	const first = "synthetic-signing-secret-alpha"
	const second = "synthetic-signing-secret-beta"
	c, err := vault.CreateWebhook(ctx, owner, WebhookConnection{Name: "github", Provider: "github", Watch: "build", Lifetime: "lifetime", Repositories: []string{"123"}}, first)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := vault.Webhook(ctx, c.ID); err != nil || got.Secret != first || got.Workspace != a.ID {
		t.Fatal("webhook lookup mismatch", err)
	}
	if err = vault.RotateWebhook(ctx, other, c.ID, second, 3600); err == nil {
		t.Fatal("unrelated workspace rotated secret")
	}
	if err = vault.RotateWebhook(ctx, owner, c.ID, second, 3600); err != nil {
		t.Fatal(err)
	}
	if got, err := vault.Webhook(ctx, c.ID); err != nil || got.Secret != second || got.PreviousSecret != first {
		t.Fatal("rotation overlap mismatch", err)
	}
	if rows, err := db.Webhooks(ctx, b.ID); err != nil || len(rows) != 0 {
		t.Fatal("connection crossed workspace", err)
	}
	if err = db.DeleteWebhook(ctx, other, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = vault.Webhook(ctx, c.ID); err != nil {
		t.Fatal("unrelated workspace deleted webhook")
	}
	if err = db.DeleteWebhook(ctx, owner, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = vault.Webhook(ctx, c.ID); err == nil {
		t.Fatal("revoked webhook available")
	}
}
