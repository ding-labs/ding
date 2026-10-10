package mcpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
)

func TestReadCancellationReachesDaemon(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) }))
	defer s.Close()
	c := New(connection(s.URL))
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Call(ctx, "GET", "/capabilities", nil, nil); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled request succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client ignored cancellation")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream ignored cancellation")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func connection(url string) mcpconfig.Connection {
	return mcpconfig.Connection{DaemonURL: url, Token: "ding_mcp_" + strings.Repeat("a", 64), GrantID: strings.Repeat("b", 64)}
}

func TestRedirectAndRouteBoundaries(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "https://attacker.example")
		w.WriteHeader(307)
	}))
	defer s.Close()
	c := New(connection(s.URL))
	defer c.Close()
	if _, err := c.Call(context.Background(), "GET", "/watches", nil, nil); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
	for _, path := range []string{"/../backup", "/grants", "/watches/api%2Fexport", "/watches/%2e%2e", "/watches/..", "/watches/a\\b", "/watches/a?x=1"} {
		if _, err := c.Call(context.Background(), "GET", path, nil, nil); err == nil {
			t.Fatal("route escaped", path)
		}
	}
	if calls != 1 {
		t.Fatal("invalid requests reached daemon")
	}
	for _, id := range []string{"", "..", ".", "a/b", "a\\b", "\x00"} {
		if _, err := Segment(id); err == nil {
			t.Fatal(id)
		}
	}
}

func TestAmbiguousWritesAndResponseBounds(t *testing.T) {
	for _, mode := range []string{"network", "truncated", "invalid", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := New(connection("http://127.0.0.1:7676"))
			defer c.Close()
			c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				if mode == "network" {
					return nil, errors.New("sensitive private upstream failure")
				}
				body := "{"
				if mode == "oversized" {
					body = strings.Repeat("x", MaxResponse+1)
				}
				var reader io.ReadCloser = io.NopCloser(strings.NewReader(body))
				if mode == "truncated" {
					reader = io.NopCloser(brokenReader{})
				}
				return &http.Response{StatusCode: 200, Body: reader}, nil
			})
			_, err := c.Call(context.Background(), "POST", "/apply", nil, map[string]any{"operationKey": "same_key_000000001"})
			if err == nil || !strings.Contains(err.Error(), "outcome_unknown") || strings.Contains(err.Error(), "sensitive") || calls != 1 {
				t.Fatal(err, calls)
			}
		})
	}
	c := New(connection("http://127.0.0.1"))
	defer c.Close()
	c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"apiVersion":"ding.ing/v1alpha1","error":{"code":"integration_denied","message":"denied"}}`))}, nil
	})
	_, err := c.Call(context.Background(), "POST", "/apply", nil, map[string]any{})
	if err == nil || !strings.HasPrefix(err.Error(), "integration_denied:") {
		t.Fatal("definite errors became ambiguous", err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
