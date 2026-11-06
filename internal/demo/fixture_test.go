package demo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthFixtureModesAndTimeoutCancellation(t *testing.T) {
	s := httptest.NewServer(HealthFixture())
	defer s.Close()
	set := func(mode string, want int) {
		t.Helper()
		r, _ := http.NewRequest(http.MethodPut, s.URL+"/mode", strings.NewReader(mode))
		response, err := s.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("set %q: %d", mode, response.StatusCode)
		}
	}
	check := func(want int) {
		t.Helper()
		response, err := s.Client().Get(s.URL + "/health")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("health: %d, want %d", response.StatusCode, want)
		}
	}
	check(200)
	set("failing", 204)
	check(503)
	set("not-a-mode", 400)
	set(strings.Repeat("x", 33), 400)
	check(503)
	set("timeout", 204)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL+"/health", nil)
	if response, err := s.Client().Do(r); !errors.Is(err, context.DeadlineExceeded) {
		if response != nil {
			response.Body.Close()
		}
		t.Fatalf("timeout mode returned %v", err)
	}
	set("healthy", 204)
	check(200)
}
