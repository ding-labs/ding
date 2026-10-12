package cloud

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/ding-labs/ding/internal/mcpui"
	"github.com/ding-labs/ding/internal/store"
)

func TestHostedHTTPWatchJourneyAndConsentRemoval(t *testing.T) {
	f := newHostedMCPFixture(t)
	token := f.token("owner", "client", "ding:inspect ding:preview ding:manage ding:retry", "")
	w := f.request(t, token, "2025-11-25", "initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "qualification", "version": "test"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"serverInfo"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = f.request(t, token, "2026-07-28", "tools/list", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "ding_list_workspaces") {
		t.Fatal(w.Code, w.Body.String())
	}
	seen := map[string]bool{}
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		seen[name] = true
		view := hostedView(t, f.call(t, token, name, args))
		if name != "ding_list_workspaces" && (view.Target == nil || view.Target.Workspace != f.session.Workspace) {
			t.Fatal("missing or incorrect target", view.Target)
		}
		return view.Data.(map[string]any)
	}
	for _, name := range []string{"ding_get_capabilities", "ding_list_workspaces", "ding_get_cloud_status", "ding_list_watches", "ding_list_destinations"} {
		call(name, nil)
	}
	manifest, err := Template(FirstWatch{ID: "http-test", URL: "https://example.com", Destination: "webhook", Credential: "WEBHOOK"})
	if err != nil {
		t.Fatal(err)
	}
	manifest = strings.Replace(manifest, "consecutive: 3", "consecutive: 1", 1)
	preview := call("ding_preview_changes", map[string]any{"manifest": manifest})
	applyArgs := map[string]any{"handle": preview["preview"].(map[string]any)["handle"], "operation_key": "hosted_apply_key_0001"}
	applied := call("ding_apply_changes", applyArgs)
	if !bytes.Equal(mustJSON(t, applied), mustJSON(t, call("ding_apply_changes", applyArgs))) {
		t.Fatal("duplicate apply did not replay receipt")
	}
	if call("ding_get_operation", map[string]any{"operation_key": "hosted_apply_key_0001"})["action"] != "apply" {
		t.Fatal("receipt missing")
	}
	tenant, err := f.s.Pool.Get(context.Background(), f.session.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	var delivery store.ConsoleDelivery
	deadline := time.Now().Add(6 * time.Second)
	for delivery.Status != "permanent" && time.Now().Before(deadline) {
		err := tenant.App.Store.View(context.Background(), func(tx *store.Tx) error {
			page, err := tx.ConsoleDeliveries(store.ConsoleQuery{Limit: 25})
			if len(page.Deliveries) > 0 {
				delivery = page.Deliveries[0]
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if delivery.Status != "permanent" {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if delivery.Status != "permanent" {
		t.Fatal("worker did not record synthetic delivery", delivery)
	}
	call("ding_list_deliveries", nil)
	call("ding_get_delivery", map[string]any{"delivery_id": delivery.ID})
	call("ding_retry_delivery", map[string]any{"delivery_id": delivery.ID, "operation_key": "hosted_retry_key_0001"})
	events := call("ding_list_events", map[string]any{"watch": "http-test"})["events"].([]any)
	if len(events) == 0 {
		t.Fatal("no watch events")
	}
	call("ding_get_event", map[string]any{"event_id": events[0].(map[string]any)["id"]})
	for _, action := range []string{"pause", "resume", "delete"} {
		watch := call("ding_get_watch", map[string]any{"watch_id": "http-test"})["watch"].(map[string]any)
		args := map[string]any{"watch_id": "http-test", "expected_revision": watch["plan"].(map[string]any)["revision"], "expected_generation": watch["generation"], "operation_key": "hosted_" + action + "_key_0001", "cancel_pending": action == "delete"}
		if action != "delete" {
			delete(args, "cancel_pending")
		}
		call("ding_"+action+"_watch", args)
	}
	if transfers := call("ding_list_transfers", nil)["transfers"].([]any); len(transfers) != 0 {
		t.Fatal("unexpected cloud transfers", transfers)
	}
	if preflight := call("ding_get_transfer_preflight", map[string]any{"watch_id": "http-test"}); preflight["ready"] != false {
		t.Fatal("deleted watch is movable")
	}
	if result := f.call(t, token, "ding_get_transfer", map[string]any{"transfer_id": strings.Repeat("a", 64)}); !result.IsError {
		t.Fatal("invented transfer receipt")
	}
	if len(seen) != 19 {
		t.Fatal("not all tools exercised", seen)
	}
	if mcpui.WorkspaceHTML != "" {
		w = f.request(t, token, "2026-07-28", "resources/read", map[string]any{"uri": mcpcontract.AppURI})
		if w.Code != 200 || !strings.Contains(w.Body.String(), "contents") {
			t.Fatal("missing embedded resource", w.Code, w.Body.String())
		}
	}
	if w := cloudRequest(f.h, "POST", "/v1/cloud/models", `{"clientID":"client","revoke":true}`, f.session, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if result := f.call(t, token, "ding_get_capabilities", nil); !result.IsError {
		t.Fatal("still-valid JWT recreated revoked consent")
	}
}
