package cloud

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"golang.org/x/time/rate"
)

type Server struct {
	DB                 *state.DB
	Vault              *state.Vault
	Pool               *Pool
	Login              LoginFlow
	PublicURL, Version string
	Assets             http.Handler
	TenantHandler      func(http.ResponseWriter, *http.Request, state.Session, *Tenant)
	mu                 sync.Mutex
	private            map[string]tenantAPI
	limits             map[string]*rate.Limiter
	requests           chan struct{}
	global             *rate.Limiter
}
type tenantAPI struct {
	handler http.Handler
	token   string
}

func (s *Server) Handler() (http.Handler, error) {
	origin, err := mcpconfig.Endpoint(s.PublicURL, true)
	if err != nil {
		return nil, err
	}
	s.PublicURL = origin
	if s.TenantHandler == nil {
		s.TenantHandler = s.serveTenant
	}
	u, _ := url.Parse(origin)
	s.private = map[string]tenantAPI{}
	s.limits = map[string]*rate.Limiter{}
	s.requests = make(chan struct{}, 32)
	s.global = rate.NewLimiter(30, 60)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Host != u.Host {
			cloudFail(w, 403, "invalid_host", "Unrecognized cloud host.")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.PublicURL {
			cloudFail(w, 403, "invalid_origin", "Request origin does not match Ding Cloud.")
			return
		}
		if !s.global.Allow() {
			cloudFail(w, 429, "busy", "Request rate exceeded; retry later.")
			return
		}
		select {
		case s.requests <- struct{}{}:
			defer func() { <-s.requests }()
		default:
			cloudFail(w, 503, "busy", "Cloud request capacity reached; retry later.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 96<<10)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		if r.Method == "GET" && r.URL.Path == "/" {
			http.Redirect(w, r, "/ui/", 302)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/ui/") || r.URL.Path == "/ui" {
			if s.Assets == nil {
				cloudFail(w, 503, "console_unavailable", "This cloud build has no Console assets.")
				return
			}
			if r.URL.Path == "/ui" {
				http.Redirect(w, r, "/ui/", 307)
				return
			}
			s.Assets.ServeHTTP(w, r)
			return
		}
		if r.Method == "GET" {
			switch r.URL.Path {
			case "/v1/cloud/config":
				cloudWrite(w, 200, map[string]any{"execution": "cloud", "signInURL": "/auth/login", "limits": Limits()})
				return
			case "/auth/login":
				s.Login.Begin(w, r)
				return
			case "/auth/callback":
				s.Login.Callback(w, r)
				return
			}
		}
		if s.devicePublic(w, r) {
			return
		}
		if strings.HasPrefix(r.URL.Path, "/connect/") {
			s.connectDevice(w, r)
			return
		}
		session, err := s.authenticate(r)
		if err != nil {
			cloudFail(w, 401, "sign_in_required", "Sign in to Ding Cloud. Local watches continue independently.")
			return
		}
		s.mu.Lock()
		limiter := s.limits[session.Account]
		if limiter == nil {
			limiter = rate.NewLimiter(2, 30)
			s.limits[session.Account] = limiter
		}
		s.mu.Unlock()
		if !limiter.Allow() {
			cloudFail(w, 429, "workspace_rate", "Workspace request rate exceeded; retry later.")
			return
		}
		if r.URL.Path == "/v1/browser/session" {
			s.browserSession(w, r, session)
			return
		}
		tenant, err := s.Pool.Get(r.Context(), session.Account)
		if err != nil {
			cloudFail(w, 503, "workspace_unavailable", "Workspace execution is unavailable; inspect service status or contact support.")
			return
		}
		if s.TenantHandler == nil {
			cloudFail(w, 503, "api_unavailable", "Tenant API is not configured.")
			return
		}
		s.TenantHandler(w, r, session, tenant)
	}), nil
}

func (s *Server) authenticate(r *http.Request) (state.Session, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		if !strings.HasPrefix(header, "Bearer ") {
			return state.Session{}, fmt.Errorf("invalid bearer")
		}
		session, err := s.DB.Session(r.Context(), strings.TrimPrefix(header, "Bearer "), time.Now())
		if err != nil || session.Kind != "device" {
			return state.Session{}, fmt.Errorf("invalid device session")
		}
		return session, nil
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return state.Session{}, err
	}
	session, err := s.DB.Session(r.Context(), cookie.Value, time.Now())
	if err != nil || session.Kind != "browser" {
		return state.Session{}, fmt.Errorf("invalid browser session")
	}
	if r.Method != "GET" && r.Method != "HEAD" && !session.ValidCSRF(r.Header.Get("X-Ding-CSRF")) {
		return state.Session{}, fmt.Errorf("CSRF required")
	}
	return session, nil
}

func (s *Server) browserSession(w http.ResponseWriter, r *http.Request, session state.Session) {
	if r.Method == "DELETE" {
		if err := s.DB.RevokeSession(r.Context(), session.Token); err != nil {
			cloudFail(w, 503, "logout_failed", "Could not revoke this session; retry.")
			return
		}
		cookie(w, sessionCookie, "", time.Unix(1, 0))
		cookie(w, csrfCookie, "", time.Unix(1, 0))
		cloudWrite(w, 200, map[string]bool{"signedOut": true})
		return
	}
	if r.Method != "GET" || session.Kind != "browser" {
		cloudFail(w, 405, "method_not_allowed", "Use browser sign-in to establish a Console session.")
		return
	}
	csrf, err := r.Cookie(csrfCookie)
	if err != nil || !session.ValidCSRF(csrf.Value) {
		cloudFail(w, 401, "session_incomplete", "Start a new sign-in to repair this browser session.")
		return
	}
	cloudWrite(w, 200, map[string]any{"csrf": csrf.Value, "expiresAt": session.ExpiresAt, "workspace": session.Account, "execution": "cloud"})
}

func (s *Server) api(t *Tenant) tenantAPI {
	s.mu.Lock()
	defer s.mu.Unlock()
	if api, ok := s.private[t.Account.ID]; ok {
		return api
	}
	c := control.Credentials{Admin: state.ID(), Ingest: state.ID()}
	api := tenantAPI{handler: control.ConsoleHandler(t.App, c, control.ConsoleConfig{Version: s.Version, Origin: s.PublicURL}), token: c.Admin}
	s.private[t.Account.ID] = api
	return api
}
