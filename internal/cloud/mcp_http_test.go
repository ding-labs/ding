package cloud

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type hostedMCPFixture struct {
	s       *Server
	h       http.Handler
	session state.Session
	token   func(subject, client, scope, audience string) string
}

func newHostedMCPFixture(t *testing.T) hostedMCPFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private, err := jwk.Import(key)
	if err != nil {
		t.Fatal(err)
	}
	private.Set(jwk.KeyIDKey, "test")
	public, err := jwk.PublicKeyOf(private)
	if err != nil {
		t.Fatal(err)
	}
	keys := jwk.NewSet()
	keys.AddKey(public)
	issuer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(keys) }))
	t.Cleanup(issuer.Close)
	s, _, _ := testCloudServer(t)
	// Real tenant workers and delivery receipts, with all outbound I/O isolated.
	s.Pool.http = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("test")), Request: r}, nil
	})}
	s.TenantHandler = s.serveTenant
	cfg := mcpconfig.HTTP{PublicURL: s.PublicURL, Issuer: issuer.URL, JWKSURI: issuer.URL + "/keys", Audience: s.PublicURL + "/mcp", Algorithm: "RS256"}
	var closeMCP func()
	s.MCP, closeMCP, err = s.MCPHandler(cfg, issuer.Client())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeMCP)
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.DB.Enroll(context.Background(), issuer.URL, "owner", 10)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.DB.NewSession(context.Background(), u.User, u.ID, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Vault.Put(context.Background(), u.ID, "WEBHOOK", "https://hooks.example/test"); err != nil {
		t.Fatal(err)
	}
	w := cloudRequest(h, "POST", "/v1/cloud/models", `{"clientID":"client","grant":{"name":"HTTP test","days":1,"scopes":["inspect","preview","manage","retry"],"secretRefs":["WEBHOOK"]}}`, session, true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	sign := func(subject, client, scope, audience string) string {
		value := jwt.New()
		if audience == "" {
			audience = cfg.Audience
		}
		for k, v := range map[string]any{"iss": issuer.URL, "sub": subject, "client_id": client, "scope": scope, "aud": audience, "exp": time.Now().Add(time.Hour)} {
			value.Set(k, v)
		}
		raw, err := jwt.Sign(value, jwt.WithKey(jwa.RS256(), private))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	return hostedMCPFixture{s, h, session, sign}
}

func (f hostedMCPFixture) request(t *testing.T, token, version, method string, params map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	if params == nil {
		params = map[string]any{}
	}
	if version == "2026-07-28" {
		params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": version, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	}
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", f.s.PublicURL+"/mcp", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", version)
	r.Header.Set("MCP-Method", method)
	if name, ok := params["name"].(string); ok {
		r.Header.Set("MCP-Name", name)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}

func (f hostedMCPFixture) call(t *testing.T, token, name string, args map[string]any) mcp.CallToolResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	w := f.request(t, token, "2026-07-28", "tools/call", map[string]any{"name": name, "arguments": args})
	var envelope struct {
		Result mcp.CallToolResult `json:"result"`
		Error  any                `json:"error"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Error != nil {
		t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
	}
	return envelope.Result
}

func hostedView(t *testing.T, result mcp.CallToolResult) mcpcontract.View {
	t.Helper()
	if result.IsError {
		t.Fatal(result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var view mcpcontract.View
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestHostedHTTPMetadataIdentityAndProtocols(t *testing.T) {
	f := newHostedMCPFixture(t)
	r := httptest.NewRequest("GET", f.s.PublicURL+"/.well-known/oauth-protected-resource/mcp", nil)
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), f.s.PublicURL+"/mcp") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, version := range []string{"2025-11-25", "2026-07-28"} {
		token := f.token("owner", "client", "ding:inspect", "")
		w := f.request(t, token, version, "tools/call", map[string]any{"name": "ding_get_capabilities", "arguments": map[string]any{}})
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"execution":"cloud"`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, token := range []string{"invalid", f.token("owner", "client", "ding:inspect", "https://other.example/mcp")} {
		w := f.request(t, token, "2026-07-28", "tools/list", nil)
		if w.Code != 401 {
			t.Fatal("bad bearer accepted", w.Code, w.Body.String())
		}
	}
	if w := f.request(t, f.token("owner", "client", "ding:manage", ""), "2026-07-28", "tools/list", nil); w.Code != 403 {
		t.Fatal("missing inspect scope accepted", w.Code, w.Body.String())
	}
	for _, token := range []string{f.token("other", "client", "ding:inspect", ""), f.token("owner", "other-client", "ding:inspect", "")} {
		if result := f.call(t, token, "ding_list_workspaces", nil); !result.IsError && strings.Contains(string(mustJSON(t, result.StructuredContent)), f.session.Workspace) {
			t.Fatal("identity crossed consent", result)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
