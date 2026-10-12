package cloud

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

// Inspect consent without starting an execution worker. Pool ownership fences
// concurrent opens/deletion; the caller holds the membership admission lock.
func (p *Pool) inspectMCPGrant(ctx context.Context, workspace, token string) (store.IntegrationGrant, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.deleting[workspace] || !accountID.MatchString(workspace) {
		return store.IntegrationGrant{}, fmt.Errorf("workspace unavailable")
	}
	if tenant := p.tenants[workspace]; tenant != nil {
		return tenant.App.IntegrationAccess(ctx, token, "inspect")
	}
	db, err := store.Open(ctx, filepath.Join(p.root, "workspaces", workspace))
	if err != nil {
		return store.IntegrationGrant{}, err
	}
	defer db.Close()
	return watchrun.New(db).IntegrationAccess(ctx, token, "inspect")
}
