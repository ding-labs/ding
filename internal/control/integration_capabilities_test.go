package control

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/integrationpolicy"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestCapabilitiesIntersectMembershipOAuthAndGrant(t *testing.T) {
	app := watchrun.New(nil)
	app.Integration = integrationpolicy.Hosted("workspace", "Team", false)
	grant := store.IntegrationGrant{Scopes: []string{"inspect", "preview", "manage"}, ExpiresAt: time.Now().Add(time.Hour)}
	for _, tc := range []struct {
		role         access.Role
		scopes, want []string
	}{
		{access.Viewer, []string{"inspect", "preview", "manage"}, []string{"inspect"}},
		{access.Owner, []string{"inspect"}, []string{"inspect"}},
		{access.Owner, []string{"inspect", "preview", "manage", "retry"}, []string{"inspect", "preview", "manage"}},
	} {
		p := access.Principal{User: "user", Workspace: "workspace", Role: tc.role, Kind: "integration", Scopes: tc.scopes}
		caps := integrationCapabilities(access.WithPrincipal(context.Background(), p), app, "instance", grant)
		if !reflect.DeepEqual(caps["effectiveScopes"], tc.want) {
			t.Fatalf("%+v: %v", tc, caps["effectiveScopes"])
		}
		if caps["execution"] != "cloud" || caps["workspace"] != "workspace" || caps["maxManifestBytes"] != integrationpolicy.CloudManifestBytes {
			t.Fatal("wrong hosted capabilities", caps)
		}
		if !reflect.DeepEqual(caps["grant"], grant) {
			t.Fatal("stored consent was rewritten")
		}
	}
}
