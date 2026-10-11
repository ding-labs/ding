package cloud

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

func TestSharedRoleChecksSurviveInternalAdminForwarding(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	ctx := context.Background()
	owner, _ := s.DB.Principal(ctx, sessions[0].User, sessions[0].Workspace, "browser", nil)
	if err := s.DB.ChangeMember(ctx, owner, sessions[1].User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := s.DB.NewSession(ctx, sessions[1].User, owner.Workspace, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if w := cloudRequest(h, "GET", "/v1/watches", "", viewer, false); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/v1/apply", "/v1/cloud/secrets", "/v1/cloud/models", "/v1/tools/test"} {
		if w := cloudRequest(h, "POST", path, "{}", viewer, true); w.Code != 403 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if err := s.DB.ChangeMember(ctx, owner, viewer.User, access.Editor); err != nil {
		t.Fatal(err)
	}
	if w := cloudRequest(h, "POST", "/v1/apply", "{}", viewer, true); w.Code != 400 {
		t.Fatal("editor did not reach validation", w.Code, w.Body.String())
	}
	if err := s.DB.ChangeMember(ctx, owner, viewer.User, ""); err != nil {
		t.Fatal(err)
	}
	if w := cloudRequest(h, "GET", "/v1/watches", "", viewer, false); w.Code != 403 {
		t.Fatal("removed member retained access", w.Code)
	}
}

func TestDeviceWorkspaceAndScopesCannotBeOverridden(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	d, err := s.DB.NewSession(context.Background(), sessions[0].User, sessions[0].Workspace, "device", []string{"inspect"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path, workspace string
		want                    int
	}{
		{"GET", "/v1/watches", d.Workspace, 200},
		{"POST", "/v1/apply", d.Workspace, 403},
		{"GET", "/v1/watches", sessions[1].Workspace, 403},
	} {
		r := httptest.NewRequest(test.method, "https://ding.example"+test.path, nil)
		r.Header.Set("Authorization", "Bearer "+d.Token)
		r.Header.Set("X-Ding-Workspace", test.workspace)
		r.Header.Set("X-Ding-Role", "owner")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Errorf("%+v: %d %s", test, w.Code, w.Body.String())
		}
	}
}
