package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Captured from FastMCP before cutover; no Python installation is needed to
// verify every route, input default, output default, and compatible extra field.
func TestPortableToolResults(t *testing.T) {
	var fixtures []struct {
		Tool         string
		Arguments    map[string]any
		Data, Result any
		Request      struct {
			Method, Path string
			Query        map[string]any
			Body         any
		}
	}
	b, err := os.ReadFile("../../testdata/mcp/fastmcp-results.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Tool, func(t *testing.T) {
			ctx := context.Background()
			var malformed atomic.Bool
			var calls atomic.Int32
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != fixture.Request.Method || r.URL.EscapedPath() != "/v1/integrations"+fixture.Request.Path {
					t.Errorf("route %s %s", r.Method, r.URL.EscapedPath())
				}
				if len(r.URL.Query()) != len(fixture.Request.Query) {
					t.Error("query fields changed")
				}
				for key, value := range fixture.Request.Query {
					if r.URL.Query().Get(key) != fmt.Sprint(value) {
						t.Errorf("query %s", key)
					}
				}
				var body any
				if fixture.Request.Body != nil {
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
				}
				if !reflect.DeepEqual(body, fixture.Request.Body) {
					t.Errorf("body %v", body)
				}
				data := fixture.Data
				if malformed.Load() {
					data = "private-parser-detail-must-not-leak"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "ding.ing/v1alpha1", "data": data})
			}))
			defer daemon.Close()
			client := mcpclient.New(mcpconfig.Connection{DaemonURL: daemon.URL, Token: "ding_mcp_" + strings.Repeat("a", 64), GrantID: strings.Repeat("b", 64)})
			defer client.Close()
			server, err := mcpserver.New(mcpserver.Options{Version: "test", Resolve: func(context.Context, string, *auth.TokenInfo) (*mcpclient.Client, error) { return client, nil }})
			if err != nil {
				t.Fatal(err)
			}
			a, b := mcp.NewInMemoryTransports()
			ss, err := server.Connect(ctx, a, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ss.Close()
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "parity", Version: "test"}, nil).Connect(ctx, b, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			if cs.InitializeResult().Capabilities.Resources != nil {
				t.Fatal("headless server advertises missing UI")
			}
			tools, err := cs.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, tool := range tools.Tools {
				if tool.Meta["ui"] != nil || tool.Meta["ui/resourceUri"] != nil {
					t.Fatal("headless tool advertises UI", tool.Name)
				}
			}
			unknown, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_not_a_tool", Arguments: map[string]any{}})
			if err == nil && !unknown.IsError {
				t.Fatal("unknown tool accepted")
			}
			if calls.Load() != 0 {
				t.Fatal("unknown tool reached daemon")
			}
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: fixture.Tool, Arguments: fixture.Arguments})
			if err != nil || result.IsError {
				t.Fatalf("call %v %v", result, err)
			}
			if !reflect.DeepEqual(result.StructuredContent, fixture.Result) {
				got, _ := json.Marshal(result.StructuredContent)
				want, _ := json.Marshal(fixture.Result)
				t.Fatalf("result parity\ngot  %s\nwant %s", got, want)
			}
			if len(result.Content) != 1 {
				t.Fatal("missing text fallback")
			}
			var fallback any
			if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &fallback); err != nil || !reflect.DeepEqual(fallback, fixture.Result) {
				t.Fatal("text fallback differs", err)
			}
			malformed.Store(true)
			result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: fixture.Tool, Arguments: fixture.Arguments})
			if err != nil || !result.IsError {
				t.Fatalf("malformed response accepted: %v %v", result, err)
			}
			message := result.Content[0].(*mcp.TextContent).Text
			code := "incompatible_response"
			if fixture.Request.Method == "POST" && fixture.Request.Path != "/preview" {
				code = "outcome_unknown"
			}
			if !strings.Contains(message, code) || strings.Contains(message, "private-parser-detail") {
				t.Fatal("unsafe error", message)
			}
			if calls.Load() != 2 {
				t.Fatal("adapter retried automatically", calls.Load())
			}
		})
	}
}
