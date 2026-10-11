package cloud

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/ding-labs/ding/internal/identity"
	state "github.com/ding-labs/ding/internal/serverstate"
	"golang.org/x/time/rate"
)

func (s *Server) execution() string {
	if s.Shared {
		return "shared"
	}
	return "cloud"
}

func (s *Server) passwordAttempt(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	limiter := s.passwordLimits[key]
	if limiter == nil {
		// Fully replenished entries no longer need memory. Reject capacity
		// pressure instead of evicting a throttled key and resetting its limit.
		if len(s.passwordLimits) >= 1024 {
			for k, v := range s.passwordLimits {
				if v.Tokens() >= 5 {
					delete(s.passwordLimits, k)
				}
			}
		}
		if len(s.passwordLimits) >= 1024 {
			return false
		}
		limiter = rate.NewLimiter(rate.Every(5*time.Second), 5)
		s.passwordLimits[key] = limiter
	}
	return limiter.Allow()
}

func (s *Server) publicIdentity(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == "GET" && r.URL.Path == "/v1/auth/config" {
		methods := []map[string]string{}
		if s.Shared {
			methods = append(methods, map[string]string{"id": "password", "label": "Instance account"})
		}
		if s.Login.Provider != nil {
			label := "Continue with GitHub"
			if s.Shared {
				label = "Company sign-in"
			}
			methods = append(methods, map[string]string{"id": "oidc", "label": label, "url": "/auth/login"})
		}
		cloudWrite(w, 200, map[string]any{"access": "shared", "execution": s.execution(), "name": s.Name, "methods": methods})
		return true
	}
	if r.Method != "POST" || (r.URL.Path != "/v1/auth/password" && r.URL.Path != "/v1/auth/activate") {
		return false
	}
	if !s.Shared {
		http.NotFound(w, r)
		return true
	}
	if r.Header.Get("Origin") != s.PublicURL {
		cloudFail(w, 403, "invalid_origin", "Use this instance's sign-in page.")
		return true
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Token    string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if decodeCloud(r, &input) != nil {
		cloudFail(w, 400, "invalid_request", "Provide sign-in details.")
		return true
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	username, _ := identity.Username(input.Username)
	if !s.passwordAttempt("ip:"+state.TokenHash(ip)) || !s.passwordAttempt("user:"+state.TokenHash(username)) {
		cloudFail(w, 429, "sign_in_limited", "Too many attempts. Try again shortly.")
		return true
	}
	if r.URL.Path == "/v1/auth/activate" {
		if err := s.DB.Activate(r.Context(), input.Token, input.Password); err != nil {
			cloudFail(w, 400, "activation_failed", err.Error())
			return true
		}
		cloudWrite(w, 200, map[string]bool{"activated": true})
		return true
	}
	s.DB.Access.RLock()
	defer s.DB.Access.RUnlock()
	user, hash, lookupErr := s.DB.PasswordUser(r.Context(), username)
	valid, err := identity.VerifyPassword(hash, input.Password)
	if errors.Is(err, identity.ErrPasswordBusy) {
		cloudFail(w, 503, "sign_in_busy", "Sign-in is busy. Try again shortly.")
		return true
	}
	if lookupErr != nil || err != nil || !valid {
		_ = s.DB.LoginFailure(r.Context(), username)
		cloudFail(w, 401, "sign_in_failed", "Sign-in details are invalid or access is unavailable.")
		return true
	}
	memberships, err := s.DB.Memberships(r.Context(), user.ID)
	if err != nil || len(memberships) == 0 {
		cloudFail(w, 403, "membership_required", "Ask an administrator for workspace access.")
		return true
	}
	session, err := s.DB.NewSession(r.Context(), user.ID, memberships[0].Workspace, "browser", nil, time.Now())
	if err != nil {
		cloudFail(w, 503, "session_unavailable", "Could not establish a session.")
		return true
	}
	if old, err := r.Cookie(sessionCookie); err == nil {
		_ = s.DB.RevokeSession(r.Context(), old.Value)
	}
	cookie(w, sessionCookie, session.Token, session.ExpiresAt)
	cookie(w, csrfCookie, session.CSRF, session.ExpiresAt)
	cloudWrite(w, 200, map[string]bool{"signedIn": true})
	return true
}
