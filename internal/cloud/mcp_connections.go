package cloud

import (
	"net/http"
	"slices"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func (s *Server) modelConnections(w http.ResponseWriter, r *http.Request, t *Tenant) {
	// Serialize consent/revocation, including the two durable stores. A crash may
	// leave an unusable orphan grant; it never creates a public implicit binding.
	s.modelMu.Lock()
	defer s.modelMu.Unlock()
	if r.Method == "GET" {
		clients, err := s.DB.MCPClients(r.Context(), t.Account.ID)
		if err != nil {
			cloudFail(w, 503, "connections_unavailable", "Could not read model connections.")
			return
		}
		result := []map[string]any{}
		for _, client := range clients {
			connection, err := s.Vault.MCP(r.Context(), t.Account.ID, client)
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
		ClientID string                `json:"clientID"`
		Revoke   bool                  `json:"revoke"`
		Grant    watchrun.GrantRequest `json:"grant"`
	}
	if decodeCloud(r, &input) != nil || input.ClientID == "" || len(input.ClientID) > 512 {
		cloudFail(w, 400, "invalid_client", "Provide the OAuth client ID shown by the identity provider.")
		return
	}
	if input.Revoke {
		c, _ := s.Vault.MCP(r.Context(), t.Account.ID, input.ClientID)
		if err := s.DB.UnbindMCP(r.Context(), t.Account.ID, input.ClientID); err != nil {
			cloudFail(w, 503, "revoke_failed", "Revocation could not be confirmed; retry.")
			return
		}
		if c.GrantID != "" {
			_ = t.App.Store.Update(r.Context(), func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(c.GrantID) })
		}
		cloudWrite(w, 200, map[string]bool{"revoked": true})
		return
	}
	if len(input.Grant.CommandRevisions) != 0 || input.Grant.Days > 30 {
		cloudFail(w, 400, "invalid_grant", "Cloud grants allow at most thirty days and no command revisions.")
		return
	}
	names, err := s.Vault.Names(r.Context(), t.Account.ID)
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
	if err := s.Vault.BindMCP(r.Context(), t.Account.ID, input.ClientID, c); err != nil {
		_ = t.App.Store.Update(r.Context(), func(tx *store.Tx) error { return tx.RevokeIntegrationGrant(grant.Grant.ID) })
		cloudFail(w, 409, "binding_conflict", "Disconnect the existing client before approving a replacement, or check the ten-client limit.")
		return
	}
	cloudWrite(w, 201, map[string]any{"clientID": input.ClientID, "grant": grant.Grant})
}
