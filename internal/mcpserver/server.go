// Package mcpserver implements Ding's tools using the official MCP Go SDK.
// It depends only on the scoped HTTP contract, never on the daemon or database.
package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Resolver func(context.Context, string, *auth.TokenInfo) (*mcpclient.Client, error)
type Options struct {
	Version, HTML string
	Resolve       Resolver
}

func New(options Options) (*mcp.Server, error) {
	if options.Resolve == nil {
		return nil, fmt.Errorf("a paired Ding connection is required")
	}
	catalog, err := mcpcontract.Load()
	if err != nil {
		return nil, err
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "Ding", Version: options.Version}, &mcp.ServerOptions{Instructions: mcpcontract.Instructions, Capabilities: &mcp.ServerCapabilities{}})
	// SDK 1.8.0 applies schema defaults to explicit null arguments before
	// validating their object type. Reject invalid shapes before that code path.
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
				raw := bytes.TrimSpace(call.Params.Arguments)
				if len(raw) > 12<<20 || len(raw) > 0 && raw[0] != '{' {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "invalid_arguments: arguments must be a bounded JSON object"}}}, nil
				}
			}
			return next(ctx, method, req)
		}
	})
	for _, tool := range catalog.Tools {
		if options.HTML == "" {
			delete(tool.Meta, "ui")
			delete(tool.Meta, "ui/resourceUri")
		}
		schema, err := mcpcontract.Resolve(tool.OutputSchema)
		if err != nil {
			return nil, err
		}
		mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, mcpcontract.View, error) {
			r, err := route(tool.Name, args)
			if err != nil {
				return nil, mcpcontract.View{}, err
			}
			var identity *auth.TokenInfo
			if req.Extra != nil {
				identity = req.Extra.TokenInfo
			}
			client, err := options.Resolve(ctx, r.scope, identity)
			if err != nil {
				return nil, mcpcontract.View{}, err
			}
			data, err := client.Call(ctx, r.method, r.path, r.query, r.body)
			if err != nil {
				return nil, mcpcontract.View{}, err
			}
			if r.query == nil {
				r.query = map[string]any{}
			}
			view, err := mcpcontract.Normalize(schema, mcpcontract.View{View: r.view, Data: data, Query: r.query, EvidenceNotice: mcpcontract.EvidenceNotice})
			// Even a successful HTTP response can be unreadable to this version.
			// Never encourage a fresh-key retry after a possibly committed write.
			if err != nil && r.method == http.MethodPost && r.path != "/preview" {
				err = mcpclient.Unknown()
			}
			return nil, view, err
		})
	}
	if options.HTML != "" {
		resource := catalog.Resources[0]
		s.AddResource(resource, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			content := *catalog.ResourceContents[0]
			content.Text = options.HTML
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{&content}}, nil
		})
	}
	return s, nil
}

type request struct {
	scope, method, path, view string
	query                     map[string]any
	body                      any
}

func route(name string, a map[string]any) (request, error) {
	r := request{scope: "inspect", method: http.MethodGet}
	id := func(key string) (string, error) { value, _ := a[key].(string); return mcpclient.Segment(value) }
	fields := func(names ...string) map[string]any {
		m := map[string]any{}
		for _, name := range names {
			m[name] = a[name]
		}
		return m
	}
	switch name {
	case "ding_get_capabilities":
		r.path = "/capabilities"
		r.view = "capabilities"
	case "ding_list_watches":
		r.path = "/watches"
		r.view = "watches"
		r.query = fields("search", "status", "cursor", "limit")
	case "ding_list_events":
		r.path = "/events"
		r.view = "events"
		r.query = fields("watch", "type", "cursor", "limit")
	case "ding_list_deliveries":
		r.path = "/deliveries"
		r.view = "deliveries"
		r.query = fields("watch", "status", "cursor", "limit")
	case "ding_list_destinations":
		r.path = "/destinations"
		r.view = "destinations"
		r.query = fields("cursor", "limit")
	case "ding_get_watch":
		value, err := id("watch_id")
		if err != nil {
			return r, err
		}
		r.path = "/watches/" + value
		r.view = "watch"
	case "ding_get_event":
		value, err := id("event_id")
		if err != nil {
			return r, err
		}
		r.path = "/events/" + value
		r.view = "evidence"
	case "ding_get_operation":
		value, err := id("operation_key")
		if err != nil {
			return r, err
		}
		r.path = "/operations/" + value
		r.view = "operation"
	case "ding_get_delivery":
		r.path = fmt.Sprintf("/deliveries/%.0f", a["delivery_id"])
		r.view = "delivery"
		r.query = fields("before")
	case "ding_preview_changes":
		r.scope = "preview"
		r.method = http.MethodPost
		r.path = "/preview"
		r.view = "preview"
		r.body = fields("manifest", "fixture", "watch")
	case "ding_apply_changes":
		r.scope = "manage"
		r.method = http.MethodPost
		r.path = "/apply"
		r.view = "applied"
		r.body = map[string]any{"handle": a["handle"], "operationKey": a["operation_key"]}
	case "ding_pause_watch", "ding_resume_watch", "ding_delete_watch":
		value, err := id("watch_id")
		if err != nil {
			return r, err
		}
		action := map[string]string{"ding_pause_watch": "pause", "ding_resume_watch": "resume", "ding_delete_watch": "delete"}[name]
		cancel, _ := a["cancel_pending"].(bool)
		r.scope = "manage"
		r.method = http.MethodPost
		r.path = "/watches/" + value + "/lifecycle"
		r.view = "lifecycle"
		r.body = map[string]any{"action": action, "expected": a["expected_revision"], "expectedGeneration": a["expected_generation"], "operationKey": a["operation_key"], "cancelPending": cancel}
	case "ding_retry_delivery":
		r.scope = "retry"
		r.method = http.MethodPost
		r.path = fmt.Sprintf("/deliveries/%.0f/retry", a["delivery_id"])
		r.view = "retry"
		r.body = map[string]any{"operationKey": a["operation_key"]}
	default:
		return r, mcpclient.Fail("invalid_tool", "unsupported integration tool")
	}
	return r, nil
}
