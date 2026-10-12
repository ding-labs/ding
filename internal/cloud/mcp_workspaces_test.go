package cloud

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpserver"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestHostedWorkspaceDiscoverySelectionAndRevocation(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler, s.MCP = s.serveTenant, http.NotFoundHandler()
	ctx := context.Background()
	owner, err := s.DB.Principal(ctx, sessions[1].User, sessions[1].Workspace, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB.ChangeMember(ctx, owner, sessions[0].User, access.Editor); err != nil {
		t.Fatal(err)
	}
	second, err := s.DB.NewSession(ctx, sessions[0].User, sessions[1].Workspace, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []state.Session{sessions[0], second} {
		consent := `{"clientID":"client","grant":{"name":"test","days":1,"scopes":["inspect","preview","manage"]}}`
		if w := cloudRequest(h, "POST", "/v1/cloud/models", consent, session, true); w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	token := &auth.TokenInfo{Scopes: []string{"ding:inspect", "ding:preview", "ding:manage"}, Extra: map[string]any{"issuer": "issuer", "subject": "a", "client_id": "client"}}
	list := s.listMCPWorkspaces("issuer")
	workspaces, err := list(ctx, token)
	if err != nil || len(workspaces) != 2 {
		t.Fatal(workspaces, err)
	}
	resolve := s.resolveMCP("issuer")
	if _, err := resolve(ctx, "inspect", token); err == nil || !strings.Contains(err.Error(), "workspace_required") {
		t.Fatal("ambiguous target accepted", err)
	}
	for _, id := range []string{sessions[0].Workspace, second.Workspace} {
		client, err := resolve(mcpserver.WithWorkspace(ctx, id), "inspect", token)
		if err != nil || client.Target.Workspace != id {
			t.Fatal("wrong target", err)
		}
		caps, err := client.Call(ctx, "GET", "/capabilities", nil, nil)
		if err != nil || caps.(map[string]any)["workspace"] != id {
			t.Fatal(caps, err)
		}
	}
	conflict := context.WithValue(ctx, mcpWorkspaceKey{}, sessions[0].Workspace)
	if _, err := resolve(mcpserver.WithWorkspace(conflict, second.Workspace), "inspect", token); err == nil || !strings.Contains(err.Error(), "invalid_target") {
		t.Fatal("conflicting target accepted", err)
	}
	if _, err := resolve(mcpserver.WithWorkspace(ctx, strings.Repeat("a", 64)), "inspect", token); err == nil {
		t.Fatal("unapproved target accepted")
	}
	if err := s.DB.ChangeMember(ctx, owner, sessions[0].User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	workspaces, err = list(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	for _, workspace := range workspaces {
		if workspace.ID == second.Workspace && slices.Contains(workspace.EffectiveScopes, "manage") {
			t.Fatal("stale membership authority", workspace)
		}
	}
	if _, err := resolve(mcpserver.WithWorkspace(ctx, second.Workspace), "manage", token); err == nil {
		t.Fatal("downgraded member retained management")
	}
	if err := s.DB.ChangeMember(ctx, owner, sessions[0].User, ""); err != nil {
		t.Fatal(err)
	}
	workspaces, err = list(ctx, token)
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != sessions[0].Workspace {
		t.Fatal("revoked membership still discoverable", workspaces, err)
	}
	if _, err := resolve(ctx, "inspect", token); err != nil {
		t.Fatal("single usable workspace not selected", err)
	}

	tenant, err := s.Pool.Get(ctx, sessions[0].Workspace)
	if err != nil {
		t.Fatal(err)
	}
	expired := store.IntegrationGrant{ID: state.ID(), Name: "expired", Scopes: []string{"inspect"}, ExpiresAt: time.Now().Add(-time.Hour)}
	credential := "ding_mcp_" + state.ID()
	if err := tenant.App.Store.Update(ctx, func(tx *store.Tx) error {
		return tx.CreateIntegrationGrant(expired, watchrun.IntegrationTokenHash(credential))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Vault.BindMCP(ctx, sessions[0].User, sessions[0].Workspace, "expired-client", mcpconfig.Connection{DaemonURL: s.PublicURL, Token: credential, GrantID: expired.ID}); err != nil {
		t.Fatal(err)
	}
	token.Extra["client_id"] = "expired-client"
	workspaces, err = list(ctx, token)
	if err != nil || len(workspaces) != 0 {
		t.Fatal("expired consent was discoverable", workspaces, err)
	}
}
