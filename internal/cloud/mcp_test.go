package cloud

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"strings"
	"testing"
)

func TestHostedMCPUsesSeparateWorkspaceClientScopesAndRevocation(t *testing.T) {
	s, h, sessions := testCloudServer(t)
	s.TenantHandler = s.serveTenant
	if err := s.Vault.Put(context.Background(), sessions[0].Account, "WEBHOOK", "https://hooks.example/test"); err != nil {
		t.Fatal(err)
	}
	consent := `{"clientID":"chatgpt-client","grant":{"name":"ChatGPT","days":7,"scopes":["inspect","preview","manage"],"secretRefs":["WEBHOOK"]}}`
	w := cloudRequest(h, "POST", "/v1/cloud/models", consent, sessions[0], true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "ding_mcp_") {
		t.Fatal("internal token escaped")
	}
	token := func(subject, client string, scopes ...string) *auth.TokenInfo {
		return &auth.TokenInfo{Scopes: scopes, Extra: map[string]any{"issuer": "issuer", "subject": subject, "client_id": client}}
	}
	resolve := s.resolveMCP("issuer")
	ctx := context.Background()
	for _, identity := range []*auth.TokenInfo{nil, token("b", "chatgpt-client", "ding:inspect"), token("a", "other-client", "ding:inspect"), token("a", "chatgpt-client", "ding:preview")} {
		if _, err := resolve(ctx, "inspect", identity); err == nil {
			t.Fatal("unapproved identity/scopes accepted")
		}
	}
	identity := token("a", "chatgpt-client", "ding:inspect", "ding:preview", "ding:manage")
	client, err := resolve(ctx, "inspect", identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(ctx, "GET", "/capabilities", nil, nil); err != nil {
		t.Fatal(err)
	}
	client, err = resolve(ctx, "preview", identity)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := Template(FirstWatch{ID: "private", URL: "https://example.com", Destination: "webhook", Credential: "WEBHOOK"})
	if err != nil {
		t.Fatal(err)
	}
	bad = strings.ReplaceAll(bad, "https://example.com", "http://127.0.0.1")
	if _, err := client.Call(ctx, "POST", "/preview", nil, map[string]any{"manifest": bad}); err == nil {
		t.Fatal("MCP bypassed cloud policy")
	}
	w = cloudRequest(h, "POST", "/v1/cloud/models", `{"clientID":"chatgpt-client","revoke":true}`, sessions[0], true)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := resolve(ctx, "inspect", identity); err == nil {
		t.Fatal("revoked model automatically recreated")
	}
	if _, err := client.Call(ctx, "GET", "/capabilities", nil, nil); err == nil {
		t.Fatal("retained internal connection survived revocation")
	}
	w = cloudRequest(h, "GET", "/v1/cloud/models", "", sessions[1], false)
	if w.Code != 200 || strings.Contains(w.Body.String(), "ChatGPT") {
		t.Fatal("cross-workspace model list", w.Body.String())
	}
}
