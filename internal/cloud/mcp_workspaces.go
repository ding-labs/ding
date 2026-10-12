package cloud

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpcontract"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func (s *Server) mcpIdentity(ctx context.Context, issuer, scope string, token *auth.TokenInfo) (state.User, string, []string, error) {
	denied := mcpclient.Fail("integration_denied", "Connect this model client in the Ding Console; inspect its permissions and expiry.")
	if token == nil || token.Extra["issuer"] != issuer || !slices.Contains(token.Scopes, "ding:inspect") || !slices.Contains(token.Scopes, "ding:"+scope) {
		return state.User{}, "", nil, denied
	}
	subject, _ := token.Extra["subject"].(string)
	client, _ := token.Extra["client_id"].(string)
	if subject == "" || client == "" {
		return state.User{}, "", nil, denied
	}
	user, err := s.DB.IdentityUser(ctx, issuer, subject)
	if slices.Contains(s.AuthKitClients, client) {
		user, err = s.authKitIdentity(ctx, issuer, subject, client, token)
		if err != nil {
			return state.User{}, "", nil, err
		}
	}
	if err != nil {
		return state.User{}, "", nil, denied
	}
	scopes := []string{}
	for _, scope := range token.Scopes {
		if value, ok := strings.CutPrefix(scope, "ding:"); ok {
			scopes = append(scopes, value)
		}
	}
	return user, client, scopes, nil
}

func (s *Server) approvedMCPWorkspaces(ctx context.Context, user, client string, scopes []string) ([]mcpcontract.Workspace, error) {
	s.DB.Access.RLock()
	defer s.DB.Access.RUnlock()
	memberships, err := s.DB.MCPWorkspaces(ctx, user, client)
	if err != nil {
		return nil, mcpclient.Fail("workspaces_unavailable", "Could not read approved workspaces.")
	}
	out := []mcpcontract.Workspace{}
	for _, member := range memberships {
		connection, err := s.Vault.MCP(ctx, user, member.Workspace, client)
		if err != nil {
			return nil, mcpclient.Fail("connection_unavailable", "A model connection requires repair in the Console.")
		}
		grant, err := s.Pool.inspectMCPGrant(ctx, member.Workspace, connection.Token)
		if errors.Is(err, store.ErrIntegrationDenied) {
			continue
		}
		if err != nil {
			return nil, mcpclient.Fail("workspaces_unavailable", "Could not inspect workspace consent.")
		}
		p := access.Principal{User: user, Workspace: member.Workspace, Role: member.Role, Kind: "integration", Scopes: scopes}
		effective := []string{}
		for _, scope := range grant.Scopes {
			if p.Allows(access.Action(scope)) {
				effective = append(effective, scope)
			}
		}
		out = append(out, mcpcontract.Workspace{ID: member.Workspace, Name: member.Name, Role: string(member.Role), Execution: s.execution(), EffectiveScopes: effective, GrantExpiresAt: grant.ExpiresAt.UTC().Format(time.RFC3339)})
	}
	return out, nil
}

func (s *Server) listMCPWorkspaces(issuer string) func(context.Context, *auth.TokenInfo) ([]mcpcontract.Workspace, error) {
	return func(ctx context.Context, token *auth.TokenInfo) ([]mcpcontract.Workspace, error) {
		user, client, scopes, err := s.mcpIdentity(ctx, issuer, "inspect", token)
		if err != nil {
			return nil, err
		}
		if !s.allowWorkspaceRequest("discovery:" + user.ID) {
			return nil, mcpclient.Fail("workspace_rate", "Workspace discovery rate exceeded; retry later.")
		}
		return s.approvedMCPWorkspaces(ctx, user.ID, client, scopes)
	}
}
