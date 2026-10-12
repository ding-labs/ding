package watchrun

import (
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
)

func TestHandoffRequiresExplicitMoveAndRechecksRevocation(t *testing.T) {
	source, target, prepare, pause := transferFixture(t)
	grant := func(app *App, move bool) (GrantPairing, context.Context) {
		scopes := []string{"inspect", "preview", "manage"}
		if move {
			scopes = append(scopes, "move")
		}
		g, err := app.CreateIntegrationGrant(ctx, GrantRequest{Name: "bridge", Days: 1, Scopes: scopes, SecretRefs: []string{"WEBHOOK_URL"}})
		if err != nil {
			t.Fatal(err)
		}
		return g, access.WithPrincipal(ctx, access.Principal{User: "user", Workspace: "workspace", Kind: "integration", Role: access.Owner, Scopes: scopes, Grant: g.Grant.ID})
	}
	_, old := grant(source, false)
	if _, err := source.PauseForHandoff(old, pause); err == nil {
		t.Fatal("ordinary manage grant paused for transfer")
	}
	g, approved := grant(source, true)
	p, _ := access.FromContext(approved)
	p.Role = access.Administrator
	if _, err := source.PauseForHandoff(access.WithPrincipal(ctx, p), pause); err == nil {
		t.Fatal("non-owner paused for transfer")
	}
	record, _ := source.Record(ctx, "api")
	if record.Status != "running" {
		t.Fatal("denial stopped source")
	}
	proof, err := source.PauseForHandoff(approved, pause)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Store.Update(ctx, func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(g.Grant.ID) }); err != nil {
		t.Fatal(err)
	}
	if _, err := source.PauseForHandoff(approved, pause); err == nil {
		t.Fatal("revoked pause receipt replay succeeded")
	}
	_, denied := grant(target, false)
	if _, err := target.ActivateHandoff(denied, prepare.ID, proof); err == nil {
		t.Fatal("ordinary grant activated target")
	}
	tg, allowed := grant(target, true)
	if _, err := target.ActivateHandoff(allowed, prepare.ID, proof); err != nil {
		t.Fatal(err)
	}
	if err := target.Store.Update(ctx, func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(tg.Grant.ID) }); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ActivateHandoff(allowed, prepare.ID, proof); err == nil {
		t.Fatal("revoked activation receipt replay succeeded")
	}
}
