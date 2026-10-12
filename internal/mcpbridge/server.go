package mcpbridge

import (
	"bytes"
	"context"
	"maps"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Bridge struct {
	journal  string
	backends map[string]backend
}

func New(version, html, profile string, client *mcpclient.Client) (*mcp.Server, error) {
	if profile == "" || client == nil {
		return nil, mcpclient.Fail("invalid_config", "a local pairing and explicit cloud profile are required")
	}
	b := &Bridge{journal: profile + ".operations", backends: map[string]backend{"local": local(client), "cloud": remote(profile)}}
	return b.server(version, html)
}

func (b *Bridge) invoke(ctx context.Context, name string, args map[string]any) (mcpcontract.View, error) {
	connection, _ := args["connection_id"].(string)
	selected, ok := b.backends[connection]
	if !ok {
		return mcpcontract.View{}, mcpclient.Fail("connection_required", "choose connection_id local or cloud explicitly; Ding never falls back between them")
	}
	return selected(ctx, func(target mcpcontract.Target, invoke call) (mcpcontract.View, error) {
		target.Connection = connection
		if workspace, _ := args["workspace_id"].(string); workspace != "" && workspace != target.Workspace {
			return mcpcontract.View{}, mcpclient.Fail("invalid_target", "workspace differs from the selected connection profile")
		}
		if connection == "local" && (name == "ding_list_workspaces" || name == "ding_get_cloud_status") {
			return mcpcontract.View{}, mcpclient.Fail("unsupported_operation", "this operation requires the cloud connection")
		}
		if key, _ := args["operation_key"].(string); key != "" {
			if err := bind(b.journal, key, name, args, target, name != "ding_get_operation"); err != nil {
				return mcpcontract.View{}, err
			}
		}
		forward := maps.Clone(args)
		delete(forward, "connection_id")
		delete(forward, "workspace_id")
		view, err := invoke(ctx, name, forward)
		if err == nil {
			if view.Target != nil && (view.Target.Instance != target.Instance || view.Target.Workspace != target.Workspace) {
				return mcpcontract.View{}, mcpclient.Unknown()
			}
			view.Target = &target
			if name == "ding_get_capabilities" {
				if data, ok := view.Data.(map[string]any); ok {
					data["connectionSelection"] = true
				}
			}
		}
		return view, err
	})
}

func (b *Bridge) server(version, html string) (*mcp.Server, error) {
	catalog, err := mcpcontract.Load()
	if err != nil {
		return nil, err
	}
	catalog = mcpcontract.Cloud(mcpcontract.TransferInspection(catalog))
	s := mcp.NewServer(&mcp.Implementation{Name: "Ding", Version: version}, &mcp.ServerOptions{Instructions: mcpcontract.Instructions + " This adapter has separate local and cloud connections. Always supply connection_id. A cloud error never selects the local instance. Discover connections before choosing where a watch should run."})
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
				raw := bytes.TrimSpace(call.Params.Arguments)
				if len(raw) > 12<<20 || (len(raw) > 0 && raw[0] != '{') {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid_arguments: use a bounded object"}}}, nil
				}
			}
			return next(ctx, method, req)
		}
	})
	for _, tool := range catalog.Tools {
		properties := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
		properties["connection_id"] = map[string]any{"type": "string", "enum": []string{"local", "cloud"}, "description": "Required explicit execution target; never substitute another connection on failure."}
		// Omission reaches the actionable connection_required error, allowing the
		// embedded app to show discovery before the first user selection.
		if html == "" {
			delete(tool.Meta, "ui")
			delete(tool.Meta, "ui/resourceUri")
		}
		schema, err := mcpcontract.Resolve(tool.OutputSchema)
		if err != nil {
			return nil, err
		}
		mcp.AddTool(s, tool, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, mcpcontract.View, error) {
			view, err := b.invoke(ctx, tool.Name, args)
			if err != nil {
				return nil, view, err
			}
			view, err = mcpcontract.Normalize(schema, view)
			if err != nil && args["operation_key"] != nil && tool.Name != "ding_get_operation" {
				err = mcpclient.Unknown()
			}
			return nil, view, err
		})
	}
	mcp.AddTool(s, &mcp.Tool{Name: "ding_list_connections", Description: "List configured local and cloud choices without signing in or starting remote workers.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, mcpcontract.View, error) {
		return nil, mcpcontract.View{View: "connections", Query: map[string]any{}, Data: map[string]any{"connections": []any{map[string]any{"id": "local", "name": "This computer"}, map[string]any{"id": "cloud", "name": "Ding Cloud"}}}, EvidenceNotice: mcpcontract.EvidenceNotice}, nil
	})
	if html != "" {
		s.AddResource(catalog.Resources[0], func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			content := *catalog.ResourceContents[0]
			content.Text = html
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{&content}}, nil
		})
	}
	return s, nil
}
