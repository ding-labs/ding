package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCloudWorkspaceSelectionIsRequestLocal(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"apiVersion": "ding.ing/v1alpha1", "data": map[string]any{"watches": []any{}, "total": 0, "more": false, "cursor": ""}})
	}))
	defer daemon.Close()
	ctx := context.Background()
	var resolutions atomic.Int32
	options := Options{Resolve: func(ctx context.Context, _ string, _ *auth.TokenInfo) (*mcpclient.Client, error) {
		resolutions.Add(1)
		return &mcpclient.Client{Connection: mcpconfig.Connection{DaemonURL: daemon.URL, Token: "ding_mcp_" + strings.Repeat("a", 64), GrantID: strings.Repeat("b", 64)}, HTTP: daemon.Client(), Target: &mcpcontract.Target{Instance: "instance", Execution: "cloud", Workspace: RequestedWorkspace(ctx)}}, nil
	}, ListWorkspaces: func(context.Context, *auth.TokenInfo) ([]mcpcontract.Workspace, error) {
		return []mcpcontract.Workspace{}, nil
	}}
	for _, cloud := range []bool{true, false} {
		if !cloud {
			options.ListWorkspaces = nil
		}
		server, err := New(options)
		if err != nil {
			t.Fatal(err)
		}
		a, b := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, a, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, b, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cs.Close()
		if !cloud {
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_list_watches", Arguments: map[string]any{"workspace_id": strings.Repeat("a", 64)}})
			if err == nil && !result.IsError {
				t.Fatal("local server ignored explicit target")
			}
			continue
		}
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_list_workspaces", Arguments: map[string]any{}})
		if err != nil || result.IsError || resolutions.Load() != 0 {
			t.Fatal("discovery resolved a tenant", result, err)
		}
		var wg sync.WaitGroup
		for _, id := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
			wg.Go(func() {
				result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_list_watches", Arguments: map[string]any{"workspace_id": id}})
				if err != nil || result.IsError {
					t.Error(result, err)
					return
				}
				data, _ := json.Marshal(result.StructuredContent)
				var view mcpcontract.View
				if err := json.Unmarshal(data, &view); err != nil || view.Target == nil || view.Target.Workspace != id {
					t.Error("crossed workspace", string(data), err)
				}
			})
		}
		wg.Wait()
	}
}
