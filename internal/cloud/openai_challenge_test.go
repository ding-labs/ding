package cloud

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIChallengePublishesExactTokenAtCanonicalOrigin(t *testing.T) {
	s, h, _ := testCloudServer(t)
	const path = "/.well-known/openai-apps-challenge"
	request := func(method, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://"+host+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "ding.example"); w.Code != 404 {
		t.Fatal("unconfigured challenge must be absent", w.Code)
	}
	s.OpenAIAppsChallenge = "portal-token.unchanged_123"
	w := request("GET", "ding.example")
	if w.Code != 200 || w.Body.String() != s.OpenAIAppsChallenge || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatal("challenge was not returned exactly as plain text", w.Code, w.Body.String())
	}
	if w := request("HEAD", "ding.example"); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD must omit the token body")
	}
	if w := request("POST", "ding.example"); w.Code != http.StatusMethodNotAllowed {
		t.Fatal("challenge endpoint accepted a write")
	}
	if w := request("GET", "other.example"); w.Code != http.StatusForbidden {
		t.Fatal("challenge leaked onto a different host")
	}
	for _, invalid := range []string{"token\n", "two tokens", "é", strings.Repeat("a", 4097)} {
		s.OpenAIAppsChallenge = invalid
		if _, err := s.Handler(); err == nil {
			t.Fatal("invalid challenge accepted")
		}
	}
}
