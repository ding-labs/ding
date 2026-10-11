package cloud

import (
	"github.com/ding-labs/ding/internal/access"
	"net/http"
	"strings"
)

func (s *Server) serveLocalAgent(w http.ResponseWriter, r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ding_mcp_")
	if !ok || !s.Shared {
		return false
	}
	token = "ding_mcp_" + token
	action, known := access.Route(r.Method, r.URL.Path)
	if !known || !strings.HasPrefix(r.URL.Path, "/v1/integrations/") {
		cloudFail(w, 403, "integration_denied", "Use the scoped integration API.")
		return true
	}
	user, workspace, _, err := s.DB.LocalAgentBinding(r.Context(), token)
	if err != nil || (r.Header.Get("X-Ding-Workspace") != "" && r.Header.Get("X-Ding-Workspace") != workspace) {
		cloudFail(w, 403, "integration_denied", "Pairing expired or revoked.")
		return true
	}
	t, err := s.Pool.Get(r.Context(), workspace)
	if err != nil {
		cloudFail(w, 503, "workspace_unavailable", "Workspace unavailable.")
		return true
	}
	grant, err := t.App.IntegrationAccess(r.Context(), token, string(action))
	if err != nil {
		cloudFail(w, 403, "integration_denied", "Grant does not permit this action.")
		return true
	}
	p, err := s.DB.Principal(r.Context(), user, workspace, "integration", grant.Scopes)
	if err != nil || !p.Allows(action) {
		cloudFail(w, 403, "permission_denied", "Current membership does not permit this action.")
		return true
	}
	p.Grant = grant.ID
	r = r.Clone(access.WithOperation(access.WithPrincipal(r.Context(), p), r.Method, r.URL.Path))
	s.api(t).handler.ServeHTTP(w, r)
	return true
}
