package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/watchrun"
)

func TestHostedMCPDestinationReadinessRoundTrip(t *testing.T) {
	f := newHostedMCPFixture(t)
	token := f.token("owner", "client", "ding:inspect", "")
	read := func(ref, want string) map[string]any {
		t.Helper()
		args := map[string]any{}
		if ref != "" {
			args["destination_ref"] = ref
		}
		view := hostedView(t, f.call(t, token, "ding_get_destination_setup", args))
		data := view.Data.(map[string]any)
		if data["state"] != want || data["accountID"] != f.session.User || data["workspaceID"] != f.session.Workspace {
			t.Fatal(data)
		}
		link, err := url.Parse(data["setupURL"].(string))
		if err != nil || !strings.HasPrefix(link.String(), f.s.PublicURL+"/ui/destination-setup?") || link.Query().Get("account") != f.session.User || link.Query().Get("workspace") != f.session.Workspace {
			t.Fatal("wrong setup target", link, err)
		}
		raw := string(mustJSON(t, data))
		if strings.Contains(raw, "digest") || strings.Contains(raw, "hooks.example") {
			t.Fatal("private setup material exposed")
		}
		return data
	}
	read("", "missing")
	read("shared-alerts", "missing")
	tenant, err := f.s.Pool.Get(context.Background(), f.session.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tenant.App.Apply(context.Background(), watchrun.ApplyRequest{Manifest: reusedDestination})
	if err != nil {
		t.Fatal(err)
	}
	read("shared-alerts", "untested")
	sent := 0
	tenant.App.HTTP.Client = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		sent++
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	req := watchrun.DestinationTestRequest{AccountID: f.session.User, WorkspaceID: f.session.Workspace, DestinationRef: "shared-alerts", Revision: result.DestinationChanges[0].Revision, OperationKey: "mcp-setup-roundtrip-0001"}
	body, _ := json.Marshal(req)
	if w := cloudRequest(f.h, "POST", "/v1/cloud/destination-test", string(body), f.session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	read("shared-alerts", "accepted")
	for _, args := range []map[string]any{{"destination_ref": "shared-alerts", "confirmed": true}, {"account_id": f.session.User}, {"workspace_id": strings.Repeat("a", 64)}} {
		if r := f.call(t, token, "ding_get_destination_setup", args); !r.IsError {
			t.Fatal("caller overrode setup authority", args)
		}
	}
	if w := cloudRequest(f.h, "POST", "/v1/onboarding/destination-confirm", string(body), f.session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for range 2 {
		read("shared-alerts", "confirmed")
	}
	if err := f.s.Vault.Put(context.Background(), f.session.Workspace, "WEBHOOK", "https://hooks.example/rotated"); err != nil {
		t.Fatal(err)
	}
	read("shared-alerts", "stale")
	if sent != 1 {
		t.Fatal("inspection sent a test", sent)
	}
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", `{"clientID":"client","revoke":true}`, f.session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if r := f.call(t, token, "ding_get_destination_setup", map[string]any{"destination_ref": "shared-alerts"}); !r.IsError {
		t.Fatal("revoked caller read readiness")
	}
}
