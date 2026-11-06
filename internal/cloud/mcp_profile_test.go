package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpcontract"
	state "github.com/ding-labs/ding/internal/serverstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHostedProfileIdentityAcrossLabelsConsentAndLinkedSignIn(t *testing.T) {
	f := newHostedMCPFixture(t)
	f.s.AuthKitClients = []string{"chatgpt"}
	ctx := context.Background()
	token := func(subject, consent, email string) string {
		return f.token(subject, "chatgpt", "ding:inspect", "", map[string]any{"sid": consent, "email": email})
	}
	read := func(bearer string) mcpcontract.Profile {
		t.Helper()
		r := f.call(t, bearer, "ding_get_profile", nil)
		if r.IsError {
			t.Fatal(r)
		}
		raw := mustJSON(t, r.StructuredContent)
		var p mcpcontract.Profile
		if err := json.Unmarshal(raw, &p); err != nil || p.ID == "" || strings.Contains(string(raw), `"data"`) || strings.Contains(string(raw), `"email"`) {
			t.Fatalf("invalid or invented profile: %s %v", raw, err)
		}
		var text mcpcontract.Profile
		if len(r.Content) != 1 || json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &text) != nil || text != p {
			t.Fatal("text and structured profile differ", r)
		}
		return p
	}
	old := token("owner", "original", "old@example.com")
	if p := read(old); p.ID != f.session.User || p.Name != "" {
		t.Fatal("profile differs from immutable account", p)
	}
	if _, err := f.s.DB.UpdateDisplayName(ctx, f.session.User, "Renamed account"); err != nil {
		t.Fatal(err)
	}
	fresh := token("owner", "reconnected", "changed@example.com")
	if p := read(fresh); p.ID != f.session.User || p.Name != "Renamed account" {
		t.Fatal("reconnect or label change replaced profile", p)
	}
	if !f.call(t, old, "ding_get_profile", nil).IsError {
		t.Fatal("old consent still identifies the account")
	}
	if err := f.s.DB.ConfigureLoginMethods(ctx, []string{"password"}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.EnableOIDC(ctx, state.IdentityLink{User: f.session.User, Issuer: f.s.AuthKitIssuer, Subject: "linked"}); err != nil {
		t.Fatal(err)
	}
	linked := token("linked", "linked-consent", "different@example.com")
	if p := read(linked); p.ID != f.session.User {
		t.Fatal("verified linked identity replaced account", p)
	}
	other, err := f.s.DB.Enroll(ctx, f.s.AuthKitIssuer, "other", 10)
	if err != nil {
		t.Fatal(err)
	}
	if p := read(token("other", "other-consent", "changed@example.com")); p.ID != other.User || p.ID == f.session.User {
		t.Fatal("email conflated distinct accounts", p)
	}
	for _, args := range []map[string]any{{"user_id": other.User}, {"workspace_id": other.ID}, {"email": "changed@example.com"}} {
		if !f.call(t, linked, "ding_get_profile", args).IsError {
			t.Fatal("profile accepted a caller-selected identity", args)
		}
	}
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", `{"clientID":"chatgpt","revoke":true}`, f.session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !f.call(t, linked, "ding_get_profile", nil).IsError {
		t.Fatal("revoked consent retained profile access")
	}
}

func TestHostedProfileMetadataAndAuthentication(t *testing.T) {
	f := newHostedMCPFixture(t)
	token := f.token("owner", "client", "ding:inspect", "")
	for _, version := range []string{"2025-11-25", "2026-07-28"} {
		w := f.request(t, token, version, "tools/list", nil)
		var envelope struct {
			Result mcp.ListToolsResult `json:"result"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &envelope) != nil {
			t.Fatal(w.Body.String())
		}
		found := false
		for _, tool := range envelope.Result.Tools {
			if tool.Name != "ding_get_profile" {
				continue
			}
			found = true
			if tool.Meta["openai/profile"] != true || !tool.Annotations.ReadOnlyHint {
				t.Fatal("missing profile metadata", tool)
			}
			schema, err := mcpcontract.Resolve(tool.OutputSchema)
			if err != nil || schema.Validate(map[string]any{"id": f.session.User}) != nil {
				t.Fatal("invalid profile schema", err)
			}
			if schema.Validate(map[string]any{"id": " "}) == nil || schema.Validate(map[string]any{"id": f.session.User, "token": "secret"}) == nil {
				t.Fatal("permissive profile schema")
			}
		}
		if !found {
			t.Fatal("profile tool absent")
		}
	}
	for _, bearer := range []string{"", f.token("owner", "client", "ding:inspect", "", map[string]any{"exp": time.Now().Add(-time.Hour)}), f.token("owner", "client", "ding:preview", "")} {
		w := f.request(t, bearer, "2026-07-28", "tools/call", map[string]any{"name": "ding_get_profile", "arguments": map[string]any{}})
		if w.Code != 401 && w.Code != 403 {
			t.Fatal("invalid credentials reached profile", w.Code, w.Body.String())
		}
	}
	if !f.call(t, f.token("owner", "unapproved-client", "ding:inspect", ""), "ding_get_profile", nil).IsError {
		t.Fatal("unapproved client received profile")
	}
}
