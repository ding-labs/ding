package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/access"
	state "github.com/ding-labs/ding/internal/serverstate"
)

func TestHostedSignInDoesNotExposeInstancePasswords(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.LoginMethods = []string{"oidc"}
	w := cloudRequest(h, "GET", "/v1/auth/config", "", sessions[0], false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"label":"Sign in or create an account"`) || strings.Contains(w.Body.String(), `"id":"password"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if s.accountFeatures().Passwords {
		t.Fatal("hosted accounts must not offer instance password management")
	}
	for _, path := range []string{"/v1/auth/password", "/v1/auth/activate"} {
		if w := cloudRequest(h, "POST", path, `{}`, sessions[0], true); w.Code != 404 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}

func TestBuiltInLoginUsesCloudSessionAndRoleBoundary(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.LoginMethods = []string{"password"}
	s.TenantHandler = s.serveTenant
	ctx := context.Background()
	if s.Shared || s.Pool.Shared {
		t.Fatal("password login enabled self-hosted execution")
	}
	if w := cloudRequest(h, "GET", "/v1/auth/config", "", sessions[0], false); !strings.Contains(w.Body.String(), `"execution":"cloud"`) || !strings.Contains(w.Body.String(), `"id":"password"`) {
		t.Fatal(w.Body.String())
	}
	for _, manifest := range []string{strings.ReplaceAll(fixture, "https://example.com/health", "http://127.0.0.1/private"), strings.ReplaceAll(fixture, "every: 5m", "every: 1s")} {
		body, _ := json.Marshal(map[string]any{"manifest": manifest, "dryRun": true})
		if w := cloudRequest(h, "POST", "/v1/apply", string(body), sessions[0], true); w.Code == 200 {
			t.Fatal("password owner bypassed cloud policy")
		}
	}
	owner, _ := s.DB.Principal(ctx, sessions[0].User, sessions[0].Workspace, "browser", nil)
	invite, err := s.DB.Invite(ctx, owner, "reader", "Reader", access.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Activate(ctx, invite.Token, "reader correct password"); err != nil {
		t.Fatal(err)
	}
	login := func(origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://ding.example/v1/auth/password", strings.NewReader(`{"username":"READER","password":"reader correct password"}`))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := login(""); w.Code != 403 {
		t.Fatal("login CSRF", w.Code)
	}
	w := login("https://ding.example")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var session state.Session
	for _, c := range w.Result().Cookies() {
		if !c.Secure || !c.HttpOnly {
			t.Fatal("unsafe cookie")
		}
		switch c.Name {
		case sessionCookie:
			session.Token = c.Value
		case csrfCookie:
			session.CSRF = c.Value
		}
	}
	if w := cloudRequest(h, "GET", "/v1/browser/session", "", session, false); w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"viewer"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := cloudRequest(h, "POST", "/v1/apply", "{}", session, true); w.Code != 403 {
		t.Fatal("password session bypassed roles", w.Code)
	}
	if w := cloudRequest(h, "DELETE", "/v1/user", fmt.Sprintf(`{"confirmUser":%q}`, invite.User), session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := cloudRequest(h, "GET", "/v1/watches", "", session, false); w.Code != 401 {
		t.Fatal("deleted user session survived", w.Code)
	}
}

func TestBrowserIdentitySurvivesLosingDefaultWorkspace(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	ctx := context.Background()
	owner, _ := s.DB.Principal(ctx, sessions[1].User, sessions[1].Workspace, "browser", nil)
	if err := s.DB.ChangeMember(ctx, owner, sessions[0].User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.BeginWorkspaceDelete(ctx, sessions[0].Workspace); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.FinishWorkspaceDelete(ctx, sessions[0].Workspace); err != nil {
		t.Fatal(err)
	}
	w := cloudRequest(h, "GET", "/v1/browser/session", "", sessions[0], false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), sessions[1].Workspace) || !strings.Contains(w.Body.String(), `"role":"viewer"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
