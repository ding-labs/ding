package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/ding-labs/ding/internal/mcpcontract"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ProfileResolver func(context.Context, *auth.TokenInfo) (mcpcontract.Profile, error)

func addProfileTool(server *mcp.Server, resolve ProfileResolver) {
	mcp.AddTool(server, mcpcontract.ProfileTool(), func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, mcpcontract.Profile, error) {
		var token *auth.TokenInfo
		if req.Extra != nil {
			token = req.Extra.TokenInfo
		}
		profile, err := resolve(ctx, token)
		if err != nil {
			return nil, mcpcontract.Profile{}, err
		}
		if strings.TrimSpace(profile.ID) == "" {
			return nil, mcpcontract.Profile{}, fmt.Errorf("profile_unavailable: reconnect Ding to identify this account")
		}
		// AddTool serializes the same object in structuredContent and JSON text.
		return nil, profile, nil
	})
}
