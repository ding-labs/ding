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
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/services/"), "/")
	if len(parts) == 2 && parts[1] == "rotate" && r.Method == "POST" {
		var input struct {
			Days           int `json:"days"`
			OverlapSeconds int `json:"overlapSeconds"`
		}
		if decodeCloud(r, &input) != nil {
			cloudFail(w, 400, "invalid_request", "Provide expiry and overlap seconds.")
			return true
		}
		out, err := s.DB.RotateService(r.Context(), p, parts[0], input.Days, input.OverlapSeconds)
		if err != nil {
			cloudFail(w, 400, "rotation_denied", err.Error())
		} else {
			cloudWrite(w, 201, out)
		}
		return true
	}
	if len(parts) == 2 && parts[1] == "keys" && r.Method == "GET" {
		out, err := s.DB.ServiceKeys(r.Context(), p.Workspace, parts[0])
		if err != nil {
			cloudFail(w, 503, "keys_unavailable", "Cannot read key metadata.")
		} else {
			cloudWrite(w, 200, out)
		}
		return true
	}
	if len(parts) == 3 && parts[1] == "keys" && r.Method == "DELETE" {
		err := s.DB.RevokeServiceKey(r.Context(), p, parts[0], parts[2])
		if err != nil {
			cloudFail(w, 403, "revoke_denied", err.Error())
		} else {
			cloudWrite(w, 200, map[string]bool{"revoked": true})
		}
		return true
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/v1/services":
		out, err := s.DB.Services(r.Context(), p.Workspace)
		if err != nil {
			cloudFail(w, 503, "services_unavailable", "Services unavailable.")
		} else {
			cloudWrite(w, 200, out)
		}
	case (r.Method == "POST" && r.URL.Path == "/v1/services") || (r.Method == "PATCH" && len(parts) == 1):
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
		var lifetimes map[string]string
		var err error
		if slices.Contains(input.Scopes, "ingest") {
			lifetimes, err = s.producerLifetimes(r.Context(), p.Workspace, input.Watches)
			if err != nil {
				cloudFail(w, 400, "invalid_watch", "Select existing push watches in this workspace.")
				return true
			}
		}
		var out state.Service
		if r.Method == "PATCH" {
			out, err = s.DB.UpdateService(r.Context(), p, parts[0], input.Name, input.Scopes, input.Watches, lifetimes)
		} else {
			out, err = s.DB.CreateBoundService(r.Context(), p, input.Name, input.Scopes, input.Watches, input.Days, lifetimes)
		}
		if err != nil {
			cloudFail(w, 400, "service_denied", err.Error())
		} else {
			status := 201
			if r.Method == "PATCH" {
				status = 200
			}
			cloudWrite(w, status, out)
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
