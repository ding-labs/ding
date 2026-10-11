package cloud

import (
	"context"
	"github.com/ding-labs/ding/internal/access"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceCredentialIsScopedRevocableAndSeparateFromBrowser(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.Shared = true
	s.TenantHandler = s.serveTenant
	p, err := s.DB.Principal(context.Background(), sessions[0].User, sessions[0].Workspace, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.DB.CreateService(context.Background(), p, "producer", []string{"ingest"}, []string{"allowed"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string) int {
		r := httptest.NewRequest(method, "https://ding.example"+path, strings.NewReader("{}"))
		r.Header.Set("Authorization", "Bearer "+grant.Token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, path := range []string{"/v1/ingest/other", "/v1/apply", "/v1/browser/session", "/v1/cloud/secrets"} {
		if code := request("POST", path); code != 403 {
			t.Fatalf("%s: %d", path, code)
		}
	}
	if code := request("POST", "/v1/ingest/allowed"); code != 404 {
		t.Fatalf("authorized missing watch: %d", code)
	}
	if err := s.DB.RevokeService(context.Background(), p, grant.ID); err != nil {
		t.Fatal(err)
	}
	if code := request("POST", "/v1/ingest/allowed"); code != 401 {
		t.Fatalf("revoked: %d", code)
	}
	viewer := p
	viewer.Role = access.Viewer
	viewer.User = sessions[1].User
	if _, err := s.DB.CreateService(context.Background(), viewer, "forbidden", []string{"inspect"}, nil, 1); err == nil {
		t.Fatal("nonmember created grant")
	}
}
