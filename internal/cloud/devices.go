package cloud

import (
	"github.com/ding-labs/ding/internal/access"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var devicePage = template.Must(template.New("device").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Connect Ding</title><style>body{font:18px system-ui;max-width:36rem;margin:10vh auto;padding:1.5rem;line-height:1.6}button,select{font:inherit;padding:.6rem}label{display:block}</style><main><h1>Connect your Ding client</h1><p>Approve only if you started a Ding login on your computer and it displays this code:</p><p><code>{{.Code}}</code></p><p>The credential expires in seven days. Choose a workspace and the permissions this client needs. Current membership is checked on every request.</p><form method="post"><input type="hidden" name="csrf" value="{{.CSRF}}"><label>Workspace<select name="workspace">{{range .Memberships}}<option value="{{.Workspace}}">{{.Name}} ({{.Role}})</option>{{end}}</select></label>{{range .Scopes}}<label><input type="checkbox" name="scope" value="{{.}}" {{if eq . "inspect"}}checked{{end}}> {{.}}</label>{{end}}<button>Approve selected access</button></form><p><a href="/ui/">Cancel</a></p></main></html>`))

func (s *Server) devicePublic(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "POST" {
		return false
	}
	switch r.URL.Path {
	case "/v1/cloud/device/start":
		var input struct {
			Challenge string `json:"challenge"`
		}
		if decodeCloud(r, &input) != nil || !accountID.MatchString(input.Challenge) {
			cloudFail(w, 400, "invalid_device", "Provide a SHA-256 device challenge.")
			return true
		}
		id, err := s.DB.BeginDevice(r.Context(), input.Challenge, time.Now())
		if err != nil {
			cloudFail(w, 503, "device_unavailable", "Try connecting later.")
			return true
		}
		cloudWrite(w, 200, map[string]any{"id": id, "verificationURL": s.PublicURL + "/connect/" + id, "code": id[:8], "expiresIn": 600, "interval": 5})
		return true
	case "/v1/cloud/device/claim":
		var input struct {
			ID       string `json:"id"`
			Verifier string `json:"verifier"`
		}
		if decodeCloud(r, &input) != nil {
			cloudFail(w, 400, "invalid_device", "Provide the device proof.")
			return true
		}
		session, ready, err := s.DB.ClaimDevice(r.Context(), input.ID, input.Verifier, time.Now())
		if err != nil {
			cloudFail(w, 400, "device_expired", "Start a new connection. An expired or consumed request cannot be claimed again.")
			return true
		}
		cloudWrite(w, 200, map[string]any{"ready": ready, "token": session.Token, "workspace": session.Workspace, "expiresAt": session.ExpiresAt})
		return true
	}
	return false
}

func (s *Server) connectDevice(w http.ResponseWriter, r *http.Request) {
	s.DB.Access.RLock()
	defer s.DB.Access.RUnlock()
	id := strings.TrimPrefix(r.URL.Path, "/connect/")
	if !accountID.MatchString(id) || !s.DB.DevicePending(r.Context(), id, time.Now()) {
		cloudFail(w, 410, "device_expired", "This connection expired or was already approved. Return to your terminal.")
		return
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		s.deviceSignIn(w, r)
		return
	}
	session, err := s.DB.Session(r.Context(), c.Value, time.Now())
	if err != nil || session.Kind != "browser" {
		s.deviceSignIn(w, r)
		return
	}
	csrf, err := r.Cookie(csrfCookie)
	if err != nil || !session.ValidCSRF(csrf.Value) {
		cloudFail(w, 401, "session_incomplete", "Sign in again to repair this session.")
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		memberships, err := s.DB.Memberships(r.Context(), session.User)
		if err != nil {
			cloudFail(w, 503, "memberships_unavailable", "Try again later.")
			return
		}
		_ = devicePage.Execute(w, map[string]any{"Code": id[:8], "CSRF": csrf.Value, "Memberships": memberships, "Scopes": []string{"inspect", "preview", "manage", "retry", "destinations", "credentials", "owners"}})
		return
	}
	if r.Method != "POST" {
		cloudFail(w, 405, "method_not_allowed", "Use the connection form.")
		return
	}
	if r.ParseForm() != nil || !session.ValidCSRF(r.Form.Get("csrf")) {
		cloudFail(w, 403, "csrf_required", "Refresh the connection page and try again.")
		return
	}
	workspace := r.Form.Get("workspace")
	p, err := s.DB.Principal(r.Context(), session.User, workspace, "browser", nil)
	scopes := r.Form["scope"]
	if err != nil || len(scopes) == 0 {
		cloudFail(w, 403, "access_denied", "Select an accessible workspace and permissions.")
		return
	}
	for _, scope := range scopes {
		if !p.Allows(access.Action(scope)) || scope == "host" || scope == "ingest" || scope == "members" || scope == "delete_workspace" {
			cloudFail(w, 403, "scope_denied", "Your role does not allow this permission.")
			return
		}
	}
	if err := s.DB.ApproveDevice(r.Context(), id, session.User, workspace, scopes, time.Now()); err != nil {
		cloudFail(w, 409, "approval_changed", "Return to your terminal to check this connection.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html><html lang=en><title>Connected</title><h1>Connection approved</h1><p>Return to your terminal. You can close this page.</p></html>"))
}

func (s *Server) deviceSignIn(w http.ResponseWriter, r *http.Request) {
	path := "/auth/login?return=" + url.QueryEscape(r.URL.Path)
	if s.Shared {
		path = "/ui/?return=" + url.QueryEscape(r.URL.Path)
	}
	http.Redirect(w, r, path, 302)
}
