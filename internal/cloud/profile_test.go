package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
)

func TestBrowserProfileEditsAreSelfOnlyAndPersistAcrossSignIn(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	ctx := context.Background()
	owner, _ := s.DB.Principal(ctx, sessions[0].User, sessions[0].Workspace, "browser", nil)
	if err := s.DB.ChangeMember(ctx, owner, sessions[1].User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := s.DB.NewSession(ctx, sessions[1].User, owner.Workspace, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// A viewer can edit their own profile, independently of workspace privileges.
	w := cloudRequest(h, "PATCH", "/v1/user/profile", `{"name":"  Zuchka Studios  "}`, viewer, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Zuchka Studios"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := s.DB.InitializeDisplayName(ctx, viewer.User, "Provider name"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/user/profile", "/v1/browser/session"} {
		w := cloudRequest(h, "GET", path, "", viewer, false)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Zuchka Studios"`) {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	other, _ := s.DB.User(ctx, owner.User)
	if other.Name != "" {
		t.Fatal("another account was modified", other)
	}
	for _, input := range []map[string]any{
		{"name": ""}, {"name": "  "}, {"name": strings.Repeat("名", 101)}, {"name": "Bad\nName"},
		{"name": "Changed", "user": owner.User}, {"name": "Changed", "role": "owner"},
	} {
		body, _ := json.Marshal(input)
		if w := cloudRequest(h, "PATCH", "/v1/user/profile", string(body), viewer, true); w.Code != 400 {
			t.Fatal(string(body), w.Code, w.Body.String())
		}
	}
	if w := cloudRequest(h, "PATCH", "/v1/user/profile", `{"name":"Forged"}`, viewer, false); w.Code != 401 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	device, err := s.DB.NewSession(ctx, viewer.User, viewer.Workspace, "device", []string{"inspect", "manage"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "PATCH"} {
		r := httptest.NewRequest(method, "https://ding.example/v1/user/profile", strings.NewReader(`{"name":"Forged"}`))
		r.Header.Set("Authorization", "Bearer "+device.Token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatal("device accessed browser profile", method, w.Code)
		}
	}
}
