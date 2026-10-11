package cloud

import (
	"github.com/ding-labs/ding/internal/access"
	state "github.com/ding-labs/ding/internal/serverstate"
	"net/http"
	"slices"
	"strings"
)

func (s *Server) servicesAPI(w http.ResponseWriter, r *http.Request, p access.Principal) bool {
	if r.URL.Path != "/v1/services" && !strings.HasPrefix(r.URL.Path, "/v1/services/") {
		return false
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/v1/services":
		out, err := s.DB.Services(r.Context(), p.Workspace)
		if err != nil {
			cloudFail(w, 503, "services_unavailable", "Services unavailable.")
		} else {
			cloudWrite(w, 200, out)
		}
	case r.Method == "POST" && r.URL.Path == "/v1/services":
		var input struct {
			Name    string   `json:"name"`
			Scopes  []string `json:"scopes"`
			Watches []string `json:"watches"`
			Days    int      `json:"days"`
		}
		if decodeCloud(r, &input) != nil {
			cloudFail(w, 400, "invalid_request", "Provide a name, scopes, watch IDs and expiry.")
			return true
		}
		if !s.Shared && slices.Contains(input.Scopes, "ingest") {
			cloudFail(w, 400, "ingest_unavailable", "Cloud does not accept push sources.")
			return true
		}
		out, err := s.DB.CreateService(r.Context(), p, input.Name, input.Scopes, input.Watches, input.Days)
		if err != nil {
			cloudFail(w, 400, "service_denied", err.Error())
		} else {
			cloudWrite(w, 201, out)
		}
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/v1/services/"):
		err := s.DB.RevokeService(r.Context(), p, strings.TrimPrefix(r.URL.Path, "/v1/services/"))
		if err != nil {
			cloudFail(w, 403, "revoke_denied", err.Error())
		} else {
			cloudWrite(w, 200, map[string]bool{"revoked": true})
		}
	default:
		cloudFail(w, 405, "method_not_allowed", "Unsupported service operation.")
	}
	return true
}

func (s *Server) serveService(w http.ResponseWriter, r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ding_service_")
	if !ok {
		return false
	}
	grant, err := s.DB.ServiceToken(r.Context(), "ding_service_"+token)
	if err != nil {
		cloudFail(w, 401, "service_revoked", "Service credential expired or unavailable.")
		return true
	}
	p := access.Principal{User: "service:" + grant.ID, Workspace: grant.Workspace, Role: access.Editor, Kind: "service", Grant: grant.ID, Scopes: grant.Scopes}
	action, known := access.Route(r.Method, r.URL.Path)
	if !known || !p.Allows(action) || (r.Header.Get("X-Ding-Workspace") != "" && r.Header.Get("X-Ding-Workspace") != p.Workspace) {
		cloudFail(w, 403, "permission_denied", "Service credential does not permit this operation.")
		return true
	}
	if action == access.Ingest && (!s.Shared || !slices.Contains(grant.Watches, strings.TrimPrefix(r.URL.Path, "/v1/ingest/"))) {
		cloudFail(w, 403, "watch_denied", "Producer credential does not permit this watch.")
		return true
	}
	t, err := s.Pool.Get(r.Context(), p.Workspace)
	if err != nil {
		cloudFail(w, 503, "workspace_unavailable", "Workspace unavailable.")
		return true
	}
	r = r.Clone(access.WithOperation(access.WithPrincipal(r.Context(), p), r.Method, r.URL.Path))
	if action == access.Ingest {
		api := s.api(t)
		r.Header = r.Header.Clone()
		r.Header.Set("Authorization", "Bearer "+api.ingest)
		api.handler.ServeHTTP(w, r)
	} else {
		s.serveTenant(w, r, state.Session{Workspace: p.Workspace, User: p.User, Kind: "service", Scopes: p.Scopes}, t)
	}
	return true
}
