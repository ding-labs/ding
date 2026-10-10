package cloud

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/cloud/egress"
	"github.com/ding-labs/ding/internal/cloud/state"
)

func testCloudServer(t *testing.T) (*Server, http.Handler, []state.Session) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	db, err := state.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := make([]byte, 32)
	rand.Read(key)
	v, _ := state.NewVault(db, key)
	client, closeHTTP := (egress.Guard{}).Client()
	t.Cleanup(closeHTTP)
	p := NewPool(ctx, root, db, v, client)
	t.Cleanup(func() { p.Close() })
	s := &Server{DB: db, Vault: v, Pool: p, PublicURL: "https://ding.example", Version: "test"}
	s.Login = LoginFlow{DB: db, Vault: v, Provider: &fakeIdentity{}}
	s.TenantHandler = func(w http.ResponseWriter, _ *http.Request, _ state.Session, t *Tenant) {
		cloudWrite(w, 200, map[string]string{"account": t.Account.ID})
	}
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	var sessions []state.Session
	for _, subject := range []string{"a", "b"} {
		a, err := db.Enroll(ctx, "issuer", subject, 2)
		if err != nil {
			t.Fatal(err)
		}
		session, err := db.NewSession(ctx, a.ID, "browser", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, session)
	}
	return s, h, sessions
}

func cloudRequest(h http.Handler, method, path, body string, session state.Session, csrf bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://ding.example"+path, strings.NewReader(body))
	if session.Token != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: session.Token})
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: session.CSRF})
	}
	if csrf {
		r.Header.Set("X-Ding-CSRF", session.CSRF)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCloudBoundaryDerivesWorkspaceFromSessionAndRequiresCSRF(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	if w := cloudRequest(h, "GET", "/v1/cloud/config", "", state.Session{}, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := cloudRequest(h, "GET", "/v1/watches", "", state.Session{}, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, session := range sessions {
		w := cloudRequest(h, "GET", "/v1/watches?workspace=other", "", session, false)
		if w.Code != 200 || !strings.Contains(w.Body.String(), session.Account) {
			t.Fatal("workspace override", w.Code, w.Body.String())
		}
	}
	if w := cloudRequest(h, "POST", "/v1/apply", "{}", sessions[0], false); w.Code != 401 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	if w := cloudRequest(h, "POST", "/v1/apply", "{}", sessions[0], true); w.Code != 200 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "https://evil.example/v1/watches", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign host accepted")
	}
	if w := cloudRequest(h, "DELETE", "/v1/browser/session", "", sessions[0], true); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := s.DB.Session(context.Background(), sessions[0].Token, time.Now()); err == nil {
		t.Fatal("logout did not revoke session")
	}
}
