package mcpcontract

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCapturedContractAndSDK(t *testing.T) {
	b, err := os.ReadFile("../../testdata/mcp/fastmcp-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var original Catalog
	if err := json.Unmarshal(b, &original); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tools) != 15 {
		t.Fatal(len(c.Tools))
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "Ding", Version: "test"}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
	for i, tool := range c.Tools {
		delete(original.Tools[i].Meta, "fastmcp")
		if len(original.Tools[i].Meta) == 0 {
			original.Tools[i].Meta = nil
		}
		if !reflect.DeepEqual(tool, original.Tools[i]) {
			t.Fatalf("contract drift: %s", tool.Name)
		}
		if _, err := Resolve(tool.OutputSchema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, View, error) {
			return nil, View{View: "watches", Data: map[string]any{"watches": []any{}, "total": 0, "more": false, "cursor": ""}, Query: in, EvidenceNotice: EvidenceNotice}, nil
		})
	}
	ctx := context.Background()
	a, bTransport := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "contract", Version: "test"}, nil)
	cs, err := client.Connect(ctx, bTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		encoded, _ := json.Marshal(tool.Annotations)
		var fields map[string]any
		_ = json.Unmarshal(encoded, &fields)
		for _, name := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
			if _, ok := fields[name]; !ok {
				t.Fatalf("%s missing %s", tool.Name, name)
			}
		}
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_list_watches", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("SDK tool: %v %v", result, err)
	}
	data, _ := json.Marshal(result.StructuredContent)
	var view View
	_ = json.Unmarshal(data, &view)
	if view.Query["limit"] != float64(25) || view.Query["cursor"] != "" {
		t.Fatalf("defaults lost: %v", view.Query)
	}
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "ding_list_watches", Arguments: map[string]any{"limit": 101}})
	if err == nil && !result.IsError {
		t.Fatal("invalid bound accepted")
	}
}
