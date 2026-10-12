package watchrun

import (
	"context"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/plan"
	"github.com/ding-labs/ding/internal/store"
)

// AuthorizeIntegrationHandoff bounds the trusted bridge transport. The same
// guard runs inside each handoff transaction, including receipt replay.
func (a *App) AuthorizeIntegrationHandoff(ctx context.Context, manifest, watchID string) error {
	p, ok := access.FromContext(ctx)
	if !ok || p.Grant == "" {
		return access.ErrDenied
	}
	return a.Store.View(ctx, func(tx *store.Tx) error { return a.authorizeHandoff(ctx, tx, manifest, watchID) })
}

func (a *App) authorizeHandoff(ctx context.Context, tx *store.Tx, manifest, watchID string) error {
	if err := access.Require(ctx, access.Move); err != nil {
		return err
	}
	p, ok := access.FromContext(ctx)
	if !ok || p.Grant == "" {
		return nil
	} // Existing owner/CLI flow.
	grant, err := requireIntegration(tx, p.Grant, "move", a.Now())
	if err != nil {
		return err
	}
	if !grant.Allows("preview", a.Now()) || !grant.Allows("manage", a.Now()) {
		return store.ErrIntegrationDenied
	}
	if manifest == "" {
		manifest, err = exportWatch(tx, watchID)
		if err != nil {
			return err
		}
	}
	bundle, err := plan.Parse([]byte(manifest))
	if err != nil {
		return err
	}
	return commandAccess(grant, bundle)
}
