package mcpbridge

import (
	"context"
	"maps"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/ding-labs/ding/internal/mcpremote"
	"github.com/ding-labs/ding/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type call func(context.Context, string, map[string]any) (mcpcontract.View, error)
type backend func(context.Context, func(mcpcontract.Target, call) (mcpcontract.View, error)) (mcpcontract.View, error)

func local(client *mcpclient.Client) backend {
	return func(ctx context.Context, fn func(mcpcontract.Target, call) (mcpcontract.View, error)) (mcpcontract.View, error) {
		server, err := mcpserver.New(mcpserver.Options{TransferInspection: true, TransferTransport: true, Version: "bridge", Resolve: func(context.Context, string, *auth.TokenInfo) (*mcpclient.Client, error) { return client, nil }})
		if err != nil {
			return mcpcontract.View{}, err
		}
		a, b := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, a, nil)
		if err != nil {
			return mcpcontract.View{}, err
		}
		defer ss.Close()
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "Ding local bridge"}, nil).Connect(ctx, b, nil)
		if err != nil {
			return mcpcontract.View{}, err
		}
		defer cs.Close()
		invoke := sessionCall(cs)
		caps, err := invoke(ctx, "ding_get_capabilities", map[string]any{})
		if err != nil {
			return caps, err
		}
		data, ok := caps.Data.(map[string]any)
		if !ok {
			return caps, mcpclient.Fail("invalid_response", "local capabilities are unavailable")
		}
		instance, _ := data["instance"].(string)
		if instance == "" || data["execution"] != "local" {
			return caps, mcpclient.Fail("invalid_target", "the local pairing must identify a local Ding instance")
		}
		return fn(mcpcontract.Target{Instance: instance, Workspace: instance, Execution: "local"}, invoke)
	}
}

func remote(path string) backend {
	return func(ctx context.Context, fn func(mcpcontract.Target, call) (mcpcontract.View, error)) (view mcpcontract.View, err error) {
		err = mcpremote.WithSession(ctx, path, func(cs *mcp.ClientSession, p mcpremote.Profile) error {
			caps, e := mcpremote.Capabilities(ctx, cs, p)
			if e != nil {
				return e
			}
			invoke := sessionCall(cs)
			view, e = fn(*caps.Target, func(ctx context.Context, name string, args map[string]any) (mcpcontract.View, error) {
				args = maps.Clone(args)
				if name != "ding_list_workspaces" {
					args["workspace_id"] = p.Workspace
				}
				result, e := invoke(ctx, name, args)
				if e == nil && name == "ding_list_workspaces" {
					data, ok := result.Data.(map[string]any)
					if !ok {
						return result, mcpclient.Fail("invalid_response", "invalid workspace result")
					}
					choices, _ := data["workspaces"].([]any)
					selected := []any{}
					for _, choice := range choices {
						if w, ok := choice.(map[string]any); ok && w["id"] == p.Workspace {
							selected = append(selected, w)
						}
					}
					data["workspaces"] = selected
				}
				return result, e
			})
			return e
		})
		return view, err
	}
}

func sessionCall(cs *mcp.ClientSession) call {
	return func(ctx context.Context, name string, args map[string]any) (mcpcontract.View, error) {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			if _, write := args["operation_key"]; write && name != "ding_get_operation" {
				return mcpcontract.View{}, mcpclient.Unknown()
			}
			return mcpcontract.View{}, mcpclient.Fail("connection_unavailable", "selected connection failed; check its status and retry on this same target")
		}
		return mcpremote.View(result)
	}
}
