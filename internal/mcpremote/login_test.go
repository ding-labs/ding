package mcpremote

import (
	"github.com/modelcontextprotocol/go-sdk/auth"
	"net/http/httptest"
	"testing"
)

func TestLoopbackRejectsForeignStateAndReplay(t *testing.T) {
	c := &loopbackCallback{state: "expected", result: make(chan *auth.AuthorizationResult, 1)}
	for _, query := range []string{"code=secret&state=wrong", "error=denied&state=expected", "state=expected"} {
		w := httptest.NewRecorder()
		c.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/callback?"+query, nil))
		if w.Code != 400 || len(c.result) != 0 {
			t.Fatal("accepted invalid OAuth callback")
		}
	}
	for i := range 2 {
		w := httptest.NewRecorder()
		c.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/callback?code=secret&state=expected&iss=https%3A%2F%2Fauth.example", nil))
		if (i == 0 && w.Code != 200) || (i == 1 && w.Code != 400) {
			t.Fatal(w.Code)
		}
	}
	if got := <-c.result; got.State != "expected" || got.Iss != "https://auth.example" {
		t.Fatal("lost response binding")
	}
}
