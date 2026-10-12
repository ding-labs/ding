package mcpbridge

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExplicitTargetsStayIsolatedAndNeverFallBack(t *testing.T) {
	var localCalls, cloudCalls atomic.Int32
	backendFor := func(instance string, count *atomic.Int32) backend {
		return func(ctx context.Context, fn func(mcpcontract.Target, call) (mcpcontract.View, error)) (mcpcontract.View, error) {
			return fn(mcpcontract.Target{Instance: instance, Workspace: instance, Execution: instance}, func(context.Context, string, map[string]any) (mcpcontract.View, error) {
				count.Add(1)
				return mcpcontract.View{View: "watch", Data: map[string]any{"id": "same-watch", "instance": instance}}, nil
			})
		}
	}
	b := &Bridge{journal: t.TempDir(), backends: map[string]backend{"local": backendFor("local", &localCalls), "cloud": backendFor("cloud", &cloudCalls)}}
	ctx := context.Background()
	for _, args := range []map[string]any{{}, {"connection_id": "unknown"}, {"connection_id": "local", "workspace_id": "cloud"}} {
		if _, err := b.invoke(ctx, "ding_get_watch", args); err == nil {
			t.Fatal("accepted ambiguous target")
		}
	}
	if localCalls.Load() != 0 || cloudCalls.Load() != 0 {
		t.Fatal("invalid target caused work")
	}
	var wg sync.WaitGroup
	for _, connection := range []string{"local", "cloud"} {
		wg.Go(func() {
			for range 10 {
				view, err := b.invoke(ctx, "ding_get_watch", map[string]any{"connection_id": connection, "watch_id": "same-watch"})
				if err != nil || view.Target.Connection != connection || view.Data.(map[string]any)["instance"] != connection {
					t.Error("crossed target", view, err)
				}
			}
		})
	}
	wg.Wait()
	if localCalls.Load() != 10 || cloudCalls.Load() != 10 {
		t.Fatal("lost calls")
	}
	args := map[string]any{"connection_id": "cloud", "operation_key": "key", "watch_id": "same-watch"}
	if _, err := b.invoke(ctx, "ding_pause_watch", args); err != nil {
		t.Fatal(err)
	}
	args["connection_id"] = "local"
	if _, err := b.invoke(ctx, "ding_pause_watch", args); err == nil {
		t.Fatal("operation key crossed target")
	}
	b.backends["cloud"] = func(context.Context, func(mcpcontract.Target, call) (mcpcontract.View, error)) (mcpcontract.View, error) {
		return mcpcontract.View{}, mcpclient.Fail("cloud_unavailable", "offline")
	}
	args["connection_id"] = "cloud"
	if _, err := b.invoke(ctx, "ding_pause_watch", args); err == nil {
		t.Fatal("remote failure ignored")
	}
	if localCalls.Load() != 10 {
		t.Fatal("remote failure fell back to local")
	}
	if _, err := b.invoke(ctx, "ding_get_watch", map[string]any{"connection_id": "local"}); err != nil {
		t.Fatal("local reads depend on remote availability")
	}
}

func TestBridgeDiscoveryOverSDKRequiresNoRemoteConnection(t *testing.T) {
	b := &Bridge{journal: t.TempDir(), backends: map[string]backend{}}
	server, err := b.server("test", "")
	if err != nil {
		t.Fatal(err)
	}
	a, z := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := server.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, z, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_list_connections", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
}
