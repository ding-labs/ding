package mcpserver

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Keep the advertised OAuth scopes and the resolver's action in one place.
func toolScope(name string) string {
	switch name {
	case "ding_preview_changes":
		return "preview"
	case "ding_apply_changes", "ding_pause_watch", "ding_resume_watch", "ding_delete_watch":
		return "manage"
	case "ding_retry_delivery":
		return "retry"
	case "ding_bridge_transfer_step":
		return "move"
	default:
		return "inspect"
	}
}

func toolScopes(name string) []string {
	scopes := []string{"ding:inspect"}
	if action := toolScope(name); action != "inspect" {
		scopes = append(scopes, "ding:"+action)
	}
	return scopes
}

type oauthScheme struct {
	Type   string   `json:"type"`
	Scopes []string `json:"scopes"`
}

type securedTool struct {
	*mcp.Tool
	SecuritySchemes []oauthScheme `json:"securitySchemes"`
}

type securedToolsResult struct {
	*mcp.ListToolsResult
	Tools []securedTool `json:"tools"`
}

// SDK 1.8 has no field for the OpenAI securitySchemes extension. Extend only
// the wire result, preserving the SDK's catalog, pagination, and result metadata.
func oauthToolMetadata(metadataURL string, names []string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// Return challenges before the typed SDK wrapper validates a success
			// output schema. Authentication errors intentionally have no View.
			if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil && slices.Contains(names, call.Params.Name) {
				var token *auth.TokenInfo
				if call.Extra != nil {
					token = call.Extra.TokenInfo
				}
				if challenge := toolAuthorization(token, call.Params.Name, metadataURL); challenge != nil {
					return challenge, nil
				}
			}
			result, err := next(ctx, method, req)
			list, ok := result.(*mcp.ListToolsResult)
			if err != nil || method != "tools/list" || !ok {
				return result, err
			}
			secured := &securedToolsResult{ListToolsResult: list, Tools: make([]securedTool, 0, len(list.Tools))}
			for _, tool := range list.Tools {
				secured.Tools = append(secured.Tools, securedTool{Tool: tool, SecuritySchemes: []oauthScheme{{Type: "oauth2", Scopes: toolScopes(tool.Name)}}})
			}
			return secured, nil
		}
	}

}

func toolAuthorization(token *auth.TokenInfo, name, metadataURL string) *mcp.CallToolResult {
	code, description := "", ""
	scopes := toolScopes(name)
	if token == nil || !token.Expiration.IsZero() && !time.Now().Before(token.Expiration) {
		code, description = "invalid_token", "Connect Ding to authorize this action."
	} else {
		for _, scope := range scopes {
			if !slices.Contains(token.Scopes, scope) {
				code, description = "insufficient_scope", "Reconnect Ding to authorize the required permissions."
				break
			}
		}
	}
	if code == "" {
		return nil
	}
	challenge := fmt.Sprintf("Bearer resource_metadata=%q, error=%q, error_description=%q, scope=%q", metadataURL, code, description, strings.Join(scopes, " "))
	return &mcp.CallToolResult{
		IsError: true,
		Meta:    mcp.Meta{"mcp/www_authenticate": []string{challenge}},
		Content: []mcp.Content{&mcp.TextContent{Text: description}},
	}
}
