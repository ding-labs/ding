package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
)

func TestBrowserBoundary(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("i", 64)}
	now := time.Now()
	cfg := ConsoleConfig{Version: "test", Origin: "http://127.0.0.1:7676", Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("console")) }), now: func() time.Time { return now }}
	h := ConsoleHandler(watchrun.New(s), c, cfg)
	call := func(method, path, body, origin, host, token, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:7676"+path, strings.NewReader(body))
		if host != "" {
			r.Host = host
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if csrf != "" {
			r.Header.Set("X-Ding-CSRF", csrf)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	mint := func() BrowserHandoff {
		t.Helper()
		w := call("POST", "/v1/browser/handoff", "{}", "", "", c.Admin, "", nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var e struct {
			Data BrowserHandoff `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		return e.Data
	}
	launch := mint()
	if strings.Contains(launch.URL, c.Admin) || !strings.Contains(launch.URL, "#handoff=") {
		t.Fatal("unsafe launch URL")
	}
	body := `{"token":"` + launch.Token + `"}`
	for _, bad := range []struct{ origin, host string }{{"https://evil.example", ""}, {cfg.Origin, "evil.example"}, {"", ""}} {
		if w := call("POST", "/v1/browser/session", body, bad.origin, bad.host, "", "", nil); w.Code != 403 {
			t.Fatal("origin bypass", w.Code)
		}
	}
	w := call("POST", "/v1/browser/session", body, cfg.Origin, "", "", "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/v1" {
		t.Fatal("unsafe cookie")
	}
	var e struct {
		Data BrowserSession `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &e)
	if e.Data.CSRF == "" || strings.Contains(w.Body.String(), c.Admin) {
		t.Fatal("session credential exposure")
	}
	if w := call("POST", "/v1/browser/session", body, cfg.Origin, "", "", "", nil); w.Code != 401 {
		t.Fatal("reused handoff")
	}
	for _, path := range []string{"/v1/info", "/v1/watches", "/v1/browser/session"} {
		if w := call("GET", path, "", "", "", "", "", cookie); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	for _, bad := range []struct{ origin, csrf, host string }{{"", "", ""}, {cfg.Origin, "bad", ""}, {"https://evil.example", e.Data.CSRF, ""}, {cfg.Origin, e.Data.CSRF, "evil.example"}} {
		if w := call("POST", "/v1/apply", "{}", bad.origin, bad.host, "", bad.csrf, cookie); w.Code != 403 && w.Code != 401 {
			t.Fatal("mutation bypass", w.Code)
		}
	}
	if w := call("POST", "/v1/browser/handoff", "{}", cfg.Origin, "", "", e.Data.CSRF, cookie); w.Code != 403 {
		t.Fatal("cookie minted another session", w.Code)
	}
	if w := call("POST", "/v1/ingest/api", "{}", cfg.Origin, "", "", e.Data.CSRF, cookie); w.Code != 401 {
		t.Fatal("session used as ingest token", w.Code)
	}
	if w := call("POST", "/v1/apply", `{"manifest":"bad"}`, cfg.Origin, "", "", e.Data.CSRF, cookie); w.Code != 400 {
		t.Fatal("authenticated API unreachable", w.Code)
	}
	if w := call("GET", "/v1/info", "", "", "", c.Ingest, "", nil); w.Code != 401 {
		t.Fatal("ingest accessed admin info")
	}
	if w := call("GET", "/v1/info", "", "", "", c.Admin, "", nil); w.Code != 200 {
		t.Fatal("bearer access broken")
	}
	if w := call("DELETE", "/v1/browser/session", "", cfg.Origin, "", "", e.Data.CSRF, cookie); w.Code != 200 {
		t.Fatal("logout", w.Code)
	}
	if w := call("GET", "/v1/info", "", "", "", "", "", cookie); w.Code != 401 {
		t.Fatal("revoked session survived")
	}
	late := mint()
	now = now.Add(61 * time.Second)
	if w := call("POST", "/v1/browser/session", `{"token":"`+late.Token+`"}`, cfg.Origin, "", "", "", nil); w.Code != 401 {
		t.Fatal("expired handoff accepted")
	}
	fresh := mint()
	w = call("POST", "/v1/browser/session", `{"token":"`+fresh.Token+`"}`, cfg.Origin, "", "", "", nil)
	cookie = w.Result().Cookies()[0]
	now = now.Add(9 * time.Hour)
	if w := call("GET", "/v1/info", "", "", "", "", "", cookie); w.Code != 401 {
		t.Fatal("expired session accepted")
	}
	for i := 0; i < 32; i++ {
		mint()
	}
	if w := call("POST", "/v1/browser/handoff", "{}", "", "", c.Admin, "", nil); w.Code != 429 {
		t.Fatal("unbounded handoffs", w.Code)
	}
}

func TestUIOrigins(t *testing.T) {
	for _, v := range []string{"http://127.0.0.1:7676", "http://[::1]:7676", "http://localhost:7676", "https://ding.example"} {
		if !ValidUIOrigin(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"http://0.0.0.0:7676", "http://192.168.1.1", "https://a/path", "https://a/", "https://user:pass@a", "https://a?token=x", "https://a#x", "//a"} {
		if ValidUIOrigin(v) {
			t.Fatal(v)
		}
	}
}
