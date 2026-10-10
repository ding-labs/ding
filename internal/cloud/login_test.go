package cloud

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ding-labs/ding/internal/cloud/state"
)

type fakeIdentity struct {
	nonce, verifier string
	called          int
}

func (f *fakeIdentity) Begin(state, nonce, verifier string) string {
	f.nonce, f.verifier = nonce, verifier
	return "https://issuer.example/authorize?state=" + state
}
func (f *fakeIdentity) Exchange(_ context.Context, code, nonce, verifier string) (Identity, error) {
	f.called++
	if code != "verified-code" || nonce != f.nonce || verifier != f.verifier {
		return Identity{}, fmt.Errorf("invalid transaction")
	}
	return Identity{"https://issuer.example", "immutable-user-id"}, nil
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
	accounts, err := db.Accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].Subject != "immutable-user-id" {
		t.Fatal("identity binding failed", err)
	}
}
