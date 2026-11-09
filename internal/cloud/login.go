package cloud

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/ding-labs/ding/internal/identity"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/ding-labs/ding/internal/watch"
	"golang.org/x/oauth2"
)

const sessionCookie = "__Host-ding-session"
const csrfCookie = "__Host-ding-csrf"
const loginCookie = "__Host-ding-login"

type LoginFlow struct {
	homepage        *homepageAnalytics
	Shared          bool
	EnrollmentLimit *int
	DB              *state.DB
	Vault           *state.Vault
	Provider        identity.Provider
}

func cloudWrite(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Data: value})
}
func cloudFail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(watch.Envelope{APIVersion: watch.APIVersion, Error: &watch.Error{Code: code, Message: message}})
}
func cookie(w http.ResponseWriter, name, value string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expiry})
}

func (f LoginFlow) Begin(w http.ResponseWriter, r *http.Request) {
	back := loginReturnPath(r.URL.Query().Get("return"))
	id := state.ID()
	login := state.Login{Nonce: state.ID(), Verifier: oauth2.GenerateVerifier(), ReturnPath: back}
	if h := r.URL.Query().Get("homepage_handoff"); f.homepage != nil && homepageHandoff.MatchString(h) {
		login.HomepageHandoff = h
	}
	if err := f.Vault.BeginLogin(r.Context(), id, login, time.Now()); err != nil {
		cloudFail(w, 503, "login_unavailable", "Sign-in is temporarily unavailable. Local watches continue running.")
		return
	}
	cookie(w, loginCookie, id, time.Now().Add(10*time.Minute))
	http.Redirect(w, r, f.Provider.Begin(id, login.Nonce, login.Verifier), http.StatusFound)
}

func (f LoginFlow) Callback(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("state")
	browser, err := r.Cookie(loginCookie)
	if err != nil || len(id) != 64 || subtle.ConstantTimeCompare([]byte(id), []byte(browser.Value)) != 1 {
		cloudFail(w, 400, "invalid_login", "This sign-in does not belong to this browser. Start a new sign-in.")
		return
	}
	login, err := f.Vault.ConsumeLogin(r.Context(), id, time.Now())
	cookie(w, loginCookie, "", time.Unix(1, 0))
	if err != nil {
		cloudFail(w, 400, "expired_login", "This sign-in expired or was already used. Start a new sign-in.")
		return
	}
	if r.URL.Query().Get("error") != "" {
		back := "/ui/?sign_in=canceled"
		if strings.HasPrefix(login.ReturnPath, "/ui/destination-setup?") {
			back = login.ReturnPath + "&sign_in=canceled"
		}
		http.Redirect(w, r, back, http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 4096 {
		cloudFail(w, 400, "invalid_login", "Missing authorization code. Start a new sign-in.")
		return
	}
	identity, err := f.Provider.Exchange(r.Context(), code, login.Nonce, login.Verifier)
	if err != nil {
		_ = f.DB.LoginFailure(r.Context(), "oidc")
		cloudFail(w, 401, "identity_failed", "Could not verify sign-in. Start a new sign-in.")
		return
	}
	limit := MaxAccounts
	if f.EnrollmentLimit != nil {
		limit = *f.EnrollmentLimit
	}
	var workspace state.Enrollment
	if f.Shared {
		workspace, err = f.DB.IdentityWorkspace(r.Context(), identity.Issuer, identity.Subject)
	} else {
		workspace, err = f.DB.Enroll(r.Context(), identity.Issuer, identity.Subject, limit)
	}
	if err != nil {
		cloudFail(w, 503, "enrollment_unavailable", "Cloud enrollment is currently unavailable. Local and self-hosted Ding remain free and available.")
		return
	}
	if identity.Name != "" {
		if err := f.DB.InitializeDisplayName(r.Context(), workspace.User, identity.Name); err != nil {
			cloudFail(w, 503, "profile_unavailable", "Could not initialize your account profile. Try signing in again.")
			return
		}
	}
	session, err := f.DB.NewSession(r.Context(), workspace.User, workspace.ID, "browser", nil, time.Now())
	if err != nil {
		cloudFail(w, 503, "session_unavailable", "Could not create a cloud session. Try again later.")
		return
	}
	cookie(w, sessionCookie, session.Token, session.ExpiresAt)
	cookie(w, csrfCookie, session.CSRF, session.ExpiresAt)
	http.Redirect(w, r, login.ReturnPath, http.StatusFound)
	if f.homepage != nil && login.HomepageHandoff != "" {
		// Release the redirect before recording attribution. A measurement
		// outage cannot prevent a verified user from reaching their Console.
		_ = http.NewResponseController(w).Flush()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 4*time.Second)
		defer cancel()
		if err := f.homepage.confirm(ctx, identity.Subject, login.HomepageHandoff); err != nil {
			log.Print("homepage signup attribution unavailable")
		}
	}
}

// Only explicit Console setup coordinates may survive sign-in. They identify a
// requested view, never an account to authenticate or a workspace to authorize.
func loginReturnPath(next string) string {
	if strings.HasPrefix(next, "/connect/") && accountID.MatchString(strings.TrimPrefix(next, "/connect/")) {
		return next
	}
	u, err := url.Parse(next)
	if err != nil || len(next) > 1024 || u.IsAbs() || u.Host != "" || u.User != nil || u.Path != "/ui/destination-setup" || u.RawPath != "" || u.Fragment != "" {
		return "/ui/"
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || !accountID.MatchString(q.Get("account")) || !accountID.MatchString(q.Get("workspace")) {
		return "/ui/"
	}
	for key, values := range q {
		if len(values) != 1 || (key != "account" && key != "workspace" && key != "destination") {
			return "/ui/"
		}
	}
	if len(q.Get("destination")) > 64 || strings.ContainsAny(q.Get("destination"), "/\\\r\n\x00") {
		return "/ui/"
	}
	return "/ui/destination-setup?" + q.Encode()
}
