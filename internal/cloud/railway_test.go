package cloud

import (
	"net/http/httptest"
	"testing"
)

func TestRailwayProbeHostCannotReachAuthenticatedOrLoginRoutes(t *testing.T) {
	s, h, _ := testCloudServer(t)
	s.Railway = true
	for _, tc := range []struct {
		host, path, proto string
		want              int
	}{
		{"healthcheck.railway.app", "/healthz", "", 200},
		{"healthcheck.railway.app", "/readyz", "", 200},
		{"healthcheck.railway.app", "/ui/", "https", 403},
		{"healthcheck.railway.app", "/auth/login", "https", 403},
		{"healthcheck.railway.app", "/v1/watches", "https", 403},
		{"other.railway.app", "/healthz", "https", 403},
		{"ding.example", "/v1/auth/config", "", 403},
		{"ding.example", "/v1/auth/config", "http", 403},
		{"ding.example", "/v1/auth/config", "https", 200},
	} {
		r := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
		r.Header.Set("X-Forwarded-Proto", tc.proto)
		r.Header.Set("X-Ding-Role", "owner")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
	for s.global.Allow() {
	} // user traffic must not make deployment readiness fail
	r := httptest.NewRequest("GET", "http://healthcheck.railway.app/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	s.Railway = false
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("probe alias escaped deployment profile")
	}
}
