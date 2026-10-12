package control

import (
	"context"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func integrationCapabilities(ctx context.Context, app *watchrun.App, instance string, grant store.IntegrationGrant) map[string]any {
	p, present := access.FromContext(ctx)
	effective := []string{}
	for _, scope := range grant.Scopes {
		if grant.Allows(scope, app.Now()) && (!present || p.Allows(access.Action(scope))) {
			effective = append(effective, scope)
		}
	}
	profile := app.Integration
	workspace := profile.Workspace
	if workspace == "" {
		workspace = instance
	}
	return map[string]any{
		"version": watchrun.IntegrationVersion, "instance": instance, "grant": grant,
		"execution": profile.Execution, "workspace": workspace, "workspaceName": profile.WorkspaceName,
		"effectiveScopes":       effective,
		"canManageDestinations": grant.Allows("manage", app.Now()) && (!present || p.Allows(access.Destinations)),
		"previewTTLSeconds":     900, "maxPageSize": 100,
		"maxManifestBytes": profile.MaxManifestBytes, "maxFixtureBytes": profile.MaxFixtureBytes,
		"maxRequestBytes": profile.MaxRequestBytes, "limits": app.Limits,
		"operationReceipts": "durable; no automatic expiration; maximum 10000", "eventsSubscriptions": false,
	}
}
