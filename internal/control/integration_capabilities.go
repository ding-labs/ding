package control

import (
	"context"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

type workspaceNameKey struct{}
type workspaceName struct{ ID, Name string }

// WithWorkspaceName supplies current presentation metadata without mutating the
// runner's immutable execution policy or restarting its watches.
func WithWorkspaceName(ctx context.Context, workspace, name string) context.Context {
	return context.WithValue(ctx, workspaceNameKey{}, workspaceName{workspace, name})
}

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
	name := profile.WorkspaceName
	if current, ok := ctx.Value(workspaceNameKey{}).(workspaceName); ok && current.ID == workspace {
		name = current.Name
	}
	return map[string]any{
		"version": watchrun.IntegrationVersion, "instance": instance, "grant": grant,
		"toolContractVersion": "ding.mcp/v1.2",
		"features":            map[string]bool{"workspaceSelection": profile.Execution != "local", "cloudStatus": profile.Execution == "cloud", "watchTransfer": false, "transferInspection": true},
		"execution":           profile.Execution, "workspace": workspace, "workspaceName": name,
		"effectiveScopes":       effective,
		"canManageDestinations": grant.Allows("manage", app.Now()) && (!present || p.Allows(access.Destinations)),
		"previewTTLSeconds":     900, "maxPageSize": 100,
		"maxManifestBytes": profile.MaxManifestBytes, "maxFixtureBytes": profile.MaxFixtureBytes,
		"maxRequestBytes": profile.MaxRequestBytes, "limits": app.Limits,
		"operationReceipts": "durable; no automatic expiration; maximum 10000", "eventsSubscriptions": false,
	}
}
