package mcpsetup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSetupHostOriginNonceExpiryAndDuplicatePairing(t *testing.T) {
	s, err := NewSession(t.TempDir(), filepath.Join(t.TempDir(), "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var calls atomic.Int32
	s.pair = func(context.Context, PairOptions, io.Writer) error {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	request := func(method, path, host, origin, nonce, body string) int {
		r := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
		r.Host = host
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Ding-Setup", nonce)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), s.nonce) {
			t.Error("nonce leaked")
		}
		return w.Code
	}
	host := s.Listener.Addr().String()
	if request("GET", "/", host, "", "", "") != 200 {
		t.Fatal("setup page")
	}
	for _, row := range [][3]string{{"evil.example", s.origin, s.nonce}, {host, "https://evil.example", s.nonce}, {host, s.origin, ""}} {
		if request("POST", "/pair", row[0], row[1], row[2], "{}") != 403 {
			t.Fatal("setup authorization bypass")
		}
	}
	if request("POST", "/pair", host, s.origin, s.nonce, `{"manage":"true"}`) != 400 {
		t.Fatal("invalid permissions accepted")
	}
	if request("POST", "/pair", host, s.origin, s.nonce, `{"state":"relative"}`) != 400 {
		t.Fatal("relative state accepted")
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); request("POST", "/pair", host, s.origin, s.nonce, "{}") }()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate grants", calls.Load())
	}
	s.deadline = time.Now().Add(-time.Second)
	if request("GET", "/status", host, s.origin, s.nonce, "") != http.StatusForbidden {
		t.Fatal("expired setup accepted")
	}
}
