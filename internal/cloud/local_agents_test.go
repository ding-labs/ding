package cloud

import (
	"context"
	"encoding/json"
	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharedAgentPairingRetainsGrantAndMembershipChecks(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.Shared = true
	s.TenantHandler = s.serveTenant
	w := cloudRequest(h, "POST", "/v1/cloud/models", `{"local":true,"grant":{"name":"Agent","days":1,"scopes":["inspect","preview","manage"]}}`, sessions[0], true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		Data struct {
			Connection mcpconfig.Connection `json:"connection"`
			ClientID   string               `json:"clientID"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if err := out.Data.Connection.Validate(); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string) int {
		r := httptest.NewRequest(method, "https://ding.example"+path, strings.NewReader("{}"))
		r.Header.Set("Authorization", "Bearer "+out.Data.Connection.Token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if code := request("GET", "/v1/integrations/capabilities"); code != 200 {
		t.Fatal(code)
	}
	if code := request("GET", "/v1/cloud/secrets"); code != 403 {
		t.Fatal("agent escaped API", code)
	}
	ctx := context.Background()
	owner, _ := s.DB.Principal(ctx, sessions[0].User, sessions[0].Workspace, "browser", nil)
	if err := s.DB.ChangeMember(ctx, owner, sessions[1].User, access.Owner); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.ChangeMember(ctx, owner, sessions[0].User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	if code := request("POST", "/v1/integrations/preview"); code != 403 {
		t.Fatal("agent escaped downgrade", code)
	}
	if err := s.DB.UnbindMCP(ctx, sessions[0].User, sessions[0].Workspace, out.Data.ClientID); err != nil {
		t.Fatal(err)
	}
	if code := request("GET", "/v1/integrations/capabilities"); code != 403 {
		t.Fatal("revocation failed", code)
	}
	s.Shared = false
	if code := request("GET", "/v1/integrations/capabilities"); code != 401 {
		t.Fatal("cloud accepted local grant", code)
	}
}
