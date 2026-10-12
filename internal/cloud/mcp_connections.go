package cloud

import (
	"database/sql"
	"errors"
	"github.com/ding-labs/ding/internal/access"
	state "github.com/ding-labs/ding/internal/serverstate"
	"net/http"
	"slices"
	"strings"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func (s *Server) modelConnections(w http.ResponseWriter, r *http.Request, t *Tenant) {
	// Serialize consent/revocation, including the two durable stores. A crash may
	// leave an unusable orphan grant; it never creates a public implicit binding.
	principal, _ := access.FromContext(r.Context())
	if principal.Kind != "browser" {
		cloudFail(w, 403, "browser_required", "Use your browser to approve or revoke model access.")
		return
	}
	s.modelMu.Lock()
	defer s.modelMu.Unlock()
	if r.Method == "GET" {
		clients, err := s.DB.MCPClients(r.Context(), principal.User, t.Workspace.ID)
		if err != nil {
			cloudFail(w, 503, "connections_unavailable", "Could not read model connections.")
			return
		}
		result := []map[string]any{}
		for _, client := range clients {
			connection, err := s.Vault.MCP(r.Context(), principal.User, t.Workspace.ID, client)
			if err != nil {
				cloudFail(w, 503, "connections_unavailable", "A connection requires repair.")
				return
			}
			var grant store.IntegrationGrant
			err = t.App.Store.View(r.Context(), func(tx *store.Tx) error { var e error; grant, e = tx.IntegrationGrant(connection.GrantID); return e })
			if err != nil {
				cloudFail(w, 503, "connections_unavailable", "A connection requires repair.")
				return
			}
			result = append(result, map[string]any{"clientID": client, "grant": grant})
		}
		cloudWrite(w, 200, result)
		return
	}
	if r.Method != "POST" {
		cloudFail(w, 405, "method_not_allowed", "Use connection consent or revoke.")
		return
	}
	var input struct {
		Local    bool                  `json:"local"`
		ClientID string                `json:"clientID"`
		Revoke   bool                  `json:"revoke"`
		Grant    watchrun.GrantRequest `json:"grant"`
	}
	if decodeCloud(r, &input) != nil {
		cloudFail(w, 400, "invalid_client", "Provide connection details.")
		return
	}
	if input.Local {
		if !s.Shared {
			cloudFail(w, 403, "local_pairing_unavailable", "Cloud requires OAuth client consent.")
			return
		}
		input.ClientID = "local:" + state.ID()
	}
	if input.ClientID == "" || len(input.ClientID) > 512 || (!input.Local && !input.Revoke && strings.HasPrefix(input.ClientID, "local:")) {
		cloudFail(w, 400, "invalid_client", "Provide the OAuth client ID shown by the identity provider.")
		return
	}
	if input.Revoke {
		c, err := s.Vault.MCP(r.Context(), principal.User, t.Workspace.ID, input.ClientID)
		if errors.Is(err, sql.ErrNoRows) {
			cloudWrite(w, 200, map[string]bool{"revoked": true})
			return
		}
		var grant store.IntegrationGrant
		if err == nil {
			err = t.App.Store.Update(r.Context(), func(tx *store.Tx) error {
				var e error
				grant, e = tx.IntegrationGrant(c.GrantID)
				if e != nil {
					return e
				}
				return tx.RevokeIntegrationGrant(c.GrantID)
			})
		}
		if err != nil {
			cloudFail(w, 503, "revoke_failed", "Revocation could not be confirmed; retry.")
			return
		}
		// Revoke locally first. Retain the disabled binding until provider cleanup
		// succeeds, so failures (including a process crash) remain visible/retryable.
		// authKitIdentity refuses every token while this grant remains revoked.
		if grant.ProviderConsent != "" {
			subject, err := s.DB.IdentitySubject(r.Context(), principal.User, s.AuthKitIssuer)
			if err != nil || s.ConsentRevoker == nil {
				cloudFail(w, 503, "provider_revoke_pending", "Ding access is disabled. Hosted sign-in cleanup is unavailable; retry disconnect when it is configured.")
				return
			}
			if err := s.ConsentRevoker.Revoke(r.Context(), subject, input.ClientID); err != nil {
				cloudFail(w, 503, "provider_revoke_pending", "Ding access is disabled. Hosted sign-in cleanup failed; retry disconnect before reconnecting.")
				return
			}
		}
		if err := s.DB.UnbindMCP(r.Context(), principal.User, t.Workspace.ID, input.ClientID); err != nil {
			cloudFail(w, 503, "revoke_failed", "Ding access is disabled. Retry disconnect to finish cleanup.")
			return
		}
		cloudWrite(w, 200, map[string]bool{"revoked": true})
		return
	}
	if (!s.Shared && len(input.Grant.CommandRevisions) != 0) || input.Grant.Days > 30 {
		cloudFail(w, 400, "invalid_grant", "Cloud grants allow at most thirty days and no command revisions.")
		return
	}
	if (len(input.Grant.SecretRefs) > 0 || len(input.Grant.CommandRevisions) > 0) && !principal.Allows(access.Credentials) {
		cloudFail(w, 403, "resource_consent_denied", "Administrator permission is required to authorize credentials or commands.")
		return
	}
	for _, revision := range input.Grant.CommandRevisions {
		if !slices.Contains(t.App.ApprovedCommands, revision) {
			cloudFail(w, 403, "command_denied", "Command revision must be approved by the host operator.")
			return
		}
	}
	if slices.Contains(input.Grant.Scopes, "move") && (input.Local || !slices.Contains(s.MCPBridgeClients, input.ClientID)) {
		cloudFail(w, 403, "bridge_required", "Move consent is limited to the operator-approved native Ding bridge client.")
		return
	}
	for _, scope := range input.Grant.Scopes {
		if !principal.Allows(access.Action(scope)) {
			cloudFail(w, 403, "scope_denied", "This scope exceeds your role.")
			return
		}
	}
	names, err := s.Vault.Names(r.Context(), t.Workspace.ID)
	if err != nil {
		cloudFail(w, 503, "credentials_unavailable", "Could not check credential references.")
		return
	}
	for _, name := range input.Grant.SecretRefs {
		if !slices.Contains(names, name) {
			cloudFail(w, 400, "unknown_credential", "Select an existing credential name. Secret values never enter MCP tools.")
			return
		}
	}
	grant, err := t.App.CreateIntegrationGrant(r.Context(), input.Grant)
	if err != nil {
		cloudFail(w, 400, "invalid_grant", "Check grant name, scopes, days, and credential names.")
		return
	}
	c := mcpconfig.Connection{DaemonURL: s.PublicURL, Token: grant.Token, GrantID: grant.Grant.ID}
	if err := s.Vault.BindMCP(r.Context(), principal.User, t.Workspace.ID, input.ClientID, c); err != nil {
		_ = t.App.Store.Update(r.Context(), func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(grant.Grant.ID) })
		cloudFail(w, 409, "binding_conflict", "Disconnect the existing client before approving a replacement, or check the ten-client limit.")
		return
	}
	if input.Local {
		cloudWrite(w, 201, map[string]any{"clientID": input.ClientID, "grant": grant.Grant, "connection": c})
		return
	}
	cloudWrite(w, 201, map[string]any{"clientID": input.ClientID, "grant": grant.Grant})
}
