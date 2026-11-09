package cloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	state "github.com/ding-labs/ding/internal/serverstate"
)

func TestHomepageFunnelAuthenticatedJourney(t *testing.T) {
	server, handler, sessions := testCloudServer(t)
	const handoff = "ad81740b-59e6-4223-8d12-a45991c023eb"
	const key = "dedicated-test-key"
	verified := 0
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts, sig, ok := strings.Cut(strings.TrimPrefix(r.Header.Get("Ding-Signature"), "t="), ", v1=")
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write(append([]byte(ts+"."), body...))
		if !ok || sig != hex.EncodeToString(mac.Sum(nil)) {
			t.Error("bad receipt signature")
		}
		var data map[string]string
		if err := json.Unmarshal(body, &data); err != nil {
			t.Error(err)
			return
		}
		switch r.URL.Path {
		case "/_funnel/verified":
			if data["handoff"] != handoff || data["account"] != state.TokenHash("immutable-user-id") {
				t.Errorf("incorrect verified handoff")
			}
			verified++
			w.WriteHeader(204)
		case "/_funnel/report":
			if data["account"] != state.TokenHash("a") {
				w.WriteHeader(403)
				return
			}
			io.WriteString(w, `{"homepage_browsers":51,"browsers_clicked_get_started":12,"signed_up_from_button":4,"all_outside_signups":6}`)
		default:
			t.Error("unexpected route")
		}
	}))
	defer worker.Close()
	server.AuthKitIssuer = "https://auth.example"
	if err := server.DB.ConfigureLoginMethods(t.Context(), []string{"password"}); err != nil {
		t.Fatal(err)
	}
	for subject, user := range map[string]string{"0-other-linked-login": sessions[0].User, "a": sessions[0].User, "b": sessions[1].User} {
		if err := server.DB.EnableOIDC(t.Context(), state.IdentityLink{User: user, Issuer: server.AuthKitIssuer, Subject: subject}); err != nil {
			t.Fatal(err)
		}
	}
	server.Login.homepage = newHomepageAnalytics(key)
	server.Login.homepage.origin = worker.URL
	anon := cloudRequest(handler, "GET", "/analytics/homepage", "", state.Session{}, false)
	if anon.Code != 302 || anon.Header().Get("Location") != "/auth/login?return=/analytics/homepage" {
		t.Fatal("report sign-in")
	}
	private := cloudRequest(handler, "GET", "/analytics/homepage", "", sessions[0], false)
	if private.Code != 200 || !strings.Contains(private.Body.String(), ">51<") {
		t.Fatal("owner could not read private report", private.Code)
	}
	if other := cloudRequest(handler, "GET", "/analytics/homepage", "", sessions[1], false); other.Code != 403 {
		t.Fatal("external account could read report")
	}
	start := cloudRequest(handler, "GET", "/auth/login?homepage_handoff="+handoff, "", state.Session{}, false)
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRequest("GET", "https://ding.example/auth/callback?code=verified-code&state="+location.Query().Get("state"), nil)
	for _, cookie := range start.Result().Cookies() {
		callback.AddCookie(cookie)
	}
	done := httptest.NewRecorder()
	handler.ServeHTTP(done, callback)
	if done.Code != 302 || verified != 1 {
		t.Fatal("verified signup handoff missing", done.Code, verified)
	}
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, callback)
	if replay.Code != 400 || verified != 1 {
		t.Fatal("callback replay counted")
	}
}
