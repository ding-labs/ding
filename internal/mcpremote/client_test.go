package mcpremote

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"golang.org/x/oauth2"
)

func TestRefreshPersistsRotatedCredentialsBeforeUse(t *testing.T) {
	p := testProfile()
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := mcpconfig.AtomicJSON(path, p); err != nil {
		t.Fatal(err)
	}
	next := *p.Token
	next.AccessToken = "rotated"
	next.RefreshToken = "rotated-refresh"
	next.Expiry = time.Now().Add(2 * time.Hour)
	s := &savingSource{source: oauth2.StaticTokenSource(&next), profile: p, path: path}
	if _, err := s.Token(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Token.RefreshToken != next.RefreshToken {
		t.Fatal(err)
	}
	// A failed durable write must not become a successful in-memory refresh.
	s.path = filepath.Join(t.TempDir(), "missing", "profile.json")
	s.profile = p
	unlock, err := mcpconfig.Lock(s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	// The destination is a private regular file but invalid JSON is irrelevant:
	// force a directory collision at the atomic rename destination instead.
	s.path = filepath.Dir(s.path)
	for range 2 {
		if _, err := s.Token(); err == nil {
			t.Fatal("used token after persistence failure")
		}
	}
}

type tripFunc func(*http.Request) (*http.Response, error)

func (f tripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResourceBearerCannotFollowAnotherEndpoint(t *testing.T) {
	called := false
	rt := &resourceTransport{endpoint: "https://cloud.example/mcp", base: tripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, nil })}
	for _, endpoint := range []string{"https://other.example/mcp", "https://cloud.example/v1/admin", "http://cloud.example/mcp"} {
		r, _ := http.NewRequest("POST", endpoint, nil)
		if _, err := rt.RoundTrip(r); err == nil || called {
			t.Fatal("forwarded outside selected resource")
		}
	}
}
