package cloud

import (
	"context"
	"fmt"
	"github.com/ding-labs/ding/internal/cloud/state"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDeviceBrowserApprovalRequiresVisibleConsentAndCSRF(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	proof := state.ID()
	id, _ := s.DB.BeginDevice(context.Background(), state.TokenHash(proof), time.Now())
	w := cloudRequest(h, "GET", "/connect/"+id, "", state.Session{}, false)
	if w.Code != 302 || !strings.HasPrefix(w.Header().Get("Location"), "/auth/login") {
		t.Fatal(w.Code)
	}
	w = cloudRequest(h, "GET", "/connect/"+id, "", sessions[0], false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), id[:8]) {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, ready, _ := s.DB.ClaimDevice(context.Background(), id, proof, time.Now()); ready {
		t.Fatal("GET approved device")
	}
	r := httptest.NewRequest("POST", "https://ding.example/connect/"+id, strings.NewReader(url.Values{"csrf": {sessions[0].CSRF}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessions[0].Token})
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: sessions[0].CSRF})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = cloudRequest(h, "POST", "/v1/cloud/device/claim", fmt.Sprintf(`{"id":%q,"verifier":%q}`, id, proof), state.Session{}, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ready":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
