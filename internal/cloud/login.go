package cloud

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ding-labs/ding/internal/cloud/state"
	"github.com/ding-labs/ding/internal/watch"
	"golang.org/x/oauth2"
)

const sessionCookie = "__Host-ding-session"
const csrfCookie = "__Host-ding-csrf"
const loginCookie = "__Host-ding-login"

type LoginFlow struct {
	DB       *state.DB
	Vault    *state.Vault
	Provider IdentityProvider
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
	back := "/ui/"
	if next := r.URL.Query().Get("return"); strings.HasPrefix(next, "/connect/") && accountID.MatchString(strings.TrimPrefix(next, "/connect/")) {
		back = next
	}
	id := state.ID()
	login := state.Login{Nonce: state.ID(), Verifier: oauth2.GenerateVerifier(), ReturnPath: back}
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
		http.Redirect(w, r, "/ui/?sign_in=canceled", http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 4096 {
		cloudFail(w, 400, "invalid_login", "Missing authorization code. Start a new sign-in.")
		return
	}
	identity, err := f.Provider.Exchange(r.Context(), code, login.Nonce, login.Verifier)
	if err != nil {
		cloudFail(w, 401, "identity_failed", "Could not verify sign-in. Start a new sign-in.")
		return
	}
	account, err := f.DB.Enroll(r.Context(), identity.Issuer, identity.Subject, MaxAccounts)
	if err != nil {
		cloudFail(w, 503, "enrollment_unavailable", "Cloud enrollment is currently unavailable. Local and self-hosted Ding remain free and available.")
		return
	}
	session, err := f.DB.NewSession(r.Context(), account.ID, "browser", time.Now())
	if err != nil {
		cloudFail(w, 503, "session_unavailable", "Could not create a cloud session. Try again later.")
		return
	}
	cookie(w, sessionCookie, session.Token, session.ExpiresAt)
	cookie(w, csrfCookie, session.CSRF, session.ExpiresAt)
	http.Redirect(w, r, login.ReturnPath, http.StatusFound)
}
