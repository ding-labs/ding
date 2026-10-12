package mcpremote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPSRefreshWorkspacePinAndRestart(t *testing.T) {
	p := testProfile()
	p.Token.Expiry = time.Now().Add(-time.Minute)
	var refreshes atomic.Int32
	var replaced atomic.Bool
	server := mcp.NewServer(&mcp.Implementation{Name: "Ding test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ding_get_capabilities"}, func(_ context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, mcpcontract.View, error) {
		if args["workspace_id"] != p.Workspace {
			t.Error("workspace target lost")
		}
		instance := p.Instance
		if replaced.Load() {
			instance = "other-instance"
		}
		return nil, mcpcontract.View{View: "capabilities", Query: map[string]any{}, Target: &mcpcontract.Target{Instance: instance, Workspace: p.Workspace, Execution: "cloud"}, Data: map[string]any{"effectiveScopes": []string{"inspect"}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh" {
			t.Error("wrong refresh credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "rotated", "refresh_token": "rotated-refresh", "token_type": "Bearer", "expires_in": 3600})
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer rotated" {
			http.Error(w, "unauthorized", 401)
			return
		}
		handler.ServeHTTP(w, r)
	})
	host := httptest.NewTLSServer(mux)
	defer host.Close()
	p.URL = host.URL + "/mcp"
	p.TokenURL = host.URL + "/token"
	path := filepath.Join(t.TempDir(), "cloud.json")
	if err := mcpconfig.AtomicJSON(path, p); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		err := withSession(context.Background(), path, host.Client(), func(cs *mcp.ClientSession, p Profile) error {
			_, err := Capabilities(context.Background(), cs, p)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if refreshes.Load() != 1 {
		t.Fatal("restart did not reuse persisted rotation", refreshes.Load())
	}
	loaded, err := Load(path)
	if err != nil || loaded.Token.RefreshToken != "rotated-refresh" {
		t.Fatal(err)
	}
	replaced.Store(true)
	if err := withSession(context.Background(), path, host.Client(), func(cs *mcp.ClientSession, p Profile) error {
		_, err := Capabilities(context.Background(), cs, p)
		return err
	}); err == nil {
		t.Fatal("instance replacement accepted")
	}
}
