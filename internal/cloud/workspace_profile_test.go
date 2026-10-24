package cloud

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

func TestWorkspaceRenameKeepsIdentityAndRefreshesMCPMetadata(t *testing.T) {
	f := newHostedMCPFixture(t)
	ctx := context.Background()
	token := f.token("owner", "client", "ding:inspect", "")
	before := hostedView(t, f.call(t, token, "ding_get_capabilities", nil))
	w := cloudRequest(f.h, "PATCH", "/v1/workspace", `{"name":"  Studio signals  "}`, f.session, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	after := hostedView(t, f.call(t, token, "ding_get_capabilities", nil))
	data := after.Data.(map[string]any)
	if data["workspaceName"] != "Studio signals" || after.Target.Workspace != f.session.Workspace || before.Data.(map[string]any)["instance"] != data["instance"] {
		t.Fatal("renaming changed execution identity or left stale MCP metadata", after)
	}
	for _, path := range []string{"/v1/browser/session", "/v1/user/profile"} {
		if w := cloudRequest(f.h, "GET", path, "", f.session, false); w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
	if w := cloudRequest(f.h, "GET", "/v1/browser/session", "", f.session, false); !strings.Contains(w.Body.String(), `"name":"Studio signals"`) {
		t.Fatal(w.Body.String())
	}
	other, err := f.s.DB.Enroll(ctx, f.s.AuthKitIssuer, "viewer", 10)
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := f.s.DB.Principal(ctx, f.session.User, f.session.Workspace, "browser", nil)
	for _, role := range []access.Role{access.Viewer, access.Editor, access.Administrator} {
		if err := f.s.DB.ChangeMember(ctx, owner, other.User, role); err != nil {
			t.Fatal(err)
		}
		session, err := f.s.DB.NewSession(ctx, other.User, f.session.Workspace, "browser", nil, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if w := cloudRequest(f.h, "PATCH", "/v1/workspace", `{"name":"Forged"}`, session, true); w.Code != 403 {
			t.Fatal("non-owner renamed workspace", role, w.Code)
		}
	}
	device, err := f.s.DB.NewSession(ctx, owner.User, owner.Workspace, "device", []string{"owners"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("PATCH", "https://ding.example/v1/workspace", strings.NewReader(`{"name":"Forged"}`))
	r.Header.Set("Authorization", "Bearer "+device.Token)
	deviceResponse := httptest.NewRecorder()
	f.h.ServeHTTP(deviceResponse, r)
	if deviceResponse.Code != 403 {
		t.Fatal("device renamed workspace", deviceResponse.Code)
	}
	for _, body := range []string{`{"name":""}`, `{"name":"Bad\nName"}`, `{"name":"New","workspace":"other"}`} {
		if w := cloudRequest(f.h, "PATCH", "/v1/workspace", body, f.session, true); w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	if w := cloudRequest(f.h, "PATCH", "/v1/workspace", `{"name":"Forged"}`, f.session, false); w.Code != 401 {
		t.Fatal("CSRF missing", w.Code)
	}
}
