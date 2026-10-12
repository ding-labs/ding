package cloud

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

// WorkOS owns sign-in, explicit client consent and refresh. This path is enabled
// only for publisher-registered clients. A verified consent materializes one
// personal workspace connection, without a second manual Ding consent screen.
func (s *Server) authKitIdentity(ctx context.Context, issuer, subject, client string, token *auth.TokenInfo) (state.User, error) {
	denied := mcpclient.Fail("integration_denied", "This Ding connection is unavailable. Reconnect through ChatGPT to approve new access.")
	consent, _ := token.Extra["consent_id"].(string)
	organization, _ := token.Extra["organization_id"].(string)
	// The initial hosted flow is for personal accounts. Never interpret an
	// organization-scoped token as authority over an unrelated personal workspace.
	if consent == "" || organization != "" {
		return state.User{}, denied
	}
	s.DB.Access.RLock()
	defer s.DB.Access.RUnlock()
	s.modelMu.Lock()
	defer s.modelMu.Unlock()
	limit := 0
	if s.Login.EnrollmentLimit != nil {
		limit = *s.Login.EnrollmentLimit
	}
	enrollment, err := s.DB.Enroll(ctx, issuer, subject, limit)
	if err != nil {
		return state.User{}, mcpclient.Fail("enrollment_unavailable", "Ding account access is unavailable or signup capacity has been reached.")
	}
	user, err := s.DB.IdentityUser(ctx, issuer, subject)
	if err != nil {
		return state.User{}, denied
	}
	memberships, err := s.DB.Memberships(ctx, user.ID)
	if err != nil || len(memberships) != 1 {
		return state.User{}, mcpclient.Fail("workspace_required", "This connection requires one personal workspace.")
	}
	principal, err := s.DB.Principal(ctx, user.ID, enrollment.ID, "integration", nil)
	if err != nil {
		return state.User{}, denied
	}
	// Permission comes from the intersection of provider consent and current role.
	principal.Kind = "browser"
	scopes := []string{}
	for _, action := range []string{"inspect", "preview", "manage", "retry"} {
		if slices.Contains(token.Scopes, "ding:"+action) && principal.Allows(access.Action(action)) {
			scopes = append(scopes, action)
		}
	}
	if !slices.Contains(scopes, "inspect") {
		return state.User{}, denied
	}
	tenant, err := s.Pool.Get(ctx, enrollment.ID)
	if err != nil {
		return state.User{}, mcpclient.Fail("workspace_unavailable", "Ding could not open your workspace.")
	}
	key := state.TokenHash(strings.Join([]string{issuer, subject, client, consent}, "\x00"))
	var grants []store.IntegrationGrant
	if err = tenant.App.Store.View(ctx, func(tx *store.Tx) error { var e error; grants, e = tx.IntegrationGrants(); return e }); err != nil {
		return state.User{}, denied
	}
	connection, connectionErr := s.Vault.MCP(ctx, user.ID, enrollment.ID, client)
	if connectionErr != nil && !errors.Is(connectionErr, sql.ErrNoRows) {
		return state.User{}, denied
	}
	for _, grant := range grants {
		// Revoked grants are durable tombstones. A retained JWT/refresh token with
		// that same provider consent can never re-create a disconnected binding.
		if grant.ProviderConsent == key && (grant.Revoked || connectionErr != nil || grant.ID != connection.GrantID) {
			return state.User{}, denied
		}
	}
	refs := []string{}
	if principal.Allows(access.Credentials) {
		refs, err = s.Vault.Names(ctx, enrollment.ID)
		if err != nil {
			return state.User{}, denied
		}
	}
	if connectionErr == nil {
		var existing store.IntegrationGrant
		for _, grant := range grants {
			if grant.ID == connection.GrantID {
				existing = grant
				break
			}
		}
		if existing.ProviderConsent == "" || existing.Revoked {
			return state.User{}, denied
		}
		if existing.ProviderConsent == key {
			err = tenant.App.Store.Update(ctx, func(tx *store.Tx) error {
				return tx.RenewProviderIntegrationGrant(existing.ID, key, scopes, refs, time.Now().Add(30*24*time.Hour))
			})
			return user, err
		}
		// A newly approved provider consent replaces the old connection. Revoking
		// its backing grant first leaves old calls fenced even if a later write fails.
		if err = tenant.App.Store.Update(ctx, func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(existing.ID) }); err != nil {
			return state.User{}, denied
		}
		if err = s.DB.UnbindMCP(ctx, user.ID, enrollment.ID, client); err != nil {
			return state.User{}, denied
		}
	}
	pairing, err := tenant.App.CreateProviderIntegrationGrant(ctx, watchrun.GrantRequest{Name: "ChatGPT", Scopes: scopes, SecretRefs: refs, Days: 30}, key)
	if err != nil {
		return state.User{}, denied
	}
	connection = mcpconfig.Connection{DaemonURL: s.PublicURL, Token: pairing.Token, GrantID: pairing.Grant.ID}
	if err = s.Vault.BindMCP(ctx, user.ID, enrollment.ID, client, connection); err != nil {
		_ = tenant.App.Store.Update(ctx, func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(pairing.Grant.ID) })
		return state.User{}, denied
	}
	return user, nil
}
