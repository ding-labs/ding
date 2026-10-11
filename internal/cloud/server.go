package cloud

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/mcpconfig"
	state "github.com/ding-labs/ding/internal/serverstate"
	"golang.org/x/time/rate"
)

type Server struct {
	Railway            bool
	LoginMethods       []string
	Shared             bool // Self-hosted execution with named accounts; cloud is the default.
	Name               string
	passwordLimits     map[string]*rate.Limiter
	DB                 *state.DB
	Vault              *state.Vault
	Pool               *Pool
	Login              LoginFlow
	PublicURL, Version string
	Assets             http.Handler
	MCP                http.Handler
	TenantHandler      func(http.ResponseWriter, *http.Request, state.Session, *Tenant)
	mu                 sync.Mutex
	modelMu            sync.Mutex
	moveMu             sync.Mutex
	private            map[string]tenantAPI
	limits             map[string]*rate.Limiter
	requests           chan struct{}
	global             *rate.Limiter
}
type tenantAPI struct {
	ingest  string
	handler http.Handler
	token   string
	done    <-chan struct{}
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
	s.passwordLimits = map[string]*rate.Limiter{}
	s.limits = map[string]*rate.Limiter{}
	s.requests = make(chan struct{}, 32)
	s.global = rate.NewLimiter(30, 60)
	health := rate.NewLimiter(5, 10)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		probe := r.Method == "GET" && r.URL.Path == "/healthz"
		if r.Host != u.Host && !(s.Railway && probe && r.Host == "healthcheck.railway.app") {
			cloudFail(w, 403, "invalid_host", "Unrecognized cloud host.")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.PublicURL {
			cloudFail(w, 403, "invalid_origin", "Request origin does not match Ding Cloud.")
			return
		}
		if probe {
			if !health.Allow() {
				cloudFail(w, 429, "busy", "Retry health check later.")
				return
			}
			if !s.Pool.Ready() {
				cloudFail(w, 503, "worker_unavailable", "Execution worker unavailable.")
			} else {
				cloudWrite(w, 200, map[string]string{"status": "ready"})
			}
			return
		}
		if s.Railway && r.Header.Get("X-Forwarded-Proto") != "https" {
			cloudFail(w, 403, "https_required", "Use the public HTTPS origin.")
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
		if s.publicIdentity(w, r) {
			return
		}
		if r.URL.Path == "/mcp" || r.URL.Path == "/.well-known/oauth-protected-resource/mcp" {
			if s.MCP == nil {
				http.NotFound(w, r)
			} else {
				s.MCP.ServeHTTP(w, r)
			}
			return
		}
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
				if s.Login.Provider == nil {
					http.Redirect(w, r, "/ui/", 302)
					return
				}
				s.Login.Begin(w, r)
				return
			case "/auth/callback":
				if s.Login.Provider == nil {
					http.NotFound(w, r)
					return
				}
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
		// Membership mutations take the exclusive lock inside their transaction.
		// All ordinary operations retain a read lock from authentication through
		// completion, so a completed revocation fences subsequent operations.
		administration := r.Method != "GET" && (r.URL.Path == "/v1/services" || strings.HasPrefix(r.URL.Path, "/v1/services/") || r.URL.Path == "/v1/members" || strings.HasPrefix(r.URL.Path, "/v1/members/") || r.URL.Path == "/v1/user" || strings.HasPrefix(r.URL.Path, "/v1/user/"))
		if !administration {
			s.DB.Access.RLock()
			defer s.DB.Access.RUnlock()
		}
		if s.serveLocalAgent(w, r) {
			return
		}
		if s.serveService(w, r) {
			return
		}
		session, err := s.authenticate(r)
		if err != nil {
			cloudFail(w, 401, "sign_in_required", "Sign in to this Ding instance.")
			return
		}
		if expected := r.Header.Get("X-Ding-Session-User"); expected != "" && expected != session.User {
			cloudFail(w, 401, "identity_changed", "This browser signed into a different user. Reconnect to continue.")
			return
		}
		if r.URL.Path == "/v1/browser/session" && r.Method == "DELETE" {
			s.browserSession(w, r, session)
			return
		}
		if s.userAPI(w, r, session) {
			return
		}
		if workspace := r.Header.Get("X-Ding-Workspace"); workspace != "" {
			if session.Kind != "browser" && workspace != session.Workspace {
				cloudFail(w, 403, "permission_denied", "This credential is bound to another workspace.")
				return
			}
			session.Workspace = workspace
		}
		principal, err := s.DB.Principal(r.Context(), session.User, session.Workspace, session.Kind, session.Scopes)
		if err != nil && session.Kind == "browser" && r.URL.Path == "/v1/browser/session" && r.Method == "GET" && r.Header.Get("X-Ding-Workspace") == "" {
			memberships, e := s.DB.Memberships(r.Context(), session.User)
			if e == nil && len(memberships) > 0 {
				session.Workspace = memberships[0].Workspace
				principal, err = s.DB.Principal(r.Context(), session.User, session.Workspace, session.Kind, session.Scopes)
			}
		}
		if err != nil {
			cloudFail(w, 403, "permission_denied", "Workspace access is unavailable.")
			return
		}
		r = r.WithContext(access.WithOperation(access.WithPrincipal(r.Context(), principal), r.Method, r.URL.Path))
		s.mu.Lock()
		limiter := s.limits[session.Workspace]
		if limiter == nil {
			// Session-only clients may never enter a tenant API. Bound this
			// expendable cache across enrollment/deletion churn as well.
			if len(s.limits) >= MaxAccounts*2 {
				for id := range s.limits {
					delete(s.limits, id)
					break
				}
			}
			limiter = rate.NewLimiter(2, 30)
			s.limits[session.Workspace] = limiter
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
		if s.userAPI(w, r, session) {
			return
		}
		action, known := access.Route(r.Method, r.URL.Path)
		if !known || action == access.Host || action == access.Ingest {
			cloudFail(w, 404, "not_found", "This operation is not available.")
			return
		}
		if !principal.Allows(action) {
			cloudFail(w, 403, "permission_denied", "Your role or credential does not permit this operation.")
			return
		}
		if s.servicesAPI(w, r, principal) {
			return
		}
		if s.membersAPI(w, r, principal) {
			return
		}
		tenant, err := s.Pool.Get(r.Context(), session.Workspace)
		if err != nil {
			cloudFail(w, 503, "workspace_unavailable", "Workspace execution is unavailable; inspect service status or contact support.")
			return
		}
		requestCtx, stop := context.WithCancel(r.Context())
		defer stop()
		unlink := context.AfterFunc(tenant.ctx, stop)
		defer unlink()
		r = r.WithContext(requestCtx)
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
	principal, _ := access.FromContext(r.Context())
	user, err := s.DB.User(r.Context(), session.User)
	if err != nil {
		cloudFail(w, 503, "session_unavailable", "User unavailable.")
		return
	}
	memberships, err := s.DB.Memberships(r.Context(), session.User)
	if err != nil {
		cloudFail(w, 503, "session_unavailable", "Memberships unavailable.")
		return
	}
	cloudWrite(w, 200, map[string]any{"csrf": csrf.Value, "expiresAt": session.ExpiresAt, "workspace": session.Workspace, "execution": s.execution(), "user": user, "memberships": memberships, "role": principal.Role, "capabilities": principal.Capabilities(), "accountFeatures": s.accountFeatures()})
}

func (s *Server) api(t *Tenant) tenantAPI {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cached := range s.private {
		select {
		case <-cached.done:
			delete(s.private, id)
			delete(s.limits, id)
		default:
		}
	}
	if api, ok := s.private[t.Workspace.ID]; ok {
		return api
	}
	c := control.Credentials{Admin: state.ID(), Ingest: state.ID()}
	api := tenantAPI{handler: control.ConsoleHandler(t.App, c, control.ConsoleConfig{Version: s.Version, Origin: s.PublicURL}), token: c.Admin, ingest: c.Ingest, done: t.ctx.Done()}
	s.private[t.Workspace.ID] = api
	return api
}
