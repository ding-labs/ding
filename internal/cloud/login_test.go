package cloud

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/ding-labs/ding/internal/identity"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	state "github.com/ding-labs/ding/internal/serverstate"
)

type fakeIdentity struct {
	nonce, verifier string
	called          int
}

func (f *fakeIdentity) Begin(state, nonce, verifier string) string {
	f.nonce, f.verifier = nonce, verifier
	return "https://issuer.example/authorize?state=" + state
}
func (f *fakeIdentity) Exchange(_ context.Context, code, nonce, verifier string) (identity.Identity, error) {
	f.called++
	if code != "verified-code" || nonce != f.nonce || verifier != f.verifier {
		return identity.Identity{}, fmt.Errorf("invalid transaction")
	}
	return identity.Identity{Issuer: "https://issuer.example", Subject: "immutable-user-id", Name: "Verified person"}, nil
}

func TestLoginRequiresBrowserStateAndCannotReplay(t *testing.T) {
	ctx := context.Background()
	db, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := make([]byte, 32)
	rand.Read(key)
	v, _ := state.NewVault(db, key)
	provider := &fakeIdentity{}
	f := LoginFlow{DB: db, Vault: v, Provider: provider}
	start := httptest.NewRecorder()
	f.Begin(start, httptest.NewRequest("GET", "https://ding.example/auth/login?return=https://evil.example", nil))
	if start.Code != 302 {
		t.Fatal(start.Code)
	}
	u, _ := url.Parse(start.Header().Get("Location"))
	id := u.Query().Get("state")
	callback := "https://ding.example/auth/callback?state=" + id + "&code=verified-code"
	wrong := httptest.NewRecorder()
	f.Callback(wrong, httptest.NewRequest("GET", callback, nil))
	if wrong.Code != 400 || provider.called != 0 {
		t.Fatal("missing browser state accepted")
	}
	r := httptest.NewRequest("GET", callback, nil)
	for _, c := range start.Result().Cookies() {
		if c.Name == loginCookie {
			if !c.Secure || !c.HttpOnly || c.Path != "/" {
				t.Fatal("unsafe login cookie")
			}
			r.AddCookie(c)
		}
	}
	finish := httptest.NewRecorder()
	f.Callback(finish, r)
	if finish.Code != 302 || finish.Header().Get("Location") != "/ui/" {
		t.Fatal("callback failed or open redirect", finish.Code, finish.Body.String())
	}
	var session *http.Cookie
	for _, c := range finish.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil || !session.Secure || !session.HttpOnly {
		t.Fatal("missing protected session")
	}
	again := httptest.NewRecorder()
	f.Callback(again, r)
	if again.Code != 400 || provider.called != 1 {
		t.Fatal("callback replayed")
	}
	workspaces, err := db.Workspaces(ctx)
	if err != nil || len(workspaces) != 1 || workspaces[0].Name != "Personal workspace" {
		t.Fatal("identity binding failed", err)
	}
	user, err := db.IdentityUser(ctx, "https://issuer.example", "immutable-user-id")
	if err != nil || user.Name != "Verified person" {
		t.Fatal("verified profile name was not initialized", user, err)
	}
}

func TestDestinationSetupLoginReturnIsBounded(t *testing.T) {
	account, workspace := strings.Repeat("a", 64), strings.Repeat("b", 64)
	target := "/ui/destination-setup?account=" + account + "&destination=alerts&workspace=" + workspace
	for _, bad := range []string{"https://evil.example" + target, "//evil.example" + target, "/ui/other?account=" + account, target + "#external", target + "&return=https://evil.example", target + "&account=" + account, strings.Replace(target, "alerts", "%2Fother", 1), "/ui/destination-setup?account=wrong"} {
		if got := loginReturnPath(bad); got != "/ui/" {
			t.Fatal("unsafe return", bad, got)
		}
	}
	db, err := state.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := make([]byte, 32)
	rand.Read(key)
	vault, _ := state.NewVault(db, key)
	provider := &fakeIdentity{}
	flow := LoginFlow{DB: db, Vault: vault, Provider: provider}
	for _, canceled := range []bool{false, true} {
		start := httptest.NewRecorder()
		flow.Begin(start, httptest.NewRequest("GET", "https://ding.example/auth/login?return="+url.QueryEscape(target), nil))
		redirect, _ := url.Parse(start.Header().Get("Location"))
		suffix := "&code=verified-code"
		want := target
		if canceled {
			suffix = "&error=access_denied"
			want += "&sign_in=canceled"
		}
		req := httptest.NewRequest("GET", "https://ding.example/auth/callback?state="+redirect.Query().Get("state")+suffix, nil)
		for _, cookie := range start.Result().Cookies() {
			if cookie.Name == loginCookie {
				req.AddCookie(cookie)
			}
		}
		finish := httptest.NewRecorder()
		flow.Callback(finish, req)
		if finish.Code != 302 || finish.Header().Get("Location") != want {
			t.Fatal("setup was lost during sign-in", finish.Code, finish.Header().Get("Location"))
		}
	}
}
