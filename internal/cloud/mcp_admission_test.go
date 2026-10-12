package cloud

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/time/rate"
)

func TestMCPAndBrowserShareWorkspaceAdmission(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler, s.MCP = s.serveTenant, http.NotFoundHandler()
	consent := `{"clientID":"client","grant":{"name":"test","days":1,"scopes":["inspect"]}}`
	if w := cloudRequest(h, "POST", "/v1/cloud/models", consent, sessions[0], true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	ctx := context.Background()
	token := &auth.TokenInfo{Scopes: []string{"ding:inspect"}, Extra: map[string]any{"issuer": "issuer", "subject": "a", "client_id": "client"}}
	client, err := s.resolveMCP("issuer")(ctx, "inspect", token)
	if err != nil {
		t.Fatal(err)
	}
	s.limits[sessions[0].Workspace] = rate.NewLimiter(0, 2)
	if _, err := client.Call(ctx, "GET", "/capabilities", nil, nil); err != nil {
		t.Fatal(err)
	}
	if w := cloudRequest(h, "GET", "/v1/info", "", sessions[0], false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := client.Call(ctx, "GET", "/capabilities", nil, nil); err == nil || !strings.Contains(err.Error(), "workspace_rate") {
		t.Fatal("MCP escaped shared admission", err)
	}
	if w := cloudRequest(h, "GET", "/v1/info", "", sessions[0], false); w.Code != 429 {
		t.Fatal("browser escaped shared admission", w.Code)
	}
	if w := cloudRequest(h, "GET", "/v1/info", "", sessions[1], false); w.Code != 200 {
		t.Fatal("another workspace was throttled", w.Code)
	}
}
