package cloud

import (
	"strings"
	"testing"
)

func TestHostedTransferTransportOnlyForNativeBridgeConsent(t *testing.T) {
	f := newHostedMCPFixture(t)
	scopes := "ding:inspect ding:preview ding:manage ding:move"
	ordinary := f.token("owner", "client", scopes, "")
	if w := f.request(t, ordinary, "2026-07-28", "tools/list", nil); strings.Contains(w.Body.String(), "ding_bridge_transfer_step") {
		t.Fatal("peer receipt transport exposed to model client")
	}
	consent := `{"clientID":"ordinary","grant":{"name":"move","days":1,"scopes":["inspect","preview","manage","move"]}}`
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", consent, f.session, true); w.Code != 403 {
		t.Fatal("model client obtained transfer consent", w.Code, w.Body.String())
	}
	consent = `{"clientID":"native-bridge","grant":{"name":"bridge","days":1,"scopes":["inspect","preview","manage","move"],"secretRefs":["WEBHOOK"]}}`
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", consent, f.session, true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	token := f.token("owner", "native-bridge", scopes, "")
	if w := f.request(t, token, "2026-07-28", "tools/list", nil); !strings.Contains(w.Body.String(), "ding_bridge_transfer_step") {
		t.Fatal("native transport missing", w.Body.String())
	}
	manifest, err := Template(FirstWatch{ID: "move-test", URL: "https://example.com", Destination: "webhook", Credential: "WEBHOOK"})
	if err != nil {
		t.Fatal(err)
	}
	preview := hostedView(t, f.call(t, token, "ding_preview_changes", map[string]any{"manifest": manifest})).Data.(map[string]any)
	review := preview["preview"].(map[string]any)["changes"].(map[string]any)["review"]
	id := strings.Repeat("c", 64)
	step := func(action string, input map[string]any) map[string]any {
		t.Helper()
		return hostedView(t, f.call(t, token, "ding_bridge_transfer_step", map[string]any{"action": action, "transfer_id": id, "input": input})).Data.(map[string]any)
	}
	prepared := step("prepare", map[string]any{"id": id, "peer": strings.Repeat("d", 64), "manifest": manifest, "review": review})
	if prepared["phase"] != "prepared" {
		t.Fatal(prepared)
	}
	watch := hostedView(t, f.call(t, token, "ding_get_watch", map[string]any{"watch_id": "move-test"})).Data.(map[string]any)["watch"].(map[string]any)
	if watch["status"] != "paused" {
		t.Fatal("preparation activated target", watch)
	}
	if tested := step("test", map[string]any{"operationKey": "native_transfer_test_0001"}); tested["outcome"] == "accepted" {
		t.Fatal("synthetic failing source accepted", tested)
	}
	if result := f.call(t, token, "ding_bridge_transfer_step", map[string]any{"action": "activate", "transfer_id": id, "input": map[string]any{}}); !result.IsError {
		t.Fatal("activated without successful test or source receipt")
	}
	for range 2 {
		if canceled := step("cancel", map[string]any{}); canceled["phase"] != "canceled" {
			t.Fatal(canceled)
		}
	}
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", `{"clientID":"native-bridge","revoke":true}`, f.session, true); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if result := f.call(t, token, "ding_bridge_transfer_step", map[string]any{"action": "cancel", "transfer_id": id, "input": map[string]any{}}); !result.IsError {
		t.Fatal("revoked transfer consent replayed")
	}
}
