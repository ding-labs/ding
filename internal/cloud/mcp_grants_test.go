package cloud

import (
	"context"
	"testing"

	"github.com/ding-labs/ding/internal/watchrun"
)

func TestInspectMCPGrantDoesNotStartWorker(t *testing.T) {
	s, _, sessions := testCloudServer(t)
	ctx := context.Background()
	tenant, err := s.Pool.Get(ctx, sessions[0].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := tenant.App.CreateIntegrationGrant(ctx, watchrun.GrantRequest{Name: "test", Days: 1, Scopes: []string{"inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.Close(); err != nil {
		t.Fatal(err)
	}
	p := NewPool(ctx, s.Pool.root, s.DB, s.Vault, s.Pool.http)
	defer p.Close()
	got, err := p.inspectMCPGrant(ctx, sessions[0].Workspace, grant.Token)
	if err != nil || got.ID != grant.Grant.ID {
		t.Fatal(got, err)
	}
	if len(p.tenants) != 0 {
		t.Fatal("discovery started execution")
	}
}
