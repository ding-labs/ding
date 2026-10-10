package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ding-labs/ding/internal/control"
	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/ding-labs/ding/internal/mcpsetup"
	"github.com/ding-labs/ding/internal/store"
	"github.com/ding-labs/ding/internal/watchrun"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const manifest = "apiVersion: ding.ing/v1alpha1\nkind: Watch\nmetadata: {id: heartbeat, name: Heartbeat}\nspec:\n  source: {type: push}\n  condition: {missingFor: 5m}\n"

func TestRealDaemonPairingReviewLifecycleRevocation(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := store.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := watchrun.New(db)
	creds := control.Credentials{Admin: strings.Repeat("a", 64), Ingest: strings.Repeat("i", 64)}
	d := httptest.NewServer(control.Handler(app, creds))
	defer d.Close()
	for name, value := range map[string]any{"connection.json": map[string]any{"url": d.URL}, "tokens.json": map[string]any{"admin": creds.Admin}} {
		f, err := mcpconfig.CreatePrivate(filepath.Join(state, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(f).Encode(value); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	config := filepath.Join(t.TempDir(), "mcp.json")
	var output bytes.Buffer
	if err := mcpsetup.Pair(ctx, mcpsetup.PairOptions{State: state, Config: config, Name: "test", Days: 1, Manage: true, Retry: true}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "ding_mcp_") {
		t.Fatal("credential printed")
	}
	connection, err := mcpconfig.LoadConnection(config)
	if err != nil {
		t.Fatal(err)
	}
	daemon := mcpclient.New(connection)
	defer daemon.Close()
	s, err := mcpserver.New(mcpserver.Options{Version: "test", HTML: "<title>Ding</title>", Resolve: func(context.Context, string) (*mcpclient.Client, error) { return daemon, nil }})
	if err != nil {
		t.Fatal(err)
	}
	a, b := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := c.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		if args == nil {
			args = map[string]any{}
		}
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || r.IsError {
			t.Fatalf("%s: %v %v", name, r, err)
		}
		buf, _ := json.Marshal(r.StructuredContent)
		var v mcpcontract.View
		if err := json.Unmarshal(buf, &v); err != nil {
			t.Fatal(err)
		}
		return v.Data.(map[string]any)
	}
	for _, name := range []string{"ding_get_capabilities", "ding_list_watches", "ding_list_events", "ding_list_deliveries", "ding_list_destinations"} {
		call(name, nil)
	}
	preview := call("ding_preview_changes", map[string]any{"manifest": manifest})
	handle := preview["preview"].(map[string]any)["handle"]
	args := map[string]any{"handle": handle, "operation_key": "real_apply_key_000001"}
	applied := call("ding_apply_changes", args)
	again := call("ding_apply_changes", args)
	x, _ := json.Marshal(applied)
	y, _ := json.Marshal(again)
	if !bytes.Equal(x, y) {
		t.Fatal("receipt replay changed result")
	}
	watch := call("ding_get_watch", map[string]any{"watch_id": "heartbeat"})["watch"].(map[string]any)
	revision := watch["plan"].(map[string]any)["revision"]
	paused := call("ding_pause_watch", map[string]any{"watch_id": "heartbeat", "expected_revision": revision, "expected_generation": watch["generation"], "operation_key": "real_pause_key_000001"})
	if paused["status"] != "paused" {
		t.Fatal(paused)
	}
	if call("ding_get_operation", map[string]any{"operation_key": "real_pause_key_000001"})["action"] != "pause" {
		t.Fatal("receipt")
	}
	events := call("ding_list_events", map[string]any{"watch": "heartbeat"})["events"].([]any)
	if len(events) == 0 {
		t.Fatal("no events")
	}
	call("ding_get_event", map[string]any{"event_id": events[0].(map[string]any)["id"]})
	resource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: mcpcontract.AppURI})
	if err != nil || resource.Contents[0].Text != "<title>Ding</title>" {
		t.Fatal("resource", err)
	}
	if _, _, err := mcpsetup.AdminCall(ctx, state, "DELETE", connection.GrantID, nil); err != nil {
		t.Fatal(err)
	}
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_apply_changes", Arguments: args})
	if err == nil && !r.IsError {
		t.Fatal("revoked receipt replay accepted")
	}
	if err := mcpsetup.Pair(ctx, mcpsetup.PairOptions{State: state, Config: config, Days: 1}, &output); err == nil {
		t.Fatal("existing pairing overwritten")
	}
}
