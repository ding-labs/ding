package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/integrationpolicy"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestCloudPreviewLimitsAgreeAcrossMCPAndREST(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	s.MCP = http.NotFoundHandler()
	ctx := context.Background()
	if err := s.Vault.Put(ctx, sessions[0].Workspace, "WEBHOOK", "https://hooks.example/test"); err != nil {
		t.Fatal(err)
	}
	consent := `{"clientID":"client","grant":{"name":"test","days":1,"scopes":["inspect","preview","manage"],"secretRefs":["WEBHOOK"]}}`
	if w := cloudRequest(h, "POST", "/v1/cloud/models", consent, sessions[0], true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	token := &auth.TokenInfo{Scopes: []string{"ding:inspect", "ding:preview", "ding:manage"}, Extra: map[string]any{"issuer": "issuer", "subject": "a", "client_id": "client"}}
	client, err := s.resolveMCP("issuer")(ctx, "preview", token)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Template(FirstWatch{ID: "limits", URL: "https://example.com", Destination: "webhook", Credential: "WEBHOOK"})
	if err != nil {
		t.Fatal(err)
	}
	// JSON encodes each '<' as six bytes. This remains a valid YAML comment.
	manifest += "\n#" + strings.Repeat("<", integrationpolicy.CloudManifestBytes-len(manifest)-2)
	body := map[string]any{"manifest": manifest}
	if _, err := client.Call(ctx, "POST", "/preview", nil, body); err != nil {
		t.Fatal("MCP rejected manifest at limit", err)
	}
	raw, _ := json.Marshal(body)
	if w := cloudRequest(h, "POST", "/v1/tools/compile", string(raw), sessions[0], true); w.Code != 200 || !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Fatal("REST rejected escaped manifest", w.Code, w.Body.String())
	}
	body["manifest"] = manifest + "x"
	if _, err := client.Call(ctx, "POST", "/preview", nil, body); err == nil || !strings.Contains(err.Error(), "invalid_request") {
		t.Fatal("MCP accepted oversized manifest", err)
	}
	raw, _ = json.Marshal(body)
	if w := cloudRequest(h, "POST", "/v1/tools/compile", string(raw), sessions[0], true); w.Code != 413 {
		t.Fatal("REST accepted oversized manifest", w.Code)
	}
	body["manifest"], body["fixture"] = "", strings.Repeat("x", integrationpolicy.CloudFixtureBytes+1)
	if _, err := client.Call(ctx, "POST", "/preview", nil, body); err == nil || !strings.Contains(err.Error(), "invalid_request") {
		t.Fatal("MCP parsed oversized fixture", err)
	}
	raw, _ = json.Marshal(body)
	if w := cloudRequest(h, "POST", "/v1/tools/test", string(raw), sessions[0], true); w.Code != 413 {
		t.Fatal("REST parsed oversized fixture", w.Code)
	}
}
