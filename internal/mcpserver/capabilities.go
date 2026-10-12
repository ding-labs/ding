package mcpserver

import (
	"maps"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// Self-hosted HTTP maps an OAuth identity to a private daemon grant. The daemon
// cannot see that OAuth token, so intersect its advertised authority here too.
func tokenCapabilities(data any, token *auth.TokenInfo) any {
	caps, ok := data.(map[string]any)
	if !ok || token == nil {
		return data
	}
	caps = maps.Clone(caps)
	scopes, present := caps["effectiveScopes"]
	if !present {
		grant, _ := caps["grant"].(map[string]any)
		scopes = grant["scopes"]
	}
	effective := []string{}
	items, _ := scopes.([]any)
	for _, item := range items {
		scope, ok := item.(string)
		if ok && slices.Contains(token.Scopes, "ding:"+scope) {
			effective = append(effective, scope)
		}
	}
	caps["effectiveScopes"] = effective
	if !slices.Contains(effective, "manage") {
		caps["canManageDestinations"] = false
	}
	return caps
}
