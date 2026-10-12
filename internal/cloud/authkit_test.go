package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/access"
	"github.com/ding-labs/ding/internal/store"
)

type consentRevokerFunc func(context.Context, string, string) error

func (f consentRevokerFunc) Revoke(ctx context.Context, subject, client string) error {
	return f(ctx, subject, client)
}

func TestAuthKitDisconnectCleanupRetry(t *testing.T) {
	f := newHostedMCPFixture(t)
	f.s.AuthKitClients = []string{"public", "private"}
	token := func(client, consent string) string {
		return f.token("owner", client, "ding:inspect", "", map[string]any{"sid": consent})
	}
	for _, client := range f.s.AuthKitClients {
		hostedView(t, f.call(t, token(client, "original"), "ding_get_capabilities", nil))
	}
	ctx := context.Background()
	connection, err := f.s.Vault.MCP(ctx, f.session.User, f.session.Workspace, "private")
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	f.s.ConsentRevoker = consentRevokerFunc(func(ctx context.Context, subject, client string) error {
		attempts++
		if subject != "owner" || client != "private" {
			t.Fatal("wrong consent target", subject, client)
		}
		tenant, err := f.s.Pool.Get(ctx, f.session.Workspace)
		if err != nil {
			t.Fatal(err)
		}
		if err := tenant.App.Store.View(ctx, func(tx *store.Tx) error {
			grant, err := tx.IntegrationGrant(connection.GrantID)
			if err == nil && !grant.Revoked {
				t.Error("contacted provider before disabling local access")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if attempts == 1 {
			return errors.New("provider unavailable")
		}
		return nil
	})
	revoke := func() int {
		return cloudRequest(f.h, "POST", "/v1/cloud/models", `{"clientID":"private","revoke":true}`, f.session, true).Code
	}
	if got := revoke(); got != 503 {
		t.Fatal("expected retryable cleanup", got)
	}
	// The disabled binding is the durable retry record; old and new consents
	// cannot reconnect while cleanup is pending, including across restarts.
	if _, err := f.s.Vault.MCP(ctx, f.session.User, f.session.Workspace, "private"); err != nil {
		t.Fatal("lost pending cleanup", err)
	}
	for _, consent := range []string{"original", "new"} {
		if !f.call(t, token("private", consent), "ding_get_capabilities", nil).IsError {
			t.Fatal("pending cleanup allowed access")
		}
	}
	hostedView(t, f.call(t, token("public", "original"), "ding_get_capabilities", nil))
	if got := revoke(); got != 200 {
		t.Fatal("cleanup retry failed", got)
	}
	if got := revoke(); got != 200 || attempts != 2 {
		t.Fatal("disconnect not idempotent", got, attempts)
	}
	if !f.call(t, token("private", "original"), "ding_get_capabilities", nil).IsError {
		t.Fatal("old token revived")
	}
	hostedView(t, f.call(t, token("private", "new"), "ding_get_capabilities", nil))
	hostedView(t, f.call(t, token("public", "original"), "ding_get_capabilities", nil))
}

func TestAuthKitHostedConsentLifecycle(t *testing.T) {
	f := newHostedMCPFixture(t)
	f.s.AuthKitClients = []string{"chatgpt"}
	limit := 10
	f.s.Login.EnrollmentLimit = &limit
	ctx := context.Background()
	token := func(consent, scope string) string {
		return f.token("new-user", "chatgpt", scope, "", map[string]any{"sid": consent})
	}
	first := token("consent-one", "ding:inspect ding:preview ding:manage ding:retry")
	before, _ := f.s.DB.Workspaces(ctx)
	view := hostedView(t, f.call(t, first, "ding_get_capabilities", nil))
	raw, _ := json.Marshal(view)
	if !strings.Contains(string(raw), `"execution":"cloud"`) {
		t.Fatal(string(raw))
	}
	after, _ := f.s.DB.Workspaces(ctx)
	if len(after) != len(before)+1 {
		t.Fatal("first consent did not provision one workspace")
	}
	var workspace string
	for _, w := range after {
		found := false
		for _, b := range before {
			if w.ID == b.ID {
				found = true
			}
		}
		if !found {
			workspace = w.ID
		}
	}
	tenant, err := f.s.Pool.Get(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	grants := func() []store.IntegrationGrant {
		t.Helper()
		var out []store.IntegrationGrant
		if err := tenant.App.Store.View(ctx, func(tx *store.Tx) error { var e error; out, e = tx.IntegrationGrants(); return e }); err != nil {
			t.Fatal(err)
		}
		return out
	}
	initial := grants()
	if len(initial) != 1 || initial[0].ProviderConsent == "" {
		t.Fatal("missing provider consent")
	}
	// A later token narrows authority without replacing the private grant or its receipts.
	hostedView(t, f.call(t, token("consent-one", "ding:inspect"), "ding_get_capabilities", nil))
	refreshed := grants()
	if len(refreshed) != 1 || refreshed[0].ID != initial[0].ID || strings.Join(refreshed[0].Scopes, " ") != "inspect" {
		t.Fatal("refresh did not reuse and narrow grant")
	}
	if !f.call(t, token("consent-one", "ding:inspect"), "ding_retry_delivery", nil).IsError {
		t.Fatal("missing scope accepted")
	}
	// Disconnect through the same browser route as the Console, then replay a valid JWT.
	members, err := f.s.DB.Members(ctx, workspace)
	if err != nil || len(members) != 1 {
		t.Fatal(err)
	}
	session, err := f.s.DB.NewSession(ctx, members[0].ID, workspace, "browser", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", `{"clientID":"chatgpt","revoke":true}`, session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !f.call(t, first, "ding_get_capabilities", nil).IsError {
		t.Fatal("disconnected consent recreated itself")
	}
	hostedView(t, f.call(t, token("consent-two", "ding:inspect"), "ding_get_capabilities", nil))
	if len(grants()) != 2 {
		t.Fatal("new consent did not create a fresh grant")
	}
	if !f.call(t, first, "ding_get_capabilities", nil).IsError {
		t.Fatal("old consent replaced the new connection")
	}
	// Current role and membership remain authoritative over already-issued tokens.
	owner, err := f.s.DB.Principal(ctx, session.User, workspace, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.ChangeMember(ctx, owner, f.session.User, access.Owner); err != nil {
		t.Fatal(err)
	}
	otherOwner, err := f.s.DB.Principal(ctx, f.session.User, workspace, "browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.ChangeMember(ctx, otherOwner, session.User, access.Viewer); err != nil {
		t.Fatal(err)
	}
	broad := token("consent-two", "ding:inspect ding:preview ding:manage ding:retry")
	hostedView(t, f.call(t, broad, "ding_get_capabilities", nil))
	for _, g := range grants() {
		if !g.Revoked && strings.Contains(strings.Join(g.Scopes, " "), "manage") {
			t.Fatal("role downgrade retained management")
		}
	}
	if err := f.s.DB.ChangeMember(ctx, otherOwner, session.User, ""); err != nil {
		t.Fatal(err)
	}
	if !f.call(t, broad, "ding_get_capabilities", nil).IsError {
		t.Fatal("removed membership recreated access")
	}
	final, _ := f.s.DB.Workspaces(ctx)
	if len(final) != len(after) {
		t.Fatal("removed identity re-enrolled")
	}
}

func TestAuthKitRejectsUntrustedEnrollment(t *testing.T) {
	f := newHostedMCPFixture(t)
	f.s.AuthKitClients = []string{"chatgpt"}
	limit := 10
	f.s.Login.EnrollmentLimit = &limit
	before, _ := f.s.DB.Workspaces(context.Background())
	for name, token := range map[string]string{
		"malformed-organization": f.token("unknown", "chatgpt", "ding:inspect", "", map[string]any{"sid": "s", "org_id": 17}),
		"malformed-consent":      f.token("unknown", "chatgpt", "ding:inspect", "", map[string]any{"sid": []string{"s"}}),
		"missing-consent":        f.token("unknown", "chatgpt", "ding:inspect", ""),
		"organization":           f.token("unknown", "chatgpt", "ding:inspect", "", map[string]any{"sid": "s", "org_id": "org"}),
		"wrong-client":           f.token("unknown", "unregistered", "ding:inspect", "", map[string]any{"sid": "s"}),
		"missing-scope":          f.token("unknown", "chatgpt", "ding:manage", "", map[string]any{"sid": "s"}),
		"wrong-audience":         f.token("unknown", "chatgpt", "ding:inspect", "https://elsewhere.example/mcp", map[string]any{"sid": "s"}),
	} {
		t.Run(name, func(t *testing.T) {
			w := f.request(t, token, "2026-07-28", "tools/call", map[string]any{"name": "ding_get_capabilities", "arguments": map[string]any{}})
			if w.Code == 200 && !strings.Contains(w.Body.String(), `"isError":true`) {
				t.Fatal(w.Body.String())
			}
		})
	}
	limit = 0
	if !f.call(t, f.token("unknown", "chatgpt", "ding:inspect", "", map[string]any{"sid": "s"}), "ding_get_capabilities", nil).IsError {
		t.Fatal("closed enrollment accepted")
	}
	after, _ := f.s.DB.Workspaces(context.Background())
	if len(after) != len(before) {
		t.Fatal("untrusted request enrolled an account")
	}
	// A manually approved client cannot silently turn into renewable provider consent.
	f.s.AuthKitClients = []string{"client"}
	if !f.call(t, f.token("owner", "client", "ding:inspect", "", map[string]any{"sid": "s"}), "ding_get_capabilities", nil).IsError {
		t.Fatal("manual grant promoted")
	}
}

func TestAuthKitPrivateClientIsolation(t *testing.T) {
	f := newHostedMCPFixture(t)
	f.s.AuthKitClients = []string{"public", "private"}
	ctx := context.Background()
	// Even the same provider consent identifier must stay scoped to its client.
	public := f.token("owner", "public", "ding:inspect", "", map[string]any{"sid": "consent"})
	private := f.token("owner", "private", "ding:inspect", "", map[string]any{"sid": "consent"})
	for _, token := range []string{public, private} {
		hostedView(t, f.call(t, token, "ding_get_capabilities", nil))
	}
	a, err := f.s.Vault.MCP(ctx, f.session.User, f.session.Workspace, "public")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.s.Vault.MCP(ctx, f.session.User, f.session.Workspace, "private")
	if err != nil {
		t.Fatal(err)
	}
	if a.GrantID == b.GrantID {
		t.Fatal("clients share a grant")
	}
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", `{"clientID":"private","revoke":true}`, f.session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !f.call(t, private, "ding_get_capabilities", nil).IsError {
		t.Fatal("revoked private consent accepted")
	}
	hostedView(t, f.call(t, public, "ding_get_capabilities", nil))
	newPrivate := f.token("owner", "private", "ding:inspect", "", map[string]any{"sid": "new-consent"})
	hostedView(t, f.call(t, newPrivate, "ding_get_capabilities", nil))
	// Operator removal fences already-bound clients as well as enrollment.
	f.s.AuthKitClients = []string{"public"}
	if !f.call(t, newPrivate, "ding_get_capabilities", nil).IsError {
		t.Fatal("removed client retained access")
	}
	hostedView(t, f.call(t, public, "ding_get_capabilities", nil))
	if !f.call(t, f.token("owner", "client", "ding:inspect", ""), "ding_get_capabilities", nil).IsError {
		t.Fatal("manual binding bypassed AuthKit allowlist")
	}
}
