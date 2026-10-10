package mcpserver_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func TestHTTPIdentityIsolationScopesProtocolsAndGuards(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private, _ := jwk.Import(key)
	_ = private.Set(jwk.KeyIDKey, "test")
	public, _ := jwk.PublicKeyOf(private)
	keys := jwk.NewSet()
	_ = keys.AddKey(public)
	issuer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(keys) }))
	defer issuer.Close()
	var daemonCalls atomic.Int32
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		daemonCalls.Add(1)
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ding_mcp_")
		who := "alice"
		if token == strings.Repeat("b", 64) {
			who = "bob"
		}
		if token == strings.Repeat("c", 64) {
			who = "rotated"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "ding.ing/v1alpha1", "data": map[string]any{"watches": []any{}, "total": 0, "more": false, "cursor": "", "subject": who}})
	}))
	defer daemon.Close()
	subjects := map[string]string{}
	for subject, suffix := range map[string]string{"alice": "a", "bob": "b"} {
		path := filepath.Join(t.TempDir(), "mcp.json")
		f, err := mcpconfig.CreatePrivate(path)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(f).Encode(mcpconfig.Connection{DaemonURL: daemon.URL, Token: "ding_mcp_" + strings.Repeat(suffix, 64), GrantID: strings.Repeat(suffix, 64)})
		f.Close()
		subjects[subject] = path
	}
	cfg := mcpconfig.HTTP{PublicURL: "https://ding.example", Issuer: issuer.URL, JWKSURI: issuer.URL + "/jwks", Audience: "https://ding.example/mcp", Subjects: subjects, Algorithm: "RS256"}
	h, close, err := mcpserver.HTTPHandler(mcpserver.HTTPOptions{Config: cfg, Version: "test", HTML: "<title>Ding</title>", JWKSClient: issuer.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	token := func(subject, scope string) string {
		value := jwt.New()
		for k, v := range map[string]any{"iss": issuer.URL, "aud": cfg.Audience, "sub": subject, "exp": time.Now().Add(time.Hour), "scope": scope} {
			_ = value.Set(k, v)
		}
		b, err := jwt.Sign(value, jwt.WithKey(jwa.RS256(), private))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	request := func(bearer, name, version, origin, host string, args any) *httptest.ResponseRecorder {
		params := map[string]any{"name": name, "arguments": args}
		if version == "2026-07-28" {
			params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": version, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "test", "method": "tools/call", "params": params})
		r := httptest.NewRequest("POST", cfg.PublicURL+"/mcp", bytes.NewReader(raw))
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", version)
		r.Header.Set("MCP-Method", "tools/call")
		r.Header.Set("MCP-Name", name)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, version := range []string{"2026-07-28", "2025-11-25"} {
		var wg sync.WaitGroup
		for _, subject := range []string{"alice", "bob"} {
			for range 5 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					w := request(token(subject, "ding:inspect"), "ding_list_watches", version, "", "ding.example", map[string]any{})
					if w.Code != 200 || !strings.Contains(w.Body.String(), `"subject":"`+subject+`"`) {
						t.Errorf("identity/negotiation: %d %s", w.Code, w.Body.String())
					}
				}()
			}
		}
		wg.Wait()
	}
	before := daemonCalls.Load()
	for _, row := range []struct {
		bearer, origin, host string
		code                 int
	}{{"", "", "ding.example", 401}, {"bad", "", "ding.example", 401}, {token("alice", "ding:manage"), "", "ding.example", 403}, {token("alice", "ding:inspect"), "https://evil.example", "ding.example", 403}, {token("alice", "ding:inspect"), "", "evil.example", 403}} {
		w := request(row.bearer, "ding_list_watches", "2026-07-28", row.origin, row.host, map[string]any{})
		if w.Code != row.code {
			t.Fatal("HTTP auth", w.Code, w.Body.String())
		}
	}
	for _, row := range []struct {
		subject, name string
		args          any
	}{{"unpaired", "ding_list_watches", map[string]any{}}, {"alice", "ding_apply_changes", map[string]any{"handle": strings.Repeat("a", 64), "operation_key": "http_write_key_000001"}}, {"alice", "ding_list_watches", nil}} {
		w := request(token(row.subject, "ding:inspect"), row.name, "2026-07-28", "", "ding.example", row.args)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"isError":true`) {
			t.Fatal("tool denial", w.Code, w.Body.String())
		}
	}
	if daemonCalls.Load() != before {
		t.Fatal("denied request reached daemon")
	}
	r := httptest.NewRequest("GET", cfg.PublicURL+"/.well-known/oauth-protected-resource/mcp", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"resource":"https://ding.example/mcp"`) {
		t.Fatal("metadata", w.Body.String())
	}
	w = request("", "ding_list_watches", "2026-07-28", "", "ding.example", map[string]any{})
	if !strings.Contains(w.Header().Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatal("missing challenge")
	}
	rotated := mcpconfig.Connection{DaemonURL: daemon.URL, Token: "ding_mcp_" + strings.Repeat("c", 64), GrantID: strings.Repeat("c", 64)}
	raw, _ := json.Marshal(rotated)
	if err := os.WriteFile(subjects["alice"], raw, 0600); err != nil {
		t.Fatal(err)
	}
	w = request(token("alice", "ding:inspect"), "ding_list_watches", "2026-07-28", "", "ding.example", map[string]any{})
	if !strings.Contains(w.Body.String(), `"subject":"rotated"`) {
		t.Fatal("credential file was cached", w.Body.String())
	}
}
