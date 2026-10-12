package watchrun

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
)

func TestProviderGrantRefreshCannotRestoreRevocation(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := New(db)
	consent := strings.Repeat("a", 64)
	g, err := app.CreateProviderIntegrationGrant(ctx, GrantRequest{Name: "ChatGPT", Scopes: []string{"inspect"}, Days: 30}, consent)
	if err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(45 * 24 * time.Hour)
	app.Now = func() time.Time { return later }
	if _, err = app.IntegrationAccess(ctx, g.Token, "inspect"); err == nil {
		t.Fatal("expired backing grant accepted before provider refresh")
	}
	renew := func(key string) error {
		return db.Update(ctx, func(tx *store.Tx) error {
			return tx.RenewProviderIntegrationGrant(g.Grant.ID, key, []string{"inspect"}, nil, later.Add(30*24*time.Hour))
		})
	}
	if err = renew(strings.Repeat("b", 64)); err == nil {
		t.Fatal("another consent renewed grant")
	}
	if err = renew(consent); err != nil {
		t.Fatal(err)
	}
	if _, err = app.IntegrationAccess(ctx, g.Token, "inspect"); err != nil {
		t.Fatal(err)
	}
	if err = db.Update(ctx, func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(g.Grant.ID) }); err != nil {
		t.Fatal(err)
	}
	if err = renew(consent); err == nil {
		t.Fatal("provider refresh restored revoked access")
	}
}
