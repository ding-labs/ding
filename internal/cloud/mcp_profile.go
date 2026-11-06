package cloud

import (
	"context"
	"slices"
	"time"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func (s *Server) mcpProfile(issuer string) func(context.Context, *auth.TokenInfo) (mcpcontract.Profile, error) {
	return func(ctx context.Context, token *auth.TokenInfo) (mcpcontract.Profile, error) {
		denied := mcpclient.Fail("integration_denied", "Reconnect Ding to identify this account.")
		if token == nil || !token.Expiration.IsZero() && !time.Now().Before(token.Expiration) {
			return mcpcontract.Profile{}, denied
		}
		user, client, scopes, err := s.mcpIdentity(ctx, issuer, "inspect", token)
		if err != nil {
			return mcpcontract.Profile{}, err
		}
		if !s.allowWorkspaceRequest("discovery:" + user.ID) {
			return mcpcontract.Profile{}, mcpclient.Fail("workspace_rate", "Account discovery rate exceeded; retry later.")
		}
		// A valid provider identity alone is not active Ding consent. Check usable
		// grants without selecting a workspace or inheriting browser authority.
		workspaces, err := s.approvedMCPWorkspaces(ctx, user.ID, client, scopes)
		if err != nil {
			return mcpcontract.Profile{}, err
		}
		for _, workspace := range workspaces {
			if slices.Contains(workspace.EffectiveScopes, "inspect") {
				return mcpcontract.Profile{ID: user.ID, Name: user.Name}, nil
			}
		}
		return mcpcontract.Profile{}, denied
	}
}
